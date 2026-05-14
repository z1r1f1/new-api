package chatgptimg

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestBuildChatPromptFromMessages(t *testing.T) {
	req := chatRequest{
		Messages: []dto.Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
			{Role: "user", Content: []any{
				map[string]any{"type": "text", "text": "how are you?"},
			}},
		},
	}
	got := buildChatPrompt(req)
	want := "System: be concise\n\nUser: hello\n\nAssistant: hi\n\nUser: how are you?"
	if got != want {
		t.Fatalf("unexpected prompt:\nwant: %q\n got: %q", want, got)
	}
}

func TestBuildChatPromptAddsImageGenerationInstruction(t *testing.T) {
	req := chatRequest{
		Model: "gpt-image-2",
		Messages: []dto.Message{
			{Role: "user", Content: "生成一张性感美女图片"},
		},
		ResponseFormat: &dto.ResponseFormat{Type: "json_object"},
	}
	got := buildChatPrompt(req)
	if !strings.Contains(got, "actually create and return image") {
		t.Fatalf("expected image generation instruction, got %q", got)
	}
	if strings.Contains(got, "valid JSON object only") {
		t.Fatalf("image generation prompt must not force JSON-only response: %q", got)
	}
}

func TestConvertOpenAIRequestAllowsChat(t *testing.T) {
	stream := false
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &dto.GeneralOpenAIRequest{
		Model:  "gpt-5",
		Stream: &stream,
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
		},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest returned error: %v", err)
	}
	req, ok := converted.(chatRequest)
	if !ok {
		t.Fatalf("expected chatRequest, got %T", converted)
	}
	if req.Model != "gpt-5" || len(req.Messages) != 1 || req.Stream == nil || *req.Stream {
		t.Fatalf("unexpected converted request: %#v", req)
	}
}

func TestStreamChatCompletionUsesRealConversationIDOnly(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["hello"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	pr, pw := io.Pipe()
	go streamChatCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test"}, "hello", imageBaseline{}, nil, "", pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read stream output failed: %v", err)
	}
	firstChunk := strings.SplitN(string(out), "\n\n", 2)[0]
	if strings.Contains(firstChunk, "chatcmpl-chatgptimg-") {
		t.Fatalf("initial stream chunk must not expose a synthetic ChatGPT Web conversation id: %s", firstChunk)
	}
	if !strings.Contains(string(out), `"id":"chatcmpl-chatgptimg-conv-1"`) {
		t.Fatalf("stream output did not expose the real conversation id for reuse:\n%s", string(out))
	}
}

func TestCollectChatGeneratedImageMarkdownSkipsLongPollForTextOnlyChat(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/backend-api/conversation/conv-text" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"mapping":{},"current_node":"node-1"}`))
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}

	start := time.Now()
	markdown, err := collectChatGeneratedImageMarkdown(context.Background(), client, "conv-text", imageBaseline{}, false, nil, "", "", "")
	if err != nil {
		t.Fatalf("collectChatGeneratedImageMarkdown returned error: %v", err)
	}
	if markdown != "" {
		t.Fatalf("expected no image markdown, got %q", markdown)
	}
	if requestCount != 1 {
		t.Fatalf("expected only initial mapping request, got %d", requestCount)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected text-only image check to return without long polling, took %s", elapsed)
	}
}

func TestShouldPollChatGeneratedImagesDetectsIntent(t *testing.T) {
	if !shouldPollChatGeneratedImages(chatRequest{Model: "gpt-5.5-pro"}, "User: 帮我生成图片：一只小猫", "", false) {
		t.Fatal("expected Chinese image generation intent to enable polling")
	}
	if !shouldPollChatGeneratedImages(chatRequest{Model: "gpt-5.5-pro"}, "User: 生成一张性感美女图片", "", false) {
		t.Fatal("expected Chinese generate-a-picture intent to enable polling")
	}
	if shouldPollChatGeneratedImages(chatRequest{Model: "gpt-5.5-pro"}, "User: hello", "hello", false) {
		t.Fatal("expected plain text chat to skip image polling")
	}
}

func TestParsePlaygroundImageReference(t *testing.T) {
	cases := []struct {
		name      string
		ref       string
		wantTask  string
		wantIndex int
		wantOK    bool
	}{
		{name: "relative", ref: "/pg/images/generations/task_abc/image/2", wantTask: "task_abc", wantIndex: 2, wantOK: true},
		{name: "absolute", ref: "https://example.com/pg/images/generations/task_xyz/image/0", wantTask: "task_xyz", wantIndex: 0, wantOK: true},
		{name: "public relative", ref: "/pg/public/images/generations/task_public/image/1", wantTask: "task_public", wantIndex: 1, wantOK: true},
		{name: "public absolute", ref: "http://151.145.66.232/pg/public/images/generations/task_ip/image/0", wantTask: "task_ip", wantIndex: 0, wantOK: true},
		{name: "invalid", ref: "https://example.com/not-an-image", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotTask, gotIndex, gotOK := parsePlaygroundImageReference(tc.ref)
			if gotOK != tc.wantOK || gotTask != tc.wantTask || gotIndex != tc.wantIndex {
				t.Fatalf("parsePlaygroundImageReference(%q) = (%q, %d, %v), want (%q, %d, %v)", tc.ref, gotTask, gotIndex, gotOK, tc.wantTask, tc.wantIndex, tc.wantOK)
			}
		})
	}
}

