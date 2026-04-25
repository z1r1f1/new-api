package chatgptimg

import (
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
