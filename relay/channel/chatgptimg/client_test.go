package chatgptimg

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
)

func TestNoRelayRetryErrorIncludesHTTPStatusInMessage(t *testing.T) {
	err := noRelayRetry(errors.New("chatgpt web channel: upstream rate limited while polling image result"), http.StatusTooManyRequests)
	if err == nil {
		t.Fatal("expected no relay retry error")
	}
	if got := err.Error(); got != "HTTP 429: chatgpt web channel: upstream rate limited while polling image result" {
		t.Fatalf("unexpected error message: %q", got)
	}
	var noRetry interface {
		SkipRelayRetry() bool
		RelayStatusCode() int
	}
	if !errors.As(err, &noRetry) {
		t.Fatal("expected no relay retry interface")
	}
	if !noRetry.SkipRelayRetry() {
		t.Fatal("expected relay retry to be skipped")
	}
	if got := noRetry.RelayStatusCode(); got != http.StatusTooManyRequests {
		t.Fatalf("expected relay status 429, got %d", got)
	}
}

func TestGetCachedClientReusesMatchingOptions(t *testing.T) {
	resetChatGPTWebClientCacheForTest()
	t.Cleanup(resetChatGPTWebClientCacheForTest)

	opt := ClientOptions{
		BaseURL:   "https://chatgpt.com",
		AuthToken: "access-token-1",
		DeviceID:  "device-1",
		ProxyURL:  "",
		Timeout:   2 * time.Second,
	}

	first, hit, err := getCachedClient(opt)
	if err != nil {
		t.Fatalf("getCachedClient first returned error: %v", err)
	}
	if hit {
		t.Fatal("first client creation must not be reported as cache hit")
	}

	second, hit, err := getCachedClient(opt)
	if err != nil {
		t.Fatalf("getCachedClient second returned error: %v", err)
	}
	if !hit {
		t.Fatal("second matching client should be served from cache")
	}
	if first != second {
		t.Fatal("expected matching options to reuse the same client")
	}

	changed := opt
	changed.AuthToken = "access-token-2"
	third, hit, err := getCachedClient(changed)
	if err != nil {
		t.Fatalf("getCachedClient changed token returned error: %v", err)
	}
	if hit {
		t.Fatal("different access token must not reuse cached client")
	}
	if third == first {
		t.Fatal("different access token must create an isolated client")
	}
}

func TestRelayStatusErrorDoesNotSkipRelayRetry(t *testing.T) {
	err := relayStatusError(errors.New("chatgpt web channel: upstream rate limited while polling image result"), http.StatusTooManyRequests)
	if err == nil {
		t.Fatal("expected relay status error")
	}
	if got := err.Error(); got != "HTTP 429: chatgpt web channel: upstream rate limited while polling image result" {
		t.Fatalf("unexpected error message: %q", got)
	}
	var retryControl interface {
		SkipRelayRetry() bool
		RelayStatusCode() int
	}
	if !errors.As(err, &retryControl) {
		t.Fatal("expected retry control interface")
	}
	if retryControl.SkipRelayRetry() {
		t.Fatal("expected relay retry to remain enabled")
	}
	if got := retryControl.RelayStatusCode(); got != http.StatusTooManyRequests {
		t.Fatalf("expected relay status 429, got %d", got)
	}
}

func TestChatConversationPayloadIncludesWebModelAndThinkingEffort(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body failed: %v", err)
		}
		if err := common.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal request body failed: %v", err)
		}
		bodies = append(bodies, payload)

		switch r.URL.Path {
		case "/backend-api/f/conversation/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{
			BaseURL:    server.URL,
			AuthToken:  "access-token",
			DeviceID:   "device-id",
			SessionID:  "session-id",
			UserAgent:  defaultUserAgent,
			Language:   "zh-CN",
			SSETimeout: time.Second,
		},
		hc: server.Client(),
	}
	opt := ChatConvOpts{
		Prompt:         "hi",
		UpstreamModel:  "gpt-5-5-thinking",
		ThinkingEffort: "standard",
		ParentMsgID:    "client-created-root",
		ChatToken:      "requirements-token",
		SSETimeout:     time.Second,
	}
	if _, err := client.PrepareChatConversation(context.Background(), opt); err != nil {
		t.Fatalf("PrepareChatConversation returned error: %v", err)
	}
	stream, err := client.StreamChatConversation(context.Background(), opt)
	if err != nil {
		t.Fatalf("StreamChatConversation returned error: %v", err)
	}
	for range stream {
	}
	if len(bodies) != 2 {
		t.Fatalf("expected prepare and conversation requests, got %d", len(bodies))
	}
	for index, payload := range bodies {
		if got := payload["model"]; got != "gpt-5-5-thinking" {
			t.Fatalf("payload %d sent unexpected model: %#v", index, got)
		}
		if got := payload["thinking_effort"]; got != "standard" {
			t.Fatalf("payload %d sent unexpected thinking_effort: %#v", index, got)
		}
	}
}

func TestStreamChatConversationCapturesFinalPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{
			BaseURL:    server.URL,
			AuthToken:  "access-token",
			DeviceID:   "device-id",
			SessionID:  "session-id",
			UserAgent:  defaultUserAgent,
			Language:   "zh-CN",
			SSETimeout: time.Second,
		},
		hc: server.Client(),
	}
	var captured []byte
	opt := ChatConvOpts{
		Prompt:         "hi",
		UpstreamModel:  "gpt-5-5-thinking",
		ThinkingEffort: "standard",
		ParentMsgID:    "client-created-root",
		ChatToken:      "requirements-token",
		SSETimeout:     time.Second,
		CaptureRequestBody: func(body []byte) {
			captured = append([]byte(nil), body...)
		},
	}
	stream, err := client.StreamChatConversation(context.Background(), opt)
	if err != nil {
		t.Fatalf("StreamChatConversation returned error: %v", err)
	}
	for range stream {
	}
	if len(captured) == 0 {
		t.Fatal("expected final chat upstream payload to be captured")
	}
	var payload map[string]any
	if err := common.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("captured payload is not valid json: %v", err)
	}
	if got := payload["model"]; got != "gpt-5-5-thinking" {
		t.Fatalf("captured unexpected model: %#v", got)
	}
	if _, ok := payload["messages"]; !ok {
		t.Fatalf("captured final payload missing messages: %#v", payload)
	}
}

func TestImageConversationPayloadUsesImageModelAndGenerationHints(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body failed: %v", err)
		}
		if err := common.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal request body failed: %v", err)
		}
		bodies = append(bodies, payload)

		switch r.URL.Path {
		case "/backend-api/f/conversation/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{
			BaseURL:    server.URL,
			AuthToken:  "access-token",
			DeviceID:   "device-id",
			SessionID:  "session-id",
			UserAgent:  defaultUserAgent,
			Language:   "zh-CN",
			SSETimeout: time.Second,
		},
		hc: server.Client(),
	}
	opt := ImageConvOpts{
		Prompt:        "draw",
		UpstreamModel: "gpt-image-2",
		ParentMsgID:   "client-created-root",
		ChatToken:     "requirements-token",
		SSETimeout:    time.Second,
	}
	if _, err := client.PrepareFConversation(context.Background(), opt); err != nil {
		t.Fatalf("PrepareFConversation returned error: %v", err)
	}
	stream, err := client.StreamFConversation(context.Background(), opt)
	if err != nil {
		t.Fatalf("StreamFConversation returned error: %v", err)
	}
	for range stream {
	}
	if len(bodies) != 2 {
		t.Fatalf("expected prepare and conversation requests, got %d", len(bodies))
	}
	if got := bodies[0]["model"]; got != "gpt-image-2" {
		t.Fatalf("prepare sent unexpected model: %#v", got)
	}
	if got := bodies[1]["model"]; got != "gpt-image-2" {
		t.Fatalf("conversation sent unexpected model: %#v", got)
	}
	if got := bodies[0]["client_prepare_state"]; got != "none" {
		t.Fatalf("prepare sent unexpected client_prepare_state: %#v", got)
	}
	if got := bodies[1]["client_prepare_state"]; got != "success" {
		t.Fatalf("conversation sent unexpected client_prepare_state: %#v", got)
	}
	if got := bodies[0]["thinking_effort"]; got != "standard" {
		t.Fatalf("prepare sent unexpected thinking_effort: %#v", got)
	}
	if got := bodies[1]["thinking_effort"]; got != "standard" {
		t.Fatalf("conversation sent unexpected thinking_effort: %#v", got)
	}
	if hints, ok := bodies[0]["system_hints"].([]any); !ok || len(hints) != 1 || hints[0] != "picture_v2" {
		t.Fatalf("prepare sent unexpected system_hints: %#v", bodies[0]["system_hints"])
	}
	if hints, ok := bodies[1]["system_hints"].([]any); !ok || len(hints) != 0 {
		t.Fatalf("conversation sent unexpected system_hints: %#v", bodies[1]["system_hints"])
	}
}

func TestStreamFConversationCapturesFinalPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{
			BaseURL:    server.URL,
			AuthToken:  "access-token",
			DeviceID:   "device-id",
			SessionID:  "session-id",
			UserAgent:  defaultUserAgent,
			Language:   "zh-CN",
			SSETimeout: time.Second,
		},
		hc: server.Client(),
	}
	var captured []byte
	opt := ImageConvOpts{
		Prompt:        "draw",
		UpstreamModel: "gpt-image-2",
		ParentMsgID:   "client-created-root",
		ChatToken:     "requirements-token",
		SSETimeout:    time.Second,
		CaptureRequestBody: func(body []byte) {
			captured = append([]byte(nil), body...)
		},
	}
	stream, err := client.StreamFConversation(context.Background(), opt)
	if err != nil {
		t.Fatalf("StreamFConversation returned error: %v", err)
	}
	for range stream {
	}
	if len(captured) == 0 {
		t.Fatal("expected final image upstream payload to be captured")
	}
	var payload map[string]any
	if err := common.Unmarshal(captured, &payload); err != nil {
		t.Fatalf("captured payload is not valid json: %v", err)
	}
	if got := payload["model"]; got != "gpt-image-2" {
		t.Fatalf("captured unexpected model: %#v", got)
	}
	if _, ok := payload["messages"]; !ok {
		t.Fatalf("captured final payload missing messages: %#v", payload)
	}
}