func TestRequestPublicBaseURLAddsForwardedPortWhenHostHasNoPort(t *testing.T) {
	c := newChatGPTImgTestContext("http://151.145.66.232/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "151.145.66.232",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "8999",
	})

	if got := requestPublicBaseURL(c); got != "http://151.145.66.232:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLDoesNotDuplicateExistingPort(t *testing.T) {
	c := newChatGPTImgTestContext("http://151.145.66.232/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "151.145.66.232:8999",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "80",
	})

	if got := requestPublicBaseURL(c); got != "http://151.145.66.232:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLUsesRFCForwardedHost(t *testing.T) {
	c := newChatGPTImgTestContext("http://internal.local/v1/images/generations", map[string]string{
		"Forwarded": "for=127.0.0.1;proto=http;host=151.145.66.232:8999",
	})

	if got := requestPublicBaseURL(c); got != "http://151.145.66.232:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLAddsForwardedPortToIPv6Host(t *testing.T) {
	c := newChatGPTImgTestContext("http://[2001:db8::1]/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "[2001:db8::1]",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "8999",
	})

	if got := requestPublicBaseURL(c); got != "http://[2001:db8::1]:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func newChatGPTImgTestContext(target string, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	c.Request = req
	return c
}

func TestConvertImageRequestCarriesConversationID(t *testing.T) {
	raw := []byte(`{"model":"gpt-image-2","prompt":"draw","conversation_id":"conv-123","fallback_prompt":"full local context","fallback_reference_images":["data:image/png;base64,abc"]}`)
	var req dto.ImageRequest
	if err := req.UnmarshalJSON(raw); err != nil {
		t.Fatalf("UnmarshalJSON returned error: %v", err)
	}
	converted, err := (&Adaptor{}).ConvertImageRequest(nil, nil, req)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}
	got, ok := converted.(generationRequest)
	if !ok {
		t.Fatalf("expected generationRequest, got %T", converted)
	}
	if got.ConversationID != "conv-123" {
		t.Fatalf("expected conversation id conv-123, got %q", got.ConversationID)
	}
	if got.FallbackPrompt != "full local context" {
		t.Fatalf("expected fallback prompt, got %q", got.FallbackPrompt)
	}
	if len(got.FallbackReferenceImages) != 1 || got.FallbackReferenceImages[0] != "data:image/png;base64,abc" {
		t.Fatalf("unexpected fallback reference images: %#v", got.FallbackReferenceImages)
	}
}

func TestImageSSETextWithoutImageError(t *testing.T) {
	err := imageSSETextWithoutImageError(ImageSSEResult{
		ConversationID: "conv-1",
		Content:        "I cannot generate that image.",
	})
	if err == nil {
		t.Fatal("expected text-only image SSE to fail")
	}
	if !strings.Contains(err.Error(), "no image generated") || !strings.Contains(err.Error(), "I cannot generate") {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := imageSSETextWithoutImageError(ImageSSEResult{
		Content:        "Image generation started.",
		ImageGenTaskID: "task-1",
	}); err != nil {
		t.Fatalf("expected image task to continue polling, got %v", err)
	}
}

func TestBuildGenerationResponseUsesSignedURLForURLField(t *testing.T) {
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/image.png" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(imageBytes)
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}
	signedURL := server.URL + "/image.png"
	resp, err := buildGenerationResponse(context.Background(), client, generationRequest{}, &imageRunResult{
		ConversationID: "conv-1",
		SignedURLs:     []string{signedURL},
	}, false, nil, "")
	if err != nil {
		t.Fatalf("buildGenerationResponse returned error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one image item, got %#v", resp.Data)
	}
	if resp.Data[0].Url != signedURL {
		t.Fatalf("expected signed URL in url field, got %q", resp.Data[0].Url)
	}
	if strings.HasPrefix(resp.Data[0].Url, "data:") {
		t.Fatalf("url field must not contain a data URL: %q", resp.Data[0].Url)
	}
	if resp.Data[0].B64Json == "" {
		t.Fatal("expected b64_json to be populated")
	}
}

func TestImageRefsToMarkdownUsesSignedURLNotDataURL(t *testing.T) {
	var imageFetchCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/files/file-1/download":
			_, _ = w.Write([]byte(`{"download_url":"` + "http://" + r.Host + `/image.png"}`))
		case "/image.png":
			imageFetchCount++
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}
	markdown := imageRefsToMarkdown(context.Background(), client, "conv-1", []string{"file-1"}, nil, "", "", "")
	if !strings.Contains(markdown, "]("+server.URL+"/image.png)") {
		t.Fatalf("expected markdown to contain signed URL, got %q", markdown)
	}
	if strings.Contains(markdown, "data:image/") {
		t.Fatalf("markdown must not contain a data URL: %q", markdown)
	}
	if imageFetchCount != 0 {
		t.Fatalf("expected markdown conversion not to fetch image bytes, got %d fetches", imageFetchCount)
	}
}

