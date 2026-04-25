package chatgptimg

import (
	"testing"
	"time"
)

func TestParseImageSSEUntilConversationReadyReturnsAfterQuietPeriod(t *testing.T) {
	stream := make(chan SSEEvent, 1)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1"}}`)}

	start := time.Now()
	result := ParseImageSSEUntilConversationReady(stream, 10*time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected early parser return, took %s", elapsed)
	}
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id to be captured, got %q", result.ConversationID)
	}
}

func TestParseImageSSEUntilConversationReadyReturnsOnImageRef(t *testing.T) {
	stream := make(chan SSEEvent, 1)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"content":{"parts":["file-service://file_abc"]}}}}`)}

	result := ParseImageSSEUntilConversationReady(stream, time.Second)
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id to be captured, got %q", result.ConversationID)
	}
	if len(result.FileIDs) != 1 || result.FileIDs[0] != "file_abc" {
		t.Fatalf("expected image file id to be captured, got %#v", result.FileIDs)
	}
}

func TestParseImageSSEDetectsUpstreamGenerationError(t *testing.T) {
	stream := make(chan SSEEvent, 1)
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"assistant"},"content":{"parts":["We experienced an error when generating images."]}}}}`)}
	close(stream)

	result := ParseImageSSE(stream)
	if result.Err == nil {
		t.Fatal("expected upstream image generation error")
	}
	if !containsImageGenerationUpstreamErrorText(result.Err.Error()) {
		t.Fatalf("expected specific upstream error text, got %v", result.Err)
	}
}

func TestMappingContainsImageGenerationError(t *testing.T) {
	mapping := map[string]any{
		"node-1": map[string]any{
			"message": map[string]any{
				"content": map[string]any{
					"parts": []any{"We experienced an error when generating images"},
				},
			},
		},
	}
	if !mappingContainsImageGenerationError(mapping) {
		t.Fatal("expected mapping error detector to match upstream image generation error")
	}
}

func TestParseChatSSEExtractsAssistantTextDelta(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["hel"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["hello"]},"metadata":{"finish_details":{"type":"stop"}}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if result.Content != "hello" {
		t.Fatalf("expected final content, got %q", result.Content)
	}
	if result.FinishType != "stop" {
		t.Fatalf("expected finish type stop, got %q", result.FinishType)
	}
}

func TestParseChatSSEExtractsPatchAppendEvents(t *testing.T) {
	stream := make(chan SSEEvent, 4)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"O"}`)}
	stream <- SSEEvent{Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"K"}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if result.Content != "OK" {
		t.Fatalf("expected patch content, got %q", result.Content)
	}
}

func TestParseChatSSEExtractsBareDeltaAfterAppendStarts(t *testing.T) {
	stream := make(chan SSEEvent, 5)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"Hel"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"v":"lo"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"v":" world"}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.Content != "Hello world" {
		t.Fatalf("expected full bare-delta content, got %q", result.Content)
	}
}
