package chatgptimg

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestExtractImageRefsFromMappingFindsNestedAssets(t *testing.T) {
	mapping := map[string]any{
		"node-1": map[string]any{
			"message": map[string]any{
				"content": map[string]any{
					"parts": []any{
						map[string]any{"asset_pointer": "sediment://sed_nested"},
						"file-service://file_nested",
					},
				},
			},
		},
	}
	fileIDs, sedimentIDs := ExtractImageRefsFromMapping(mapping)
	if len(fileIDs) != 1 || fileIDs[0] != "file_nested" {
		t.Fatalf("expected nested file id, got %#v", fileIDs)
	}
	if len(sedimentIDs) != 1 || sedimentIDs[0] != "sed_nested" {
		t.Fatalf("expected nested sediment id, got %#v", sedimentIDs)
	}
}

func TestPollConversationForImagesReturnsPreviewWhenSedimentIsReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation/conv-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"mapping": {
				"tool-1": {
					"message": {
						"author": {"role": "tool", "name": "image_gen"},
						"metadata": {"async_task_type": "image_gen"},
						"content": {
							"content_type": "multimodal_text",
							"parts": [{"asset_pointer": "sediment://sed_ready"}]
						},
						"create_time": 1,
						"recipient": "image_gen.text2im"
					}
				}
			}
		}`))
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}

	status, fids, sids := client.PollConversationForImages(context.Background(), "conv-1", PollOpts{
		MaxWait:     50 * time.Millisecond,
		Interval:    time.Millisecond,
		PreviewWait: time.Millisecond,
	})
	if status != PollStatusPreviewOnly {
		t.Fatalf("expected preview status, got %s", status)
	}
	if len(fids) != 0 {
		t.Fatalf("expected no file ids, got %#v", fids)
	}
	if len(sids) != 1 || sids[0] != "sed_ready" {
		t.Fatalf("expected sediment id, got %#v", sids)
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