func TestChatConversationPayloadIncludesDeepResearchHints(t *testing.T) {
	var bodies []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body failed: %v", err)
		}
		if err := common.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal request body failed: %v", err)
		}
		bodies = append(bodies, payload)

		switch r.URL.Path {
		case "/backend-api/f/conversation/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{
			BaseURL:    server.URL,
			AuthToken:  "access-token",
			DeviceID:   "device-id",
			SessionID:  "session-id",
			UserAgent:  defaultUserAgent,
			Language:   "zh-CN",
			SSETimeout: time.Second,
		},
		hc: server.Client(),
	}
	opt := ChatConvOpts{
		Prompt:         "hi",
		UpstreamModel:  "gpt-5-5-thinking",
		ThinkingEffort: "standard",
		DeepResearch:   true,
		ParentMsgID:    "client-created-root",
		ChatToken:      "requirements-token",
		SSETimeout:     time.Second,
	}
	if _, err := client.PrepareChatConversation(context.Background(), opt); err != nil {
		t.Fatalf("PrepareChatConversation returned error: %v", err)
	}
	stream, err := client.StreamChatConversation(context.Background(), opt)
	if err != nil {
		t.Fatalf("StreamChatConversation returned error: %v", err)
	}
	for range stream {
	}
	if len(bodies) != 2 {
		t.Fatalf("expected prepare and conversation requests, got %d", len(bodies))
	}

	for index, payload := range bodies {
		assertDeepResearchPayload(t, index, payload)
	}
}

func assertDeepResearchPayload(t *testing.T, index int, payload map[string]any) {
	t.Helper()
	hints, ok := payload["system_hints"].([]any)
	if !ok || len(hints) != 1 || hints[0] != "connector:connector_openai_deep_research" {
		t.Fatalf("payload %d sent unexpected system_hints: %#v", index, payload["system_hints"])
	}

	var metadata any
	if partialQuery, ok := payload["partial_query"].(map[string]any); ok {
		metadata = partialQuery["metadata"]
	} else if messages, ok := payload["messages"].([]any); ok && len(messages) > 0 {
		firstMessage, _ := messages[0].(map[string]any)
		metadata = firstMessage["metadata"]
	}

	meta, ok := metadata.(map[string]any)
	if !ok {
		t.Fatalf("payload %d missing deep research metadata: %#v", index, metadata)
	}
	if got := meta["deep_research_version"]; got != "standard" {
		t.Fatalf("payload %d sent unexpected deep_research_version: %#v", index, got)
	}
	if got := meta["venus_model_variant"]; got != "standard" {
		t.Fatalf("payload %d sent unexpected venus_model_variant: %#v", index, got)
	}
	selectedSources, ok := meta["selected_sources"].([]any)
	if !ok || len(selectedSources) != 1 || selectedSources[0] != "web" {
		t.Fatalf("payload %d sent unexpected selected_sources: %#v", index, meta["selected_sources"])
	}
}

func TestParseChatSSECapturesAssistantImageRefs(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"tool","name":"image_gen"},"metadata":{"async_task_type":"image_gen","image_gen_task_id":"task-1"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://file_generated"},{"asset_pointer":"sediment://sed_preview"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if len(result.FileIDs) != 1 || result.FileIDs[0] != "file_generated" {
		t.Fatalf("expected generated file id, got %#v", result.FileIDs)
	}
	if len(result.SedimentIDs) != 1 || result.SedimentIDs[0] != "sed_preview" {
		t.Fatalf("expected preview sediment id, got %#v", result.SedimentIDs)
	}
	if result.ImageGenTaskID != "task-1" {
		t.Fatalf("expected image task id, got %q", result.ImageGenTaskID)
	}
}

