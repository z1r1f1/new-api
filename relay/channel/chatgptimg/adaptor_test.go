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
	go streamChatCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test"}, "hello", imageBaseline{}, nil, pw)

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
	markdown, err := collectChatGeneratedImageMarkdown(context.Background(), client, "conv-text", imageBaseline{}, false)
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