func TestMaterializeChatGPTContentImageURLsRemovesFailedUpstreamURL(t *testing.T) {
	client := &Client{hc: http.DefaultClient}
	content := "https://chatgpt.com/backend-api/estuary/content?id=file_1&sig=x"
	got := materializeChatGPTContentImageURLs(context.Background(), client, content, &relaycommon.RelayInfo{
		UserId: 1,
	}, "prompt", "gpt-image-2", "")
	if strings.Contains(got, "chatgpt.com/backend-api/estuary/content") {
		t.Fatalf("expected failed upstream URL to be removed, got %q", got)
	}
}

func TestExtractChatGPTImageURLsFromMarkdown(t *testing.T) {
	content := "done ![image](https://chatgpt.com/backend-api/estuary/content?id=file_1&amp;sig=x)"
	urls := extractChatGPTImageURLs(content)
	if len(urls) != 1 || !strings.Contains(urls[0], "/backend-api/estuary/content") {
		t.Fatalf("unexpected urls: %#v", urls)
	}
}

func TestReplaceChatGPTImageURLsWrapsRawURLAsMarkdownImage(t *testing.T) {
	content := "done https://chatgpt.com/backend-api/estuary/content?id=file_1&sig=x"
	got := replaceChatGPTImageURLs(content, map[string]string{
		"https://chatgpt.com/backend-api/estuary/content?id=file_1&sig=x": "/pg/public/images/generations/task_abc/image/0",
	})
	want := "done ![image_1](/pg/public/images/generations/task_abc/image/0)"
	if got != want {
		t.Fatalf("unexpected replacement:\nwant: %q\n got: %q", want, got)
	}
}

func TestReplaceChatGPTImageURLsKeepsMarkdownImageSyntax(t *testing.T) {
	content := "done ![result](https://chatgpt.com/backend-api/estuary/content?id=file_1&amp;sig=x)"
	got := replaceChatGPTImageURLs(content, map[string]string{
		"https://chatgpt.com/backend-api/estuary/content?id=file_1&sig=x": "/pg/public/images/generations/task_abc/image/0",
	})
	want := "done ![result](/pg/public/images/generations/task_abc/image/0)"
	if got != want {
		t.Fatalf("unexpected replacement:\nwant: %q\n got: %q", want, got)
	}
}

func TestBuildImageStreamPayloadWrapsFinalPayloadAndDone(t *testing.T) {
	payload := &generationResponse{
		Created:        123,
		ConversationID: "conv-1",
		Usage: dto.Usage{
			PromptTokens:     1,
			CompletionTokens: 2,
			TotalTokens:      3,
		},
		Data: []dto.ImageData{{Url: "https://example.com/image.png"}},
	}

	rawPayload, err := common.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	out := buildImageStreamPayload(rawPayload)
	if !strings.Contains(string(out), "data: [DONE]") {
		t.Fatalf("expected DONE marker, got %q", string(out))
	}
	decoded := strings.TrimSpace(strings.TrimPrefix(strings.SplitN(string(out), "\n\n", 2)[0], "data: "))
	var got generationResponse
	if err := common.UnmarshalJsonStr(decoded, &got); err != nil {
		t.Fatalf("failed to decode streamed payload: %v", err)
	}
	if got.ConversationID != "conv-1" || got.Usage.TotalTokens != 3 {
		t.Fatalf("unexpected streamed payload: %#v", got)
	}
}

type closeTrackingReadCloser struct {
	*bytes.Reader
	closed bool
}

func (c *closeTrackingReadCloser) Close() error {
	c.closed = true
	return nil
}

func TestStreamImageResponseClosesBodyAndWritesSSE(t *testing.T) {
	payload := &generationResponse{
		Created:        123,
		ConversationID: "conv-1",
		Usage: dto.Usage{
			PromptTokens:     1,
			CompletionTokens: 2,
			TotalTokens:      3,
		},
		Data: []dto.ImageData{{Url: "https://example.com/image.png"}},
	}
	rawPayload, err := common.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	body := buildImageStreamPayload(rawPayload)
	rc := &closeTrackingReadCloser{Reader: bytes.NewReader(body)}
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       rc,
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	usage, apiErr := streamImageResponse(c, resp)
	if apiErr != nil {
		t.Fatalf("streamImageResponse returned error: %v", apiErr)
	}
	if !rc.closed {
		t.Fatal("expected response body to be closed")
	}
	if usage == nil || usage.TotalTokens != 3 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected SSE content-type, got %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "data: [DONE]") {
		t.Fatalf("expected DONE marker in downstream SSE, got %q", recorder.Body.String())
	}
}