func TestParseChatSSEIgnoresUserUploadedImageRefs(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"user"},"content":{"parts":[{"asset_pointer":"file-service://uploaded_input"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"tool","name":"image_gen"},"metadata":{"async_task_type":"image_gen"},"content":{"parts":[{"asset_pointer":"file-service://generated_output"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseChatSSE(stream)
	if len(result.FileIDs) != 1 || result.FileIDs[0] != "generated_output" {
		t.Fatalf("expected only generated file id, got %#v", result.FileIDs)
	}
}

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

func TestParseImageSSECapturesSedimentOnlyRef(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"content":{"parts":[{"asset_pointer":"sediment://sed_only"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseImageSSE(stream)
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if len(result.FileIDs) != 0 {
		t.Fatalf("expected no file ids, got %#v", result.FileIDs)
	}
	if len(result.SedimentIDs) != 1 || result.SedimentIDs[0] != "sed_only" {
		t.Fatalf("expected sediment id, got %#v", result.SedimentIDs)
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

func TestParseImageSSECapturesAssistantTextWithoutImage(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"parts":["I cannot generate that image."]},"metadata":{"finish_details":{"type":"stop"}}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseImageSSE(stream)
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if result.Content != "I cannot generate that image." {
		t.Fatalf("expected assistant text to be captured, got %q", result.Content)
	}
	if len(result.FileIDs) != 0 || len(result.SedimentIDs) != 0 || result.ImageGenTaskID != "" {
		t.Fatalf("expected no image refs/task id, got files=%#v sediments=%#v task=%q", result.FileIDs, result.SedimentIDs, result.ImageGenTaskID)
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

func TestExtractImageRefsFromMappingSkipsUserUploadedAssets(t *testing.T) {
	mapping := map[string]any{
		"user-1": map[string]any{
			"message": map[string]any{
				"author": map[string]any{"role": "user"},
				"content": map[string]any{
					"parts": []any{
						map[string]any{"asset_pointer": "file-service://uploaded_input"},
						map[string]any{"asset_pointer": "sediment://uploaded_preview"},
					},
				},
			},
		},
		"tool-1": map[string]any{
			"message": map[string]any{
				"author":   map[string]any{"role": "tool", "name": "image_gen"},
				"metadata": map[string]any{"async_task_type": "image_gen"},
				"content": map[string]any{
					"parts": []any{
						map[string]any{"asset_pointer": "file-service://generated_output"},
						map[string]any{"asset_pointer": "sediment://generated_preview"},
					},
				},
			},
		},
	}

	fileIDs, sedimentIDs := ExtractImageRefsFromMapping(mapping)
	if len(fileIDs) != 1 || fileIDs[0] != "generated_output" {
		t.Fatalf("expected only generated file id, got %#v", fileIDs)
	}
	if len(sedimentIDs) != 1 || sedimentIDs[0] != "generated_preview" {
		t.Fatalf("expected only generated sediment id, got %#v", sedimentIDs)
	}
}

func TestParseImageSSEIgnoresUserUploadedAssetPointer(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"user"},"content":{"parts":[{"asset_pointer":"file-service://uploaded_input"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"tool","name":"image_gen"},"metadata":{"async_task_type":"image_gen"},"content":{"parts":[{"asset_pointer":"file-service://generated_output"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseImageSSE(stream)
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if len(result.FileIDs) != 1 || result.FileIDs[0] != "generated_output" {
		t.Fatalf("expected only generated output file id, got %#v", result.FileIDs)
	}
}

func TestFilterExcludedFileIDsRemovesUploadedReference(t *testing.T) {
	got := filterExcludedFileIDs([]string{"uploaded_input", "generated_output"}, map[string]struct{}{
		"uploaded_input": {},
	})
	if len(got) != 1 || got[0] != "generated_output" {
		t.Fatalf("expected only generated output file id, got %#v", got)
	}
}

func TestPollConversationForImagesReturnsPreviewWhenSedimentIsReady(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/files/library" {
			_, _ = w.Write([]byte(`{"items": []}`))
			return
		}
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

func TestPollConversationForImagesDoesNotReturnUploadedReference(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/files/library" {
			_, _ = w.Write([]byte(`{"items": []}`))
			return
		}
		if r.URL.Path != "/backend-api/conversation/conv-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"mapping": {
				"user-1": {
					"message": {
						"author": {"role": "user"},
						"content": {
							"content_type": "multimodal_text",
							"parts": [{"asset_pointer": "file-service://uploaded_input"}]
						}
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
		MaxWait:         5 * time.Millisecond,
		Interval:        time.Millisecond,
		PreviewWait:     time.Millisecond,
		ExcludedFileIDs: map[string]struct{}{"uploaded_input": {}},
	})
	if status != PollStatusTimeout {
		t.Fatalf("expected timeout instead of returning uploaded input, got status=%s fids=%#v sids=%#v", status, fids, sids)
	}
}

func TestPollConversationForImagesReportsCanceledContextSeparately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := &Client{opts: ClientOptions{BaseURL: "http://example.invalid"}}
	status, fids, sids := client.PollConversationForImages(ctx, "conv-1", PollOpts{
		MaxWait:  time.Second,
		Interval: time.Millisecond,
	})
	if status != PollStatusCanceled {
		t.Fatalf("expected canceled status, got status=%s fids=%#v sids=%#v", status, fids, sids)
	}
	if len(fids) != 0 || len(sids) != 0 {
		t.Fatalf("expected no refs for canceled context, got fids=%#v sids=%#v", fids, sids)
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

func TestParseChatSSEUntilReadyReturnsAfterFirstContent(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"pong"}`)}

	start := time.Now()
	result := ParseChatSSEUntilReady(stream, time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected parser to return after first content, took %s", elapsed)
	}
	if result.Err != nil {
		t.Fatalf("ParseChatSSEUntilReady returned error: %v", result.Err)
	}
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if result.Content != "pong" {
		t.Fatalf("expected first content, got %q", result.Content)
	}
}

func TestParseChatSSEUntilReadyReturnsAfterConversationIDQuietPeriod(t *testing.T) {
	stream := make(chan SSEEvent, 1)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}

	start := time.Now()
	result := ParseChatSSEUntilReady(stream, 10*time.Millisecond)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected parser to return after quiet period, took %s", elapsed)
	}
	if result.Err != nil {
		t.Fatalf("ParseChatSSEUntilReady returned error: %v", result.Err)
	}
	if result.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id, got %q", result.ConversationID)
	}
	if result.Content != "" {
		t.Fatalf("expected no content before full completion, got %q", result.Content)
	}
}

func TestParseChatSSEDetectsImageGenerationMarker(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"tool","name":"image_gen"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[]}}}}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if !result.HasImageGeneration {
		t.Fatal("expected image generation marker to be detected")
	}
}

func TestParseChatSSENormalizesSkippedMainlineInlineImage(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["{\"skipped_mainline\":true}\n\n![image_1](data:image/png;base64,iVBORw0KGgo=)"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if strings.Contains(result.Content, "skipped_mainline") {
		t.Fatalf("expected skipped_mainline metadata to be removed, got %q", result.Content)
	}
	if result.Content != "![image_1](data:image/png;base64,iVBORw0KGgo=)" {
		t.Fatalf("unexpected normalized content: %q", result.Content)
	}
	if !result.HasInlineImage {
		t.Fatal("expected inline data image to be detected")
	}
}

func TestParseChatSSEPatchSkipsMainlineMetadataDelta(t *testing.T) {
	stream := make(chan SSEEvent, 5)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"{\"skipped_mainline\":true}"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"v":"\n\n![image_1](data:image/png;base64,iVBORw0KGgo=)"}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.Content != "![image_1](data:image/png;base64,iVBORw0KGgo=)" {
		t.Fatalf("unexpected normalized patch content: %q", result.Content)
	}
	if !result.HasInlineImage {
		t.Fatal("expected inline data image to be detected")
	}
}

func TestParseChatSSESuppressesDeepResearchImplicitLinkDelta(t *testing.T) {
	stream := make(chan SSEEvent, 5)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"{\"path\":\"/Deep Research App/implicit_link::connector_openai_deep_research/start\",\"args\":{\"user"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"v":"\":\"alice\"}}"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"研究结果已完成"}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.Content != "研究结果已完成" {
		t.Fatalf("expected deep research internal payload to be suppressed, got %q", result.Content)
	}
	if strings.Contains(result.Content, "implicit_link::connector_openai_deep_research") || strings.Contains(result.Content, "Deep Research App") {
		t.Fatalf("deep research internal payload leaked into content: %q", result.Content)
	}
}

func TestParseChatSSESuppressesDeepResearchImplicitLinkSnapshot(t *testing.T) {
	stream := make(chan SSEEvent, 4)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["{\"path\":\"/Deep Research App/implicit_link::connector_openai_deep_research/start\",\"args\":{\"user"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["{\"path\":\"/Deep Research App/implicit_link::connector_openai_deep_research/start\",\"args\":{\"user\":\"alice\"}}\n\n最终研究报告"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.Content != "最终研究报告" {
		t.Fatalf("expected snapshot internal payload to be suppressed, got %q", result.Content)
	}
	if strings.Contains(result.Content, "implicit_link::connector_openai_deep_research") || strings.Contains(result.Content, "Deep Research App") {
		t.Fatalf("deep research internal payload leaked into content: %q", result.Content)
	}
}

func TestExtractLatestAssistantTextSuppressesDeepResearchImplicitLinkLine(t *testing.T) {
	internal := `{"path":"/Deep Research App/implicit_link::connector_openai_deep_research/start","args":{"user":"alice"}}`
	conversation := map[string]any{
		"current_node": "assistant-1",
		"mapping": map[string]any{
			"assistant-1": map[string]any{
				"message": map[string]any{
					"author":  map[string]any{"role": "assistant"},
					"content": map[string]any{"parts": []any{internal + "\n\n最终研究报告"}},
				},
			},
		},
	}

	if got := ExtractLatestAssistantTextFromConversation(conversation); got != "最终研究报告" {
		t.Fatalf("expected only final report text, got %q", got)
	}
}

func TestChatGPTWebConversationHasDeepResearchEmbeddedUI(t *testing.T) {
	conversation := map[string]any{
		"current_node": "assistant-empty",
		"mapping": map[string]any{
			"tool-1": map[string]any{
				"message": map[string]any{
					"author": map[string]any{"role": "tool", "name": "api_tool"},
					"content": map[string]any{
						"parts": []any{"The tool included embedded UI which has been displayed to the user.\n\nEmbedded UI description: \n\nRendered a widget that contains the deep research experience."},
					},
				},
			},
			"assistant-empty": map[string]any{
				"parent": "tool-1",
				"message": map[string]any{
					"author":  map[string]any{"role": "assistant"},
					"content": map[string]any{"parts": []any{""}},
				},
			},
		},
	}

	if !ChatGPTWebConversationHasDeepResearchEmbeddedUI(conversation) {
		t.Fatal("expected deep research embedded UI marker to be detected")
	}
	if got := ExtractLatestAssistantTextFromConversation(conversation); got != "" {
		t.Fatalf("embedded UI marker should not be treated as assistant text, got %q", got)
	}
}

func TestExtractDeepResearchReportTextFromConversationReadsWidgetState(t *testing.T) {
	conversation := map[string]any{
		"current_node": "assistant-empty",
		"mapping": map[string]any{
			"widget-state": map[string]any{
				"message": map[string]any{
					"author": map[string]any{"role": "tool", "name": "api_tool.widget_state"},
					"content": map[string]any{
						"parts": []any{`The latest state of the widget is: {"status":"completed","report_message":{"author":{"role":"assistant"},"update_time":2,"content":{"content_type":"text","parts":["# 最终报告\n\nharness 是测试夹具。"]}}}`},
					},
				},
			},
			"assistant-empty": map[string]any{
				"parent": "widget-state",
				"message": map[string]any{
					"author":  map[string]any{"role": "assistant"},
					"content": map[string]any{"parts": []any{""}},
				},
			},
		},
	}

	if got := ExtractDeepResearchReportTextFromConversation(conversation); got != "# 最终报告\n\nharness 是测试夹具。" {
		t.Fatalf("expected report text from widget state, got %q", got)
	}
	if got := ExtractLatestAssistantTextFromConversation(conversation); got != "" {
		t.Fatalf("widget state should not be treated as normal assistant text, got %q", got)
	}
}

func TestParseChatSSEPreservesOrdinaryDeepResearchText(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"字符串 implicit_link::connector_openai_deep_research 只是说明文字"}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.Content != "字符串 implicit_link::connector_openai_deep_research 只是说明文字" {
		t.Fatalf("expected ordinary explanatory text to be preserved, got %q", result.Content)
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

func TestCollectChatSSEEventDetectsStreamHandoff(t *testing.T) {
	state := &ChatSSEState{}
	_, done, err := CollectChatSSEEvent(SSEEvent{Data: []byte(`{"type":"stream_handoff","conversation_id":"conv-1","turn_exchange_id":"turn-1","options":[{"type":"resume_sse_endpoint","topic_id":"conversation-turn-1"}]}`)}, state)
	if err != nil {
		t.Fatalf("CollectChatSSEEvent returned error: %v", err)
	}
	if done {
		t.Fatal("stream_handoff should not finish the stream by itself")
	}
	if !state.HasStreamHandoff {
		t.Fatal("expected stream handoff marker to be recorded")
	}
	if state.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id to be captured, got %q", state.ConversationID)
	}
}

func TestParseChatSSEExtractsStructuredPatchText(t *testing.T) {
	stream := make(chan SSEEvent, 4)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":{"text":"Hello"}}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts","o":"replace","v":["Hello world"]}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"message_stream_complete","conversation_id":"conv-1"}`)}
	close(stream)

	result := ParseChatSSE(stream)
	if result.Err != nil {
		t.Fatalf("ParseChatSSE returned error: %v", result.Err)
	}
	if result.Content != "Hello world" {
		t.Fatalf("expected structured patch content, got %q", result.Content)
	}
}

func TestExtractLatestAssistantTextFromConversationUsesCurrentNode(t *testing.T) {
	conversation := map[string]any{
		"current_node": "assistant-2",
		"mapping": map[string]any{
			"user-1": map[string]any{
				"parent": nil,
				"message": map[string]any{
					"author":  map[string]any{"role": "user"},
					"content": map[string]any{"parts": []any{"hi"}},
				},
			},
			"assistant-1": map[string]any{
				"parent": "user-1",
				"message": map[string]any{
					"author":      map[string]any{"role": "assistant"},
					"create_time": float64(1),
					"content":     map[string]any{"parts": []any{"old"}},
				},
			},
			"assistant-2": map[string]any{
				"parent": "assistant-1",
				"message": map[string]any{
					"author":      map[string]any{"role": "assistant"},
					"create_time": float64(2),
					"content":     map[string]any{"parts": []any{map[string]any{"text": "new"}}},
				},
			},
		},
	}

	if got := ExtractLatestAssistantTextFromConversation(conversation); got != "new" {
		t.Fatalf("expected latest assistant text from current node, got %q", got)
	}
}

func TestPollConversationForImagesIgnoresBaselineToolMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/backend-api/files/library" {
			_, _ = w.Write([]byte(`{"items": []}`))
			return
		}
		if r.URL.Path != "/backend-api/conversation/conv-1" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"mapping": {
				"old-tool": {
					"message": {
						"author": {"role": "tool", "name": "image_gen"},
						"metadata": {"async_task_type": "image_gen"},
						"content": {"content_type": "multimodal_text", "parts": [{"asset_pointer": "sediment://old_sed"}]},
						"recipient": "image_gen.text2im"
					}
				},
				"new-tool": {
					"message": {
						"author": {"role": "tool", "name": "image_gen"},
						"metadata": {"async_task_type": "image_gen"},
						"content": {"content_type": "multimodal_text", "parts": [{"asset_pointer": "sediment://new_sed"}]},
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
		MaxWait:             50 * time.Millisecond,
		Interval:            time.Millisecond,
		PreviewWait:         time.Millisecond,
		BaselineToolIDs:     map[string]struct{}{"old-tool": {}},
		BaselineSedimentIDs: map[string]struct{}{"old_sed": {}},
	})
	if status != PollStatusPreviewOnly {
		t.Fatalf("expected preview status, got %s", status)
	}
	if len(fids) != 0 {
		t.Fatalf("expected no file ids, got %#v", fids)
	}
	if len(sids) != 1 || sids[0] != "new_sed" {
		t.Fatalf("expected only new sediment id, got %#v", sids)
	}
}

func TestProbeImageQuotaReadsConversationInitLimits(t *testing.T) {
	var sawInit bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte("ok"))
		case "/backend-api/conversation/init":
			sawInit = true
			if r.Method != http.MethodPost {
				t.Fatalf("expected POST, got %s", r.Method)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				t.Fatalf("unexpected authorization header: %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"blocked_features":["voice"],
				"default_model_slug":"gpt-5.5-thinking",
				"limits_progress":[
					{"feature_name":"message_cap","remaining":99,"max_value":100,"reset_after":"2026-05-15T10:00:00Z"},
					{"feature_name":"image_generation","remaining":7,"max_value":50,"reset_after":"2026-05-15T09:00:00Z"},
					{"feature_name":"image_edit","remaining":3,"cap":25,"reset_after":"2026-05-15T08:00:00Z"}
				]
			}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, AuthToken: "test-token", DeviceID: "test-device", Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	info, err := client.ProbeImageQuota(context.Background())
	if err != nil {
		t.Fatalf("probe image quota: %v", err)
	}
	if !sawInit {
		t.Fatal("expected conversation/init to be called")
	}
	if info.DefaultModelSlug != "gpt-5.5-thinking" {
		t.Fatalf("unexpected default model: %q", info.DefaultModelSlug)
	}
	if info.ImageQuotaRemaining != 3 {
		t.Fatalf("expected min remaining 3, got %d", info.ImageQuotaRemaining)
	}
	if info.ImageQuotaTotal != 50 {
		t.Fatalf("expected max total 50, got %d", info.ImageQuotaTotal)
	}
	if info.ImageQuotaResetAt != 1778832000 {
		t.Fatalf("expected earliest reset timestamp 1778832000, got %d", info.ImageQuotaResetAt)
	}
	if len(info.BlockedFeatures) != 1 || info.BlockedFeatures[0] != "voice" {
		t.Fatalf("unexpected blocked features: %#v", info.BlockedFeatures)
	}
}

func TestProbeImageQuotaFallsBackTotalFromUsed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			_, _ = w.Write([]byte("ok"))
			return
		}
		if r.URL.Path != "/backend-api/conversation/init" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"limits_progress":[
				{"feature_name":"img_gen","remaining":8,"used":12}
			]
		}`))
	}))
	defer server.Close()

	client, err := NewClient(ClientOptions{BaseURL: server.URL, AuthToken: "test-token", DeviceID: "test-device", Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	info, err := client.ProbeImageQuota(context.Background())
	if err != nil {
		t.Fatalf("probe image quota: %v", err)
	}
	if info.ImageQuotaRemaining != 8 {
		t.Fatalf("expected remaining 8, got %d", info.ImageQuotaRemaining)
	}
	if info.ImageQuotaTotal != 20 {
		t.Fatalf("expected fallback total 20, got %d", info.ImageQuotaTotal)
	}
}

func TestInferImageQuotaWindow(t *testing.T) {
	now := time.Unix(1778832000, 0)
	cases := []struct {
		name    string
		resetAt int64
		want    string
	}{
		{name: "missing reset", resetAt: 0, want: "unknown"},
		{name: "resetting soon", resetAt: now.Add(10 * time.Minute).Unix(), want: "resetting_soon"},
		{name: "daily", resetAt: now.Add(24 * time.Hour).Unix(), want: "daily"},
		{name: "weekly", resetAt: now.Add(7 * 24 * time.Hour).Unix(), want: "weekly"},
		{name: "monthly", resetAt: now.Add(30 * 24 * time.Hour).Unix(), want: "monthly"},
		{name: "too far", resetAt: now.Add(60 * 24 * time.Hour).Unix(), want: "unknown"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := inferImageQuotaWindow(tc.resetAt, now); got != tc.want {
				t.Fatalf("inferImageQuotaWindow() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestImageQuotaResetAfterSeconds(t *testing.T) {
	now := time.Unix(1778832000, 0)
	if got := imageQuotaResetAfterSeconds(now.Add(2*time.Hour).Unix(), now); got != 7200 {
		t.Fatalf("expected 7200 reset seconds, got %d", got)
	}
	if got := imageQuotaResetAfterSeconds(now.Add(-time.Hour).Unix(), now); got != 0 {
		t.Fatalf("past reset time should clamp to 0, got %d", got)
	}
	if got := imageQuotaResetAfterSeconds(0, now); got != 0 {
		t.Fatalf("missing reset time should be 0, got %d", got)
	}
}

func TestParseProcessUploadStreamLibraryFileID(t *testing.T) {
	got, err := parseProcessUploadStreamLibraryFileID([]byte("data: {\"event\":\"done\",\"extra\":{\"metadata_object_id\":\"file-lib-1\"}}\n\n[DONE]\n"))
	if err != nil {
		t.Fatalf("parse returned error: %v", err)
	}
	if got != "file-lib-1" {
		t.Fatalf("expected library file id, got %q", got)
	}
}

func TestImageDownloadURLAcceptsDirectImageResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation/conv-1/attachment/sed-1/download" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte{0x89, 0x50, 0x4e, 0x47})
	}))
	defer server.Close()

	client := &Client{opts: ClientOptions{BaseURL: server.URL, UserAgent: defaultUserAgent}, hc: server.Client()}
	downloadURL, err := client.ImageDownloadURL(context.Background(), "conv-1", "sed:sed-1")
	if err != nil {
		t.Fatalf("ImageDownloadURL returned error: %v", err)
	}
	if downloadURL != server.URL+"/backend-api/conversation/conv-1/attachment/sed-1/download" {
		t.Fatalf("expected direct attachment URL, got %q", downloadURL)
	}
}

func TestUploadFileProcessesImageIntoLibrary(t *testing.T) {
	pngData, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lCqF4gAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	var processCalled bool
	var uploadURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/files":
			uploadURL = "http://" + r.Host + "/upload"
			_, _ = w.Write([]byte(`{"file_id":"file-upload-1","upload_url":"` + uploadURL + `"}`))
		case "/upload":
			if r.Method != http.MethodPut {
				t.Fatalf("expected PUT upload, got %s", r.Method)
			}
			w.WriteHeader(http.StatusNoContent)
		case "/backend-api/files/file-upload-1/uploaded":
			_, _ = w.Write([]byte(`{"download_url":"https://example.test/image.png"}`))
		case "/backend-api/files/process_upload_stream":
			processCalled = true
			if ct := r.Header.Get("Accept"); ct != "text/event-stream" {
				t.Fatalf("expected event-stream accept, got %q", ct)
			}
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `"library_persistence_mode":"opportunistic"`) {
				t.Fatalf("process body missing library persistence mode: %s", body)
			}
			_, _ = w.Write([]byte("data: {\"extra\":{\"metadata_object_id\":\"file-library-1\"}}\n"))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{opts: ClientOptions{BaseURL: server.URL, UserAgent: defaultUserAgent}, hc: server.Client()}
	uploaded, err := client.UploadFile(context.Background(), pngData, "reference.png")
	if err != nil {
		t.Fatalf("UploadFile returned error: %v", err)
	}
	if !processCalled {
		t.Fatal("expected process_upload_stream to be called")
	}
	if uploaded.FileID != "file-upload-1" || uploaded.LibraryFileID != "file-library-1" {
		t.Fatalf("unexpected upload metadata: %#v", uploaded)
	}
}

func TestLibraryImageIDsFiltersCurrentConversationReadyImages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/files/library" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{
			"items": [
				{"file_id":"file-ref","mime_type":"image/png","library_file_category":"image","state":"ready","origination_thread_id":"conv-1"},
				{"file_id":"file-generated","mime_type":"image/png","library_file_category":"image","state":"ready","origination_thread_id":"conv-1"},
				{"file_id":"file-other","mime_type":"image/png","library_file_category":"image","state":"ready","origination_thread_id":"conv-2"},
				{"file_id":"file-pending","mime_type":"image/png","library_file_category":"image","state":"processing","origination_thread_id":"conv-1"}
			]
		}`))
	}))
	defer server.Close()

	client := &Client{opts: ClientOptions{BaseURL: server.URL}, hc: server.Client()}
	ids, err := client.LibraryImageIDs(context.Background(), "conv-1", map[string]struct{}{"file-ref": {}})
	if err != nil {
		t.Fatalf("LibraryImageIDs returned error: %v", err)
	}
	if len(ids) != 1 || ids[0] != "file-generated" {
		t.Fatalf("expected only generated library file, got %#v", ids)
	}
}

func TestPollConversationForImagesUsesLibraryFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/conversation/conv-1":
			_, _ = w.Write([]byte(`{"mapping": {}}`))
		case "/backend-api/files/library":
			_, _ = w.Write([]byte(`{"items":[{"file_id":"file-library-generated","mime_type":"image/png","library_file_category":"image","state":"ready","origination_thread_id":"conv-1"}]}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := &Client{opts: ClientOptions{BaseURL: server.URL}, hc: server.Client()}
	status, fids, sids := client.PollConversationForImages(context.Background(), "conv-1", PollOpts{
		MaxWait:  50 * time.Millisecond,
		Interval: time.Millisecond,
	})
	if status != PollStatusIMG2 {
		t.Fatalf("expected img2 status, got %s", status)
	}
	if len(fids) != 1 || fids[0] != "file-library-generated" || len(sids) != 0 {
		t.Fatalf("unexpected refs fids=%#v sids=%#v", fids, sids)
	}
}

func TestUploadedFileIDSetIncludesLibraryFileID(t *testing.T) {
	got := uploadedFileIDSet([]*UploadedFile{{FileID: "file-upload", LibraryFileID: "file-library"}})
	if _, ok := got["file-upload"]; !ok {
		t.Fatalf("missing file id in set: %#v", got)
	}
	if _, ok := got["file-library"]; !ok {
		t.Fatalf("missing library file id in set: %#v", got)
	}
}
