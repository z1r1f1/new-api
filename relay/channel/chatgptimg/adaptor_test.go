package chatgptimg

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/dto"
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
	go streamChatCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test"}, "hello", imageBaseline{}, pw)

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
