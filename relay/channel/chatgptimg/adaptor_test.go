package chatgptimg

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/QuantumNous/new-api/types"
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

func TestBuildChatPromptIgnoresAssistantImageGenerationText(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-instant",
		Messages: []dto.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "我可以帮你生成图片，也可以解释这类任务为什么会变慢。"},
			{Role: "user", Content: "继续解释"},
		},
		ResponseFormat: &dto.ResponseFormat{Type: "json_object"},
	}

	got := buildChatPrompt(req)
	if strings.Contains(got, chatImageGenerationInstruction) {
		t.Fatalf("assistant text must not inject image generation instruction: %q", got)
	}
	if !strings.Contains(got, "valid JSON object only") {
		t.Fatalf("non-image prompt should preserve JSON response-format instruction: %q", got)
	}
}

func TestBuildChatPromptAddsToolBridgeInstruction(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "读取 AGENTS.md"},
		},
		Tools: []dto.ToolCallRequest{{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        "Read",
				Description: "Reads a file from the local filesystem",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"file_path": map[string]any{"type": "string"},
					},
					"required": []any{"file_path"},
				},
			},
		}},
	}
	got := buildChatPrompt(req)
	for _, want := range []string{"TOOL BRIDGE PROTOCOL", "MUST call an available tool", "Treat the listed tools", `"tool_call"`, "Read", "file_path"} {
		if !strings.Contains(got, want) {
			t.Fatalf("tool bridge prompt missing %q:\n%s", want, got)
		}
	}
}

func TestConvertClaudeRequestUsesChatGPTWebChatPath(t *testing.T) {
	stream := true
	maxTokens := uint(128)
	req := &dto.ClaudeRequest{
		Model:     "gpt-5.5-thinking",
		System:    "be concise",
		MaxTokens: &maxTokens,
		Stream:    &stream,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "读取 AGENTS.md"},
		},
		Tools: []dto.Tool{{
			Name:        "Read",
			Description: "Reads a file from the local filesystem",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string"},
				},
			},
		}},
	}
	info := &relaycommon.RelayInfo{
		OriginModelName:    "gpt-5.5-thinking",
		RelayFormat:        types.RelayFormatClaude,
		ClaudeConvertInfo:  &relaycommon.ClaudeConvertInfo{},
		ShouldIncludeUsage: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName:    "gpt-5.5-thinking",
			SupportStreamOptions: true,
		},
	}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(nil, info, req)
	if err != nil {
		t.Fatalf("ConvertClaudeRequest returned error: %v", err)
	}
	chatReq, ok := converted.(chatRequest)
	if !ok {
		t.Fatalf("expected chatRequest, got %T", converted)
	}
	if chatReq.Model != "gpt-5.5-thinking" {
		t.Fatalf("unexpected model: %q", chatReq.Model)
	}
	if chatReq.Stream == nil || !*chatReq.Stream {
		t.Fatalf("expected stream=true to be preserved, got %#v", chatReq.Stream)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("expected system and user messages after conversion, got %#v", chatReq.Messages)
	}
	if got := chatReq.Messages[0].Role; got != "system" {
		t.Fatalf("expected first message to be system, got %q", got)
	}
	if len(chatReq.Tools) != 1 || chatReq.Tools[0].Function.Name != "Read" {
		t.Fatalf("expected Claude tools to be converted for ChatGPT Web tool bridge, got %#v", chatReq.Tools)
	}
}

func TestChatGPTWebImagePollMaxWaitDefaultsToTenMinutes(t *testing.T) {
	t.Setenv("CHATGPT_WEB_IMAGE_POLL_TIMEOUT_SECONDS", "")
	if got := chatGPTWebImagePollMaxWait(false); got != 10*time.Minute {
		t.Fatalf("expected normal image poll timeout to default to 10m, got %s", got)
	}
	if got := chatGPTWebImagePollMaxWait(true); got != 45*time.Second {
		t.Fatalf("expected channel-test image poll timeout to stay short, got %s", got)
	}
}

func TestChatGPTWebImagePollMaxWaitCanBeExtendedByEnv(t *testing.T) {
	t.Setenv("CHATGPT_WEB_IMAGE_POLL_TIMEOUT_SECONDS", "900")
	if got := chatGPTWebImagePollMaxWait(false); got != 15*time.Minute {
		t.Fatalf("expected env image poll timeout to be 15m, got %s", got)
	}
}

func TestChatGPTWebImageOperationContextIgnoresClientCancel(t *testing.T) {
	type ctxKey string
	parent := context.WithValue(context.Background(), ctxKey("request_id"), "req-1")
	parent, cancelParent := context.WithCancel(parent)
	cancelParent()

	timing := service.NewChatGPTWebTiming()
	ctx, cancel := chatGPTWebImageOperationContext(parent, false, timing)
	defer cancel()

	select {
	case <-ctx.Done():
		t.Fatalf("image operation context must not be canceled by client context: %v", ctx.Err())
	default:
	}
	if got := ctx.Value(ctxKey("request_id")); got != "req-1" {
		t.Fatalf("expected context values to be preserved, got %#v", got)
	}
	snapshot := timing.Snapshot()
	if snapshot["image_request_context_detached"] != true {
		t.Fatalf("expected timing to mark detached context, got %#v", snapshot)
	}
}

func TestResolveImageDownloadURLsRetriesUntilReady(t *testing.T) {
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/files/file_ready/download" && r.URL.Path != "/backend-api/files/download/file_ready" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Path == "/backend-api/files/download/file_ready" {
			http.Error(w, `{"error":"legacy not ready"}`, http.StatusNotFound)
			return
		}
		attempts++
		if attempts == 1 {
			http.Error(w, `{"error":"not ready"}`, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"download_url":"https://example.test/image.png"}`))
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}
	timing := service.NewChatGPTWebTiming()
	urls := resolveImageDownloadURLsWithWait(context.Background(), client, "conv-1", []string{"file_ready"}, 100*time.Millisecond, time.Millisecond, timing)
	if len(urls) != 1 || urls[0] != "https://example.test/image.png" {
		t.Fatalf("expected retried download URL, got %#v", urls)
	}
	if attempts < 2 {
		t.Fatalf("expected at least two attempts, got %d", attempts)
	}
	snapshot := timing.Snapshot()
	if snapshot["image_download_url_attempts"] != attempts {
		t.Fatalf("expected attempt count in timing, got %#v", snapshot)
	}
}

func TestRunImageGenerationFiltersBaselineRefsFromSSE(t *testing.T) {
	var mappingCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/sentinel/chat-requirements/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"persona":"chatgpt","prepare_token":"prepare-token","turnstile":{"required":false},"proofofwork":{"required":false}}`))
		case "/backend-api/sentinel/chat-requirements/finalize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"requirements-token","persona":"chatgpt"}`))
		case "/backend-api/conversation/conv-1":
			mappingCalls++
			w.Header().Set("Content-Type", "application/json")
			if mappingCalls == 1 {
				_, _ = w.Write([]byte(`{
					"current_node":"old-node",
					"mapping":{
						"old-node":{"message":{"author":{"role":"assistant"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://old-file"},{"asset_pointer":"sediment://old-sed"}]}}}
					}
				}`))
				return
			}
			_, _ = w.Write([]byte(`{
				"current_node":"new-tool",
				"mapping":{
					"old-node":{"message":{"author":{"role":"assistant"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://old-file"},{"asset_pointer":"sediment://old-sed"}]}}},
					"new-tool":{"message":{"author":{"role":"tool","name":"dalle.text2im"},"recipient":"assistant","metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://new-file"}]}}}
				}
			}`))
		case "/backend-api/f/conversation/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`data: {"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://old-file"},{"asset_pointer":"sediment://old-sed"}]}}}}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		case "/backend-api/files/old-file/download", "/backend-api/files/download/old-file":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":"https://example.test/old.png"}`))
		case "/backend-api/conversation/conv-1/attachment/old-sed/download":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":"https://example.test/old-sed.png"}`))
		case "/backend-api/files/new-file/download", "/backend-api/files/download/new-file":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"download_url":"https://example.test/new.png"}`))
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

	result, err := runImageGeneration(context.Background(), client, generationRequest{
		Model:          "gpt-image-2",
		Prompt:         "edit this image",
		ConversationID: "conv-1",
		N:              1,
	}, nil, false, nil)
	if err != nil {
		t.Fatalf("runImageGeneration returned error: %v", err)
	}

	var hasOld, hasNew bool
	for _, ref := range result.FileRefs {
		switch ref {
		case "old-file", "sed:old-sed":
			hasOld = true
		case "new-file":
			hasNew = true
		}
	}
	if hasOld {
		t.Fatalf("SSE baseline refs must not be returned as the new image result: %#v", result.FileRefs)
	}
	if !hasNew {
		t.Fatalf("expected poll fallback to return new-file after filtering SSE baseline refs, got %#v", result.FileRefs)
	}
	if len(result.SignedURLs) != 1 || result.SignedURLs[0] != "https://example.test/new.png" {
		t.Fatalf("expected only new image download URL, got %#v", result.SignedURLs)
	}
}

func TestPreemptiveLocalToolResponseForClaudeFileRead(t *testing.T) {
	service.InitTokenEncoders()
	stream := true
	req := chatRequest{
		Model:  "gpt-5.5-thinking",
		Stream: &stream,
		Messages: []dto.Message{
			{Role: "user", Content: "读取 /Users/zrf/project/git-project/aicustomer/AGENTS.md"},
		},
		Tools: []dto.ToolCallRequest{{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        "exec_command",
				Description: "Runs a local shell command",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RelayMode:   relayconstant.RelayModeResponses,
	}
	timing := service.NewChatGPTWebTiming()

	resp, ok, err := buildChatGPTWebPreemptiveLocalToolResponse(req, info, timing)
	if err != nil {
		t.Fatalf("buildChatGPTWebPreemptiveLocalToolResponse returned error: %v", err)
	}
	if !ok || resp == nil || resp.Body == nil {
		t.Fatal("expected preemptive tool response")
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	for _, want := range []string{
		`"type":"function_call"`,
		`"name":"exec_command"`,
		`cat 'AGENTS.md'`,
		`"workdir":"/Users/zrf/project/git-project/aicustomer"`,
		`data: [DONE]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected synthetic response to contain %q, got:\n%s", want, got)
		}
	}
	if timing.Snapshot()["tool_bridge_preemptive"] != true {
		t.Fatalf("expected timing to mark preemptive tool bridge, got %#v", timing.Snapshot())
	}
}

func TestPreemptiveLocalToolResponseForOpenAIResponsesFileRead(t *testing.T) {
	service.InitTokenEncoders()
	stream := true
	req := chatRequest{
		Model:  "gpt-5.5-thinking",
		Stream: &stream,
		Messages: []dto.Message{
			{Role: "user", Content: "读取 AGENTS.md"},
		},
		Tools: []dto.ToolCallRequest{{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        "Read",
				Description: "Reads a file from the local filesystem",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatOpenAI,
		RelayMode:   relayconstant.RelayModeResponses,
	}
	timing := service.NewChatGPTWebTiming()

	resp, ok, err := buildChatGPTWebPreemptiveLocalToolResponse(req, info, timing)
	if err != nil {
		t.Fatalf("buildChatGPTWebPreemptiveLocalToolResponse returned error: %v", err)
	}
	if !ok || resp == nil || resp.Body == nil {
		t.Fatal("expected preemptive responses tool response")
	}
	body, _ := io.ReadAll(resp.Body)
	got := string(body)
	for _, want := range []string{
		`"type":"function_call"`,
		`"name":"Read"`,
		`"file_path":"AGENTS.md"`,
		`"type":"response.completed"`,
		`data: [DONE]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected synthetic responses stream to contain %q, got:\n%s", want, got)
		}
	}
	if timing.Snapshot()["tool_bridge_preemptive"] != true {
		t.Fatalf("expected timing to mark preemptive tool bridge, got %#v", timing.Snapshot())
	}
}

func TestPreemptiveLocalToolResponseSkipsWhenToolResultPresent(t *testing.T) {
	stream := true
	req := chatRequest{
		Model:  "gpt-5.5-thinking",
		Stream: &stream,
		Messages: []dto.Message{
			{Role: "user", Content: "读取 /Users/zrf/project/git-project/aicustomer/AGENTS.md"},
			{Role: "assistant", Content: `[function_call] exec_command {"cmd":"cat 'AGENTS.md'"}`},
			{Role: "tool", ToolCallId: "call_1", Content: "# AGENTS.md\n项目规则"},
		},
		Tools: []dto.ToolCallRequest{{
			Type:     "function",
			Function: dto.FunctionRequest{Name: "exec_command"},
		}},
	}
	info := &relaycommon.RelayInfo{
		RelayFormat: types.RelayFormatClaude,
		RelayMode:   relayconstant.RelayModeResponses,
	}
	timing := service.NewChatGPTWebTiming()

	resp, ok, err := buildChatGPTWebPreemptiveLocalToolResponse(req, info, timing)
	if err != nil {
		t.Fatalf("buildChatGPTWebPreemptiveLocalToolResponse returned error: %v", err)
	}
	if ok || resp != nil {
		t.Fatalf("tool-result follow-up must be sent upstream instead of preemptively re-calling the tool, ok=%v resp=%v", ok, resp)
	}
	snapshot := timing.Snapshot()
	if snapshot["tool_bridge_preemptive"] != false || snapshot["tool_bridge_preemptive_reason"] != "tool_result_present" {
		t.Fatalf("expected preemptive skip reason for tool result, got %#v", snapshot)
	}
}

func TestToolInstructionDoesNotTriggerImageHeuristic(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "读取 AGENTS.md"},
		},
		Tools: []dto.ToolCallRequest{{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        "ReadImageMetadata",
				Description: "reads image metadata from a local file",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	}
	prompt := buildChatPromptForRelay(req, nil)
	if strings.Contains(prompt, chatImageGenerationInstruction) {
		t.Fatalf("tool descriptions must not trigger image generation intent: %q", prompt)
	}
}

func TestBuildLatestChatGPTWebMessagePromptForRelaySendsOnlyLatestUser(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
			{Role: "user", Content: "follow up only"},
		},
	}

	got := buildLatestChatGPTWebMessagePromptForRelay(req, nil)
	if got != "follow up only" {
		t.Fatalf("expected only latest user message without role transcript, got %q", got)
	}
	for _, forbidden := range []string{"System:", "User:", "Assistant:", "first question", "first answer"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("latest-message prompt must not include %q: %q", forbidden, got)
		}
	}
}

func TestBuildLatestChatGPTWebMessagePromptForRelayIncludesTrailingToolResult(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "system", Content: "be concise"},
			{Role: "user", Content: "读取 /Users/zrf/project/git-project/aicustomer/AGENTS.md"},
			{Role: "assistant", Content: `[function_call] exec_command {"cmd":"cat 'AGENTS.md'"}`},
			{Role: "tool", ToolCallId: "call_1", Content: "# AGENTS.md\n项目规则"},
		},
		Tools: []dto.ToolCallRequest{{
			Type:     "function",
			Function: dto.FunctionRequest{Name: "exec_command"},
		}},
	}

	got := buildLatestChatGPTWebMessagePromptForRelay(req, nil)
	for _, want := range []string{
		"User request:",
		"读取 /Users/zrf/project/git-project/aicustomer/AGENTS.md",
		"Tool result call_1:",
		"# AGENTS.md\n项目规则",
		"Use the tool result above",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("latest tool-result prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "TOOL BRIDGE PROTOCOL") {
		t.Fatalf("tool-result follow-up should not ask the model to call a tool again by default:\n%s", got)
	}
	if strings.Contains(got, "System: be concise") || strings.Contains(got, "[function_call]") {
		t.Fatalf("latest tool-result prompt should remain incremental and omit old transcript noise:\n%s", got)
	}
}

func TestChatGPTWebSessionRouteKeyIsolatesChannelAndAccount(t *testing.T) {
	base := &relaycommon.RelayInfo{
		UserId:  100,
		TokenId: 200,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            10,
			ChannelMultiKeyIndex: 0,
			ApiKey:               "account-a",
		},
	}

	key1 := chatGPTWebSessionRouteKey(base, "client-session-1")
	key2 := chatGPTWebSessionRouteKey(base, "client-session-1")
	if key1 == "" || key1 != key2 {
		t.Fatalf("expected stable non-empty route key, got %q and %q", key1, key2)
	}
	if strings.Contains(key1, "client-session-1") || strings.Contains(key1, "account-a") {
		t.Fatalf("route key must be hashed and not expose raw session/account data: %q", key1)
	}

	otherChannel := *base
	otherChannelMeta := *base.ChannelMeta
	otherChannelMeta.ChannelId = 11
	otherChannel.ChannelMeta = &otherChannelMeta
	if got := chatGPTWebSessionRouteKey(&otherChannel, "client-session-1"); got == key1 {
		t.Fatalf("different ChatGPT Web channels must not share a session route key")
	}

	otherAccount := *base
	otherAccountMeta := *base.ChannelMeta
	otherAccountMeta.ApiKey = "account-b"
	otherAccount.ChannelMeta = &otherAccountMeta
	if got := chatGPTWebSessionRouteKey(&otherAccount, "client-session-1"); got == key1 {
		t.Fatalf("different ChatGPT Web accounts must not share a session route key")
	}
}

func TestResolveChatGPTWebSessionRouteUsesCachedConversation(t *testing.T) {
	resetChatGPTWebSessionRouteCacheForTest()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		resetChatGPTWebSessionRouteCacheForTest()
	})

	info := &relaycommon.RelayInfo{
		UserId:  100,
		TokenId: 200,
		RequestHeaders: map[string]string{
			"User-Agent": "claude-code/1.0",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            10,
			ChannelMultiKeyIndex: 0,
			ApiKey:               "account-a",
		},
	}
	routeKey := chatGPTWebSessionRouteKey(info, "client-session-1")
	recordChatGPTWebSessionRoute(chatGPTWebSessionRoute{
		Enabled: true,
		Key:     routeKey,
	}, "conv-123", nil)

	raw := []byte(`{"metadata":{"user_id":"{\"session_id\":\"client-session-1\"}"}}`)
	route := resolveChatGPTWebSessionRoute(info, chatRequest{Model: "claude-test"}, raw, nil)
	if !route.Enabled || !route.Reused || route.CachedConversationID != "conv-123" {
		t.Fatalf("expected cached conversation route, got %#v", route)
	}
}

func TestResolveChatGPTWebSessionRouteUsesMultipartPromptCacheKey(t *testing.T) {
	resetChatGPTWebSessionRouteCacheForTest()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		resetChatGPTWebSessionRouteCacheForTest()
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-2")
	_ = writer.WriteField("prompt", "edit this image")
	_ = writer.WriteField("prompt_cache_key", "multipart-client-session")
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	info := &relaycommon.RelayInfo{
		UserId:  100,
		TokenId: 200,
		RequestHeaders: map[string]string{
			"Content-Type": writer.FormDataContentType(),
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            10,
			ChannelMultiKeyIndex: 0,
			ApiKey:               "account-a",
		},
	}
	routeKey := chatGPTWebSessionRouteKey(info, "multipart-client-session")
	recordChatGPTWebSessionRoute(chatGPTWebSessionRoute{
		Enabled: true,
		Key:     routeKey,
	}, "conv-multipart", nil)

	route := resolveChatGPTWebSessionRoute(info, chatRequest{Model: "gpt-image-2"}, body.Bytes(), nil)
	if !route.Enabled || !route.Reused || route.CachedConversationID != "conv-multipart" {
		t.Fatalf("expected cached multipart conversation route, got %#v", route)
	}
}

func TestResolveChatGPTWebSessionRouteDoesNotReuseMessageSeedWithoutExplicitSessionKey(t *testing.T) {
	resetChatGPTWebSessionRouteCacheForTest()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		resetChatGPTWebSessionRouteCacheForTest()
	})

	info := &relaycommon.RelayInfo{
		UserId:  100,
		TokenId: 200,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelId:            10,
			ChannelMultiKeyIndex: 0,
			ApiKey:               "account-a",
		},
	}
	firstReq := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
		},
	}
	firstRoute := resolveChatGPTWebSessionRoute(info, firstReq, []byte(`{"model":"gpt-5.5-thinking"}`), nil)
	if firstRoute.Enabled || firstRoute.SessionSource != "" || firstRoute.Reused {
		t.Fatalf("expected request without explicit session key to skip route reuse, got %#v", firstRoute)
	}

	secondReq := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "hi"},
			{Role: "assistant", Content: "hello"},
			{Role: "user", Content: "continue"},
		},
	}
	secondRoute := resolveChatGPTWebSessionRoute(info, secondReq, []byte(`{"model":"gpt-5.5-thinking"}`), nil)
	if secondRoute.Enabled || secondRoute.Reused || secondRoute.CachedConversationID != "" {
		t.Fatalf("expected follow-up without explicit session key to skip route reuse, got %#v", secondRoute)
	}
}

func TestApplyChatGPTWebSessionRouteUsesLatestPromptAndFullFallback(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "first question"},
			{Role: "assistant", Content: "first answer"},
			{Role: "user", Content: "follow up only"},
		},
	}
	fullPrompt := buildChatPromptForRelay(req, nil)
	route := chatGPTWebSessionRoute{
		Enabled:              true,
		Reused:               true,
		CachedConversationID: "conv-123",
	}

	got := applyChatGPTWebSessionRoute(&req, &route, fullPrompt, nil)
	if got != "follow up only" {
		t.Fatalf("expected incremental prompt to use latest user message, got %q", got)
	}
	if req.ConversationID != "conv-123" {
		t.Fatalf("expected cached conversation id to be applied, got %q", req.ConversationID)
	}
	if req.FallbackPrompt != fullPrompt {
		t.Fatalf("expected full context fallback prompt, got %q want %q", req.FallbackPrompt, fullPrompt)
	}
	if !route.Incremental {
		t.Fatalf("expected route to be marked incremental")
	}
}

func TestStartChatStreamFallsBackToFullContextWhenCachedConversationForbidden(t *testing.T) {
	resetChatGPTWebSessionRouteCacheForTest()
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
		resetChatGPTWebSessionRouteCacheForTest()
	})

	routeKey := "route-for-stale-conv"
	recordChatGPTWebSessionRoute(chatGPTWebSessionRoute{
		Enabled: true,
		Key:     routeKey,
	}, "stale-conv", nil)

	var conversationPayloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/sentinel/chat-requirements/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"persona":"chatgpt","prepare_token":"prepare-token","turnstile":{"required":false},"proofofwork":{"required":false}}`))
		case "/backend-api/sentinel/chat-requirements/finalize":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"requirements-token","persona":"chatgpt"}`))
		case "/backend-api/conversation/stale-conv":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"current_node":"parent-stale","mapping":{}}`))
		case "/backend-api/f/conversation/prepare":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"conduit_token":"conduit-test"}`))
		case "/backend-api/f/conversation":
			var payload map[string]any
			body, _ := io.ReadAll(r.Body)
			if err := common.Unmarshal(body, &payload); err != nil {
				t.Fatalf("conversation payload is not json: %v", err)
			}
			conversationPayloads = append(conversationPayloads, payload)
			if payload["conversation_id"] == "stale-conv" {
				http.Error(w, `{"error":"conversation forbidden"}`, http.StatusForbidden)
				return
			}
			if _, exists := payload["conversation_id"]; exists {
				t.Fatalf("fallback request must start a fresh conversation, got %#v", payload["conversation_id"])
			}
			messages, _ := payload["messages"].([]any)
			if len(messages) != 1 {
				t.Fatalf("unexpected fallback messages: %#v", payload["messages"])
			}
			msg, _ := messages[0].(map[string]any)
			content, _ := msg["content"].(map[string]any)
			parts, _ := content["parts"].([]any)
			if len(parts) != 1 || parts[0] != "full context prompt" {
				t.Fatalf("fallback must send full context prompt, got %#v", parts)
			}
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
	req := chatRequest{
		Model:           "gpt-5.5-instant",
		ConversationID:  "stale-conv",
		FallbackPrompt:  "full context prompt",
		sessionRouteKey: routeKey,
	}
	timing := service.NewChatGPTWebTiming()
	started, err := startChatStream(context.Background(), client, req, "incremental prompt", nil, timing)
	if err != nil {
		t.Fatalf("startChatStream returned error: %v", err)
	}
	for range started.Stream {
	}
	if started.Prompt != "full context prompt" {
		t.Fatalf("expected started prompt to switch to full context, got %q", started.Prompt)
	}
	if len(conversationPayloads) != 2 {
		t.Fatalf("expected stale conversation attempt plus fallback attempt, got %d", len(conversationPayloads))
	}
	if snapshot := timing.Snapshot(); snapshot["session_route_retry_full_context"] != true || snapshot["session_route_retry_reason"] != "stream_403" {
		t.Fatalf("expected route fallback timing, got %#v", snapshot)
	}
	if _, found, err := getChatGPTWebSessionRouteCache().Get(routeKey); err != nil || found {
		t.Fatalf("expected stale session route cache to be cleared, found=%v err=%v", found, err)
	}
}

func TestResponsesTextPromptDoesNotTriggerImageHeuristic(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "分析为什么生成图片任务很慢"},
		},
	}

	prompt := buildChatPromptForRelay(req, info)
	if strings.Contains(prompt, chatImageGenerationInstruction) {
		t.Fatalf("responses text prompt must not inject image generation instruction: %q", prompt)
	}
	if shouldPollChatGeneratedImagesForRelay(info, req, prompt, "", false) {
		t.Fatalf("responses text prompt must not enable image polling heuristic: %q", prompt)
	}
}

func TestChatTextPromptStillTriggersImageHeuristic(t *testing.T) {
	req := chatRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "生成一张小猫图片"},
		},
	}

	prompt := buildChatPromptForRelay(req, nil)
	if !strings.Contains(prompt, chatImageGenerationInstruction) {
		t.Fatalf("chat prompt should still inject image generation instruction: %q", prompt)
	}
	if !shouldPollChatGeneratedImagesForRelay(nil, req, prompt, "", false) {
		t.Fatalf("chat prompt should still enable image polling heuristic: %q", prompt)
	}
}

func TestResponsesImageModelStillTriggersImagePolling(t *testing.T) {
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeResponses}
	req := chatRequest{
		Model: "gpt-image-2",
		Messages: []dto.Message{
			{Role: "user", Content: "生成一张小猫图片"},
		},
	}

	prompt := buildChatPromptForRelay(req, info)
	if !strings.Contains(prompt, chatImageGenerationInstruction) {
		t.Fatalf("responses image model should keep image generation instruction: %q", prompt)
	}
	if !shouldPollChatGeneratedImagesForRelay(info, req, prompt, "", false) {
		t.Fatalf("responses image model should enable image polling")
	}
	if !shouldPollChatGeneratedImagesForRelay(info, chatRequest{Model: "claude-test"}, "User: hello", "", true) {
		t.Fatalf("responses relay should poll when upstream explicitly reports image generation")
	}
}

func TestConvertOpenAIRequestAllowsChat(t *testing.T) {
	stream := false
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &dto.GeneralOpenAIRequest{
		Model:           "gpt-5.5-thinking",
		Stream:          &stream,
		ReasoningEffort: "medium",
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
	if req.Model != "gpt-5.5-thinking" || len(req.Messages) != 1 || req.Stream == nil || *req.Stream {
		t.Fatalf("unexpected converted request: %#v", req)
	}
	if req.ThinkingEffort != "standard" {
		t.Fatalf("expected ChatGPT Web thinking effort to default to standard, got %q", req.ThinkingEffort)
	}
}

func TestConvertOpenAIRequestReadsChatGPTWebDeepResearchFlag(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions",
		bytes.NewReader([]byte(`{"chatgpt_web_deep_research":true}`)),
	)

	converted, err := (&Adaptor{}).ConvertOpenAIRequest(c, nil, &dto.GeneralOpenAIRequest{
		Model: "gpt-5.5-thinking",
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
	if !req.DeepResearch {
		t.Fatalf("expected deep research flag to be extracted from raw request body")
	}
}

func TestConvertOpenAIRequestPreservesToolsForChatGPTWeb(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertOpenAIRequest(nil, nil, &dto.GeneralOpenAIRequest{
		Model: "gpt-5.5-thinking",
		Messages: []dto.Message{
			{Role: "user", Content: "read a file"},
		},
		Tools: []dto.ToolCallRequest{{
			Type: "function",
			Function: dto.FunctionRequest{
				Name:        "Read",
				Description: "read local file",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIRequest returned error: %v", err)
	}
	req, ok := converted.(chatRequest)
	if !ok {
		t.Fatalf("expected chatRequest, got %T", converted)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "Read" {
		t.Fatalf("expected tools to be preserved, got %#v", req.Tools)
	}
}

func TestConvertOpenAIResponsesRequestAllowsChat(t *testing.T) {
	stream := true
	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5-thinking"},
	}, dto.OpenAIResponsesRequest{
		Model:              "gpt-5.5-thinking",
		Instructions:       json.RawMessage(`"be concise"`),
		Input:              json.RawMessage(`[{"role":"user","content":[{"type":"input_text","text":"hello"},{"type":"input_image","image_url":"https://example.com/cat.png"}]}]`),
		Stream:             &stream,
		Text:               json.RawMessage(`{"format":{"type":"json_object"}}`),
		Tools:              json.RawMessage(`[{"type":"function","name":"Read","description":"read local file","parameters":{"type":"object","properties":{"file_path":{"type":"string"}},"required":["file_path"]}}]`),
		Reasoning:          &dto.Reasoning{Effort: "high"},
		PreviousResponseID: "resp_chatgptimg-conv-123",
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}
	req, ok := converted.(chatRequest)
	if !ok {
		t.Fatalf("expected chatRequest, got %T", converted)
	}
	if req.Model != "gpt-5.5-thinking" || req.Stream == nil || !*req.Stream {
		t.Fatalf("unexpected converted request basics: %#v", req)
	}
	if req.ConversationID != "conv-123" {
		t.Fatalf("expected previous response id to recover conversation id, got %q", req.ConversationID)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Fatalf("expected responses text format to map to chat response_format, got %#v", req.ResponseFormat)
	}
	if req.ThinkingEffort != "extended" {
		t.Fatalf("expected responses reasoning effort to map to ChatGPT Web extended, got %q", req.ThinkingEffort)
	}
	if len(req.Tools) != 1 || req.Tools[0].Function.Name != "Read" {
		t.Fatalf("expected responses tools to map to chat tools, got %#v", req.Tools)
	}
	if len(req.Messages) != 2 {
		t.Fatalf("expected system + user messages, got %#v", req.Messages)
	}
	prompt := buildChatPrompt(req)
	if !strings.Contains(prompt, "System: be concise") ||
		!strings.Contains(prompt, "User: hello") ||
		!strings.Contains(prompt, "[image_url: https://example.com/cat.png]") {
		t.Fatalf("unexpected prompt from converted responses request: %q", prompt)
	}
}

func TestParseChatGPTWebToolCallFromJSONFence(t *testing.T) {
	tools := []dto.ToolCallRequest{{
		Type:     "function",
		Function: dto.FunctionRequest{Name: "Read"},
	}}
	content := "```json\n{\"tool_call\":{\"name\":\"read\",\"arguments\":{\"file_path\":\"/etc/hostname\"}}}\n```"
	call, ok := parseChatGPTWebToolCall(content, tools)
	if !ok {
		t.Fatalf("expected tool call to be parsed")
	}
	if call.Function.Name != "Read" || !strings.Contains(call.Function.Arguments, `"/etc/hostname"`) {
		t.Fatalf("unexpected parsed tool call: %#v", call)
	}
}

func TestChatModelForWebUsesChatGPTWebSlug(t *testing.T) {
	cases := []struct {
		model string
		want  string
	}{
		{model: "", want: "auto"},
		{model: "gpt-image-2", want: "auto"},
		{model: "gpt-5.5-thinking", want: "gpt-5-5-thinking"},
		{model: "gpt-5.5-pro", want: "gpt-5-5-pro"},
		{model: "gpt-5.4-thinking", want: "gpt-5-4-thinking"},
		{model: "gpt-5.4-pro", want: "gpt-5-4-pro"},
		{model: "gpt-5.4-instant", want: "gpt-5-4-instant"},
		{model: "custom-web-model", want: "custom-web-model"},
	}

	for _, tc := range cases {
		t.Run(tc.model, func(t *testing.T) {
			if got := chatModelForWeb(tc.model); got != tc.want {
				t.Fatalf("chatModelForWeb(%q) = %q, want %q", tc.model, got, tc.want)
			}
		})
	}
}

func TestChatGPTWebThinkingEffortOnlyForThinkingModels(t *testing.T) {
	cases := []struct {
		name      string
		model     string
		requested string
		want      string
	}{
		{name: "empty defaults to standard", model: "gpt-5-5-thinking", requested: "", want: "standard"},
		{name: "medium stays standard", model: "gpt-5-5-thinking", requested: "medium", want: "standard"},
		{name: "standard stays standard", model: "gpt-5-5-thinking", requested: "standard", want: "standard"},
		{name: "high maps to extended", model: "gpt-5-5-thinking", requested: "high", want: "extended"},
		{name: "xhigh maps to extended", model: "gpt-5-5-thinking", requested: "xhigh", want: "extended"},
		{name: "extended stays extended", model: "gpt-5-5-thinking", requested: "extended", want: "extended"},
		{name: "non thinking model omits effort", model: "gpt-5-5-pro", requested: "high", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := chatGPTWebThinkingEffort(tc.model, tc.requested); got != tc.want {
				t.Fatalf("chatGPTWebThinkingEffort(%q, %q) = %q, want %q", tc.model, tc.requested, got, tc.want)
			}
		})
	}
}

func TestDoResponseResponsesRelayUsesResponsesHandler(t *testing.T) {
	payload := buildResponsesResponse("resp_chatgptimg-conv-1", "msg_chatgptimg-conv-1", time.Now().Unix(), "gpt-5.5-thinking", "hello", dto.Usage{
		PromptTokens:     2,
		CompletionTokens: 3,
		TotalTokens:      5,
	})
	body, err := common.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal response payload failed: %v", err)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	usage, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}, &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5-thinking"},
	})
	if apiErr != nil {
		t.Fatalf("DoResponse returned error: %v", apiErr)
	}
	gotUsage, ok := usage.(*dto.Usage)
	if !ok {
		t.Fatalf("expected *dto.Usage, got %T", usage)
	}
	if gotUsage.PromptTokens != 2 || gotUsage.CompletionTokens != 3 || gotUsage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %#v", gotUsage)
	}
	out := w.Body.String()
	if !strings.Contains(out, `"object":"response"`) || strings.Contains(out, `"chat.completion"`) {
		t.Fatalf("expected responses-compatible body, got %s", out)
	}
}

func TestStreamResponsesCompletionEmitsResponsesEvents(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["hello"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	pr, pw := io.Pipe()
	go streamResponsesCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test"}, "hello", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read responses stream output failed: %v", err)
	}
	body := string(out)
	for _, want := range []string{
		`"type":"response.created"`,
		`"type":"response.output_text.delta"`,
		`"type":"response.completed"`,
		`"id":"resp_chatgptimg-conv-1"`,
		`data: [DONE]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("responses stream missing %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, "chat.completion.chunk") {
		t.Fatalf("responses stream must not emit chat completion chunks:\n%s", body)
	}
}

func TestStreamResponsesCompletionConvertsToolJSONToFunctionCall(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["{\"tool_call\":{\"name\":\"Read\",\"arguments\":{\"file_path\":\"/etc/hostname\"}}}"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	req := chatRequest{
		Model: "claude-test",
		Tools: []dto.ToolCallRequest{{
			Type:     "function",
			Function: dto.FunctionRequest{Name: "Read"},
		}},
	}

	pr, pw := io.Pipe()
	go streamResponsesCompletion(context.Background(), nil, stream, req, "read file", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read responses stream output failed: %v", err)
	}
	body := string(out)
	for _, want := range []string{
		`"type":"function_call"`,
		`"name":"Read"`,
		`"type":"response.function_call_arguments.delta"`,
		`"type":"response.completed"`,
		`data: [DONE]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("responses tool stream missing %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `"type":"response.output_text.delta"`) {
		t.Fatalf("tool stream must not leak structured JSON as text:\n%s", body)
	}
}

func TestStreamChatCompletionUsesSSEImageRefsAsConversationPollHint(t *testing.T) {
	service.InitTokenEncoders()
	var mappingRequests int
	var directSSEDownloadRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/backend-api/files/file_generated/download":
			directSSEDownloadRequests++
			http.Error(w, `{"error":"sse ref is web-internal"}`, http.StatusNotFound)
		case "/backend-api/conversation/conv-1":
			mappingRequests++
			_, _ = w.Write([]byte(`{"mapping":{"node-1":{"message":{"author":{"role":"tool","name":"image_gen"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://file_polled"}]}}}},"current_node":"node-1"}`))
		case "/backend-api/files/file_polled/download":
			_, _ = w.Write([]byte(`{"download_url":"` + "http://" + r.Host + `/image.png"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"tool","name":"image_gen"},"metadata":{"async_task_type":"image_gen"},"content":{"content_type":"multimodal_text","parts":[{"asset_pointer":"file-service://file_generated"}]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	client := &Client{opts: ClientOptions{BaseURL: server.URL}, hc: server.Client()}
	pr, pw := io.Pipe()
	req := chatRequest{Model: "gpt-5.5-pro", Messages: []dto.Message{{Role: "user", Content: "生成一张小猫图片"}}}
	go streamChatCompletion(context.Background(), client, stream, req, "User: 生成一张小猫图片", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read stream output failed: %v", err)
	}
	body := string(out)
	if !strings.Contains(body, server.URL+"/image.png") {
		t.Fatalf("expected stream to contain polled image markdown, got:\n%s", body)
	}
	if mappingRequests == 0 {
		t.Fatal("expected SSE refs to trigger conversation image collection")
	}
	if directSSEDownloadRequests != 0 {
		t.Fatalf("expected web-internal SSE ref not to be materialized directly, got %d direct requests", directSSEDownloadRequests)
	}
}

func TestStreamChatCompletionUsesRealConversationIDOnly(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["hello"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	pr, pw := io.Pipe()
	go streamChatCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test"}, "hello", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

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

func TestStreamChatCompletionSuppressesDeepResearchInternalPayload(t *testing.T) {
	stream := make(chan SSEEvent, 4)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Event: "delta", Data: []byte(`{"p":"/message/content/parts/0","o":"append","v":"{\"path\":\"/Deep Research App/implicit_link::connector_openai_deep_research/start\",\"args\":{\"user\":\"alice\"}}"}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	pr, pw := io.Pipe()
	go streamChatCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test", DeepResearch: true}, "做深度研究", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read stream output failed: %v", err)
	}
	body := string(out)
	if strings.Contains(body, "implicit_link::connector_openai_deep_research") || strings.Contains(body, "Deep Research App") {
		t.Fatalf("deep research internal payload leaked into stream:\n%s", body)
	}
	if !strings.Contains(body, "深度研究任务已启动") {
		t.Fatalf("expected user-facing deep research pending message, got:\n%s", body)
	}
}

func TestStreamChatCompletionSuppressesDeepResearchInternalSnapshotPayload(t *testing.T) {
	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-1"}`)}
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["{\"path\":\"/Deep Research App/implicit_link::connector_openai_deep_research/start\",\"args\":{\"user"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	pr, pw := io.Pipe()
	go streamChatCompletion(context.Background(), nil, stream, chatRequest{Model: "claude-test", DeepResearch: true}, "做深度研究", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read stream output failed: %v", err)
	}
	body := string(out)
	if strings.Contains(body, "implicit_link::connector_openai_deep_research") || strings.Contains(body, "Deep Research App") {
		t.Fatalf("deep research internal snapshot payload leaked into stream:\n%s", body)
	}
	if !strings.Contains(body, "深度研究任务已启动") {
		t.Fatalf("expected user-facing deep research pending message, got:\n%s", body)
	}
}

func TestRecoverChatCompletionTextWithFetcherWaitsForDeepResearchResult(t *testing.T) {
	attempts := 0
	timing := service.NewChatGPTWebTiming()
	got := recoverChatCompletionTextWithFetcher(context.Background(), func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", nil
		}
		return "最终深度研究结果", nil
	}, time.Second, time.Millisecond, timing)

	if got != "最终深度研究结果" {
		t.Fatalf("expected delayed deep research result, got %q", got)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 polling attempts, got %d", attempts)
	}
	snapshot := timing.Snapshot()
	if snapshot["chat_text_mapping_recovered"] != true {
		t.Fatalf("expected timing to mark recovery, got %#v", snapshot)
	}
}

func TestRecoverChatCompletionTextWithFetcherRetriesTransientMappingError(t *testing.T) {
	attempts := 0
	got := recoverChatCompletionTextWithFetcher(context.Background(), func(context.Context) (string, error) {
		attempts++
		if attempts == 1 {
			return "", errors.New("chatgpt upstream 404: conversation get failed")
		}
		return "handoff recovered text", nil
	}, time.Second, time.Millisecond, nil)

	if got != "handoff recovered text" {
		t.Fatalf("expected recovered text after transient mapping error, got %q", got)
	}
	if attempts != 2 {
		t.Fatalf("expected 2 polling attempts, got %d", attempts)
	}
}

func TestRecoverChatCompletionTextFromConversationExtractsDeepResearchWidgetReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation/conv-deep-research" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"current_node": "assistant-empty",
			"mapping": {
				"widget-state": {
					"message": {
						"author": {"role":"tool","name":"api_tool.widget_state"},
						"content": {"parts": ["The latest state of the widget is: {\"status\":\"completed\",\"report_message\":{\"author\":{\"role\":\"assistant\"},\"update_time\":2,\"content\":{\"content_type\":\"text\",\"parts\":[\"最终深度研究报告\"]}}}"]}
					}
				},
				"assistant-empty": {
					"parent": "widget-state",
					"message": {"author":{"role":"assistant"},"content":{"parts":[""]}}
				}
			}
		}`))
	}))
	defer server.Close()

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}

	got := recoverChatCompletionTextFromConversationWithWait(context.Background(), client, "conv-deep-research", 0, 0)
	if got != "最终深度研究报告" {
		t.Fatalf("expected report recovered from widget state, got %q", got)
	}
}

func TestStreamChatCompletionRecoversTextAfterStreamHandoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/backend-api/conversation/conv-handoff" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"current_node": "assistant-1",
			"mapping": {
				"assistant-1": {
					"message": {
						"author": {"role":"assistant"},
						"content": {"parts": ["handoff recovered text"]}
					}
				}
			}
		}`))
	}))
	defer server.Close()

	stream := make(chan SSEEvent, 3)
	stream <- SSEEvent{Data: []byte(`{"type":"resume_conversation_token","conversation_id":"conv-handoff"}`)}
	stream <- SSEEvent{Data: []byte(`{"type":"stream_handoff","conversation_id":"conv-handoff","turn_exchange_id":"turn-1","options":[{"type":"resume_sse_endpoint","topic_id":"conversation-turn-1"}]}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	client := &Client{
		opts: ClientOptions{BaseURL: server.URL},
		hc:   server.Client(),
	}
	pr, pw := io.Pipe()
	go streamChatCompletion(context.Background(), client, stream, chatRequest{Model: "claude-test"}, "hello", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read stream output failed: %v", err)
	}
	if !strings.Contains(string(out), "handoff recovered text") {
		t.Fatalf("expected recovered handoff text in stream, got:\n%s", string(out))
	}
}

func TestStreamChatCompletionConvertsToolJSONToToolCalls(t *testing.T) {
	stream := make(chan SSEEvent, 2)
	stream <- SSEEvent{Data: []byte(`{"v":{"conversation_id":"conv-1","message":{"author":{"role":"assistant"},"content":{"content_type":"text","parts":["{\"tool_call\":{\"name\":\"Read\",\"arguments\":{\"file_path\":\"/etc/hostname\"}}}"]}}}}`)}
	stream <- SSEEvent{Data: []byte(`[DONE]`)}
	close(stream)

	req := chatRequest{
		Model: "claude-test",
		Tools: []dto.ToolCallRequest{{
			Type:     "function",
			Function: dto.FunctionRequest{Name: "Read"},
		}},
	}

	pr, pw := io.Pipe()
	go streamChatCompletion(context.Background(), nil, stream, req, "read file", imageBaseline{}, nil, "", chatGPTWebSessionRoute{}, pw)

	out, err := io.ReadAll(pr)
	if err != nil {
		t.Fatalf("read chat stream output failed: %v", err)
	}
	body := string(out)
	for _, want := range []string{
		`"tool_calls"`,
		`"name":"Read"`,
		`"finish_reason":"tool_calls"`,
		`data: [DONE]`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("chat tool stream missing %s:\n%s", want, body)
		}
	}
	if strings.Contains(body, `"content":"{\"tool_call\"`) {
		t.Fatalf("tool stream must not leak structured JSON as text:\n%s", body)
	}
}

func TestChatImageRefsFromSSEStateFiltersBaseline(t *testing.T) {
	state := &ChatSSEState{
		FileIDs:     []string{"old_file", "new_file", "new_file"},
		SedimentIDs: []string{"old_sed", "new_sed", "new_sed"},
	}
	refs := chatImageRefsFromSSEState(state, imageBaseline{
		FileIDs:     map[string]struct{}{"old_file": {}},
		SedimentIDs: map[string]struct{}{"old_sed": {}},
	})
	want := []string{"new_file", "sed:new_sed"}
	if len(refs) != len(want) {
		t.Fatalf("expected refs %#v, got %#v", want, refs)
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("expected refs %#v, got %#v", want, refs)
		}
	}
}

func TestChatGeneratedImagePollMaxWaitIsThirtySeconds(t *testing.T) {
	if chatGPTWebChatImagePollMaxWait != 30*time.Second {
		t.Fatalf("expected chat image poll max wait to be 30s, got %s", chatGPTWebChatImagePollMaxWait)
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
	markdown, err := collectChatGeneratedImageMarkdown(context.Background(), client, "conv-text", imageBaseline{}, false, false, nil, "", "", "")
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
	catRequest := chatRequest{
		Model:    "gpt-5.5-pro",
		Messages: []dto.Message{{Role: "user", Content: "帮我生成图片：一只小猫"}},
	}
	if !shouldPollChatGeneratedImages(catRequest, "User: 帮我生成图片：一只小猫", "", false) {
		t.Fatal("expected Chinese image generation intent to enable polling")
	}

	portraitRequest := chatRequest{
		Model:    "gpt-5.5-pro",
		Messages: []dto.Message{{Role: "user", Content: "生成一张性感美女图片"}},
	}
	if !shouldPollChatGeneratedImages(portraitRequest, "User: 生成一张性感美女图片", "", false) {
		t.Fatal("expected Chinese generate-a-picture intent to enable polling")
	}

	textRequest := chatRequest{
		Model:    "gpt-5.5-pro",
		Messages: []dto.Message{{Role: "user", Content: "hello"}},
	}
	if shouldPollChatGeneratedImages(textRequest, "User: hello", "hello", false) {
		t.Fatal("expected plain text chat to skip image polling")
	}
}

func TestShouldPollChatGeneratedImagesIgnoresAssistantTextIntent(t *testing.T) {
	req := chatRequest{Model: "gpt-5.5-instant", Messages: []dto.Message{{Role: "user", Content: "hello"}}}
	content := "我可以帮你生成图片，也可以解释这类任务为什么会变慢。"
	if shouldPollChatGeneratedImages(req, "User: hello", content, false) {
		t.Fatal("assistant text mentioning image generation must not trigger image polling")
	}
	if !shouldPollChatGeneratedImages(req, "User: hello", content, true) {
		t.Fatal("explicit upstream image-generation marker must still trigger image polling")
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
		{name: "public absolute", ref: "https://pazom.xyz/pg/public/images/generations/task_domain/image/0", wantTask: "task_domain", wantIndex: 0, wantOK: true},
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
	c := newChatGPTImgTestContext("http://legacy.example.com/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "legacy.example.com",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "8999",
	})

	if got := requestPublicBaseURL(c); got != "http://legacy.example.com:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLDoesNotDuplicateExistingPort(t *testing.T) {
	c := newChatGPTImgTestContext("http://legacy.example.com/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "legacy.example.com:8999",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "80",
	})

	if got := requestPublicBaseURL(c); got != "http://legacy.example.com:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLUsesRFCForwardedHost(t *testing.T) {
	c := newChatGPTImgTestContext("http://internal.local/v1/images/generations", map[string]string{
		"Forwarded": "for=127.0.0.1;proto=http;host=legacy.example.com:8999",
	})

	if got := requestPublicBaseURL(c); got != "http://legacy.example.com:8999" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLPrefersConfiguredServerAddress(t *testing.T) {
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://pazom.xyz/"
	t.Cleanup(func() { system_setting.ServerAddress = oldServerAddress })

	c := newChatGPTImgTestContext("http://legacy.example.com/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "legacy.example.com:8999",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "8999",
	})

	if got := requestPublicBaseURL(c); got != "https://pazom.xyz" {
		t.Fatalf("unexpected public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLForImagesUsesConfiguredServerAddressInPlayground(t *testing.T) {
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = "https://pazom.xyz/"
	t.Cleanup(func() { system_setting.ServerAddress = oldServerAddress })

	c := newChatGPTImgTestContext("http://legacy.example.com/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "legacy.example.com:8999",
		"X-Forwarded-Proto": "http",
		"X-Forwarded-Port":  "8999",
	})

	got := requestPublicBaseURLForImages(c, &relaycommon.RelayInfo{IsPlayground: true})
	if got != "https://pazom.xyz" {
		t.Fatalf("unexpected playground public base URL: %q", got)
	}
}

func TestRequestPublicBaseURLOmitsDefaultHTTPSPort(t *testing.T) {
	oldServerAddress := system_setting.ServerAddress
	system_setting.ServerAddress = ""
	t.Cleanup(func() { system_setting.ServerAddress = oldServerAddress })

	c := newChatGPTImgTestContext("https://pazom.xyz/v1/images/generations", map[string]string{
		"X-Forwarded-Host":  "pazom.xyz",
		"X-Forwarded-Proto": "https",
		"X-Forwarded-Port":  "443",
	})

	if got := requestPublicBaseURL(c); got != "https://pazom.xyz" {
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

func TestConvertImageRequestDefaultsImageEditsToB64JSON(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertImageRequest(
		nil,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits},
		dto.ImageRequest{
			Model:  "chatgpt-image-2",
			Prompt: "edit this image",
		},
	)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}
	got, ok := converted.(generationRequest)
	if !ok {
		t.Fatalf("expected generationRequest, got %T", converted)
	}
	if got.ResponseFormat != "b64_json" {
		t.Fatalf("expected image edits default response_format b64_json, got %q", got.ResponseFormat)
	}
}

func TestConvertImageRequestDefaultsImageGenerationsToB64JSON(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertImageRequest(
		nil,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations},
		dto.ImageRequest{
			Model:  "chatgpt-image-2",
			Prompt: "draw an image",
		},
	)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}
	got, ok := converted.(generationRequest)
	if !ok {
		t.Fatalf("expected generationRequest, got %T", converted)
	}
	if got.ResponseFormat != "b64_json" {
		t.Fatalf("expected image generations default response_format b64_json, got %q", got.ResponseFormat)
	}
}

func TestConvertImageRequestKeepsExplicitImageEditsResponseFormat(t *testing.T) {
	converted, err := (&Adaptor{}).ConvertImageRequest(
		nil,
		&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesEdits},
		dto.ImageRequest{
			Model:          "other-image-model",
			Prompt:         "edit this image",
			ResponseFormat: "url",
		},
	)
	if err != nil {
		t.Fatalf("ConvertImageRequest returned error: %v", err)
	}
	got, ok := converted.(generationRequest)
	if !ok {
		t.Fatalf("expected generationRequest, got %T", converted)
	}
	if got.ResponseFormat != "url" {
		t.Fatalf("expected explicit response_format url to be preserved, got %q", got.ResponseFormat)
	}
}

func TestConvertImageRequestForcesGPTImage2ToB64JSON(t *testing.T) {
	for _, modelName := range []string{"gpt-image-2", "chatgpt-image-2"} {
		t.Run(modelName, func(t *testing.T) {
			converted, err := (&Adaptor{}).ConvertImageRequest(
				nil,
				&relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations},
				dto.ImageRequest{
					Model:          modelName,
					Prompt:         "draw an image",
					ResponseFormat: "url",
				},
			)
			if err != nil {
				t.Fatalf("ConvertImageRequest returned error: %v", err)
			}
			got, ok := converted.(generationRequest)
			if !ok {
				t.Fatalf("expected generationRequest, got %T", converted)
			}
			if got.ResponseFormat != "b64_json" {
				t.Fatalf("expected %s to force response_format b64_json, got %q", modelName, got.ResponseFormat)
			}
		})
	}
}

func TestNormalizeGenerationRequestForcesPassThroughGPTImage2ToB64JSON(t *testing.T) {
	req := generationRequest{
		Model:          "gpt-image-2",
		Prompt:         "draw an image",
		ResponseFormat: "url",
	}

	normalizeGenerationRequestModelAndResponseFormat(&req, nil)

	if req.ResponseFormat != "b64_json" {
		t.Fatalf("expected pass-through gpt-image-2 response_format b64_json, got %q", req.ResponseFormat)
	}
}

func TestNormalizeGenerationRequestUsesUpstreamModelForB64JSONForce(t *testing.T) {
	req := generationRequest{
		Prompt:         "draw an image",
		ResponseFormat: "url",
	}

	normalizeGenerationRequestModelAndResponseFormat(&req, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-image-2",
		},
	})

	if req.Model != "gpt-image-2" {
		t.Fatalf("expected upstream model to populate request model, got %q", req.Model)
	}
	if req.ResponseFormat != "b64_json" {
		t.Fatalf("expected upstream gpt-image-2 response_format b64_json, got %q", req.ResponseFormat)
	}
}

func TestNormalizeGenerationRequestUsesMappedUpstreamModelForB64JSONForce(t *testing.T) {
	req := generationRequest{
		Model:          "customer-facing-image-alias",
		Prompt:         "draw an image",
		ResponseFormat: "url",
	}

	normalizeGenerationRequestModelAndResponseFormat(&req, &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-image-2",
		},
	})

	if req.ResponseFormat != "b64_json" {
		t.Fatalf("expected mapped upstream gpt-image-2 response_format b64_json, got %q", req.ResponseFormat)
	}
}

func TestNormalizeGenerationRequestDefaultsWithNilChannelMeta(t *testing.T) {
	req := generationRequest{Prompt: "draw an image"}

	normalizeGenerationRequestModelAndResponseFormat(&req, &relaycommon.RelayInfo{})

	if req.Model != ModelList[0] {
		t.Fatalf("expected default model %q, got %q", ModelList[0], req.Model)
	}
	if req.ResponseFormat != "b64_json" {
		t.Fatalf("expected default response_format b64_json, got %q", req.ResponseFormat)
	}
}

func TestChatGPTWebImageConversationModelUsesRequestModel(t *testing.T) {
	if got := chatGPTWebImageConversationModel(generationRequest{Model: "gpt-image-2"}); got != "gpt-image-2" {
		t.Fatalf("expected gpt-image-2 to be sent upstream, got %q", got)
	}
	if got := chatGPTWebImageConversationModel(generationRequest{Model: "auto"}); got != ModelList[0] {
		t.Fatalf("expected auto to fall back to default image model %q, got %q", ModelList[0], got)
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

func TestShouldPollTextOnlyImageSSEAllowsSearchFallback(t *testing.T) {
	if !shouldPollTextOnlyImageSSE(ImageSSEResult{
		ConversationID: "conv-1",
		Content:        `search("三唔识七，乱噏廿四 视频合集封面")`,
	}) {
		t.Fatal("expected search-like text-only image SSE to allow conversation polling")
	}
	if shouldPollTextOnlyImageSSE(ImageSSEResult{
		ConversationID: "conv-1",
		Content:        "I cannot generate that image.",
	}) {
		t.Fatal("expected refusal text to fail without polling")
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

func TestBuildGenerationResponseB64JSONOmitsURL(t *testing.T) {
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
	resp, err := buildGenerationResponse(context.Background(), client, generationRequest{
		ResponseFormat: "b64_json",
	}, &imageRunResult{
		ConversationID: "conv-1",
		SignedURLs:     []string{server.URL + "/image.png"},
	}, false, nil, "")
	if err != nil {
		t.Fatalf("buildGenerationResponse returned error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one image item, got %#v", resp.Data)
	}
	if resp.Data[0].Url != "" {
		t.Fatalf("expected b64_json response to omit url, got %q", resp.Data[0].Url)
	}
	if resp.Data[0].B64Json == "" {
		t.Fatal("expected b64_json to be populated")
	}
}

func TestBuildGenerationResponseGPTImage2ForcesB64JSONEvenWhenRequestFormatURL(t *testing.T) {
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
	resp, err := buildGenerationResponse(context.Background(), client, generationRequest{
		Model:          "gpt-image-2",
		Prompt:         "draw an image",
		ResponseFormat: "url",
	}, &imageRunResult{
		ConversationID: "conv-1",
		SignedURLs:     []string{server.URL + "/image.png"},
	}, false, nil, "")
	if err != nil {
		t.Fatalf("buildGenerationResponse returned error: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected one image item, got %#v", resp.Data)
	}
	if resp.Data[0].Url != "" {
		t.Fatalf("expected gpt-image-2 response to omit url even when request format is url, got %q", resp.Data[0].Url)
	}
	if resp.Data[0].B64Json != base64.StdEncoding.EncodeToString(imageBytes) {
		t.Fatalf("expected base64 image bytes, got %q", resp.Data[0].B64Json)
	}
}

func TestBuildGenerationResponseB64JSONMarshalOmitsURLField(t *testing.T) {
	resp := generationResponse{
		Data: []dto.ImageData{{
			B64Json: base64.StdEncoding.EncodeToString([]byte("image")),
		}},
	}

	payload, err := common.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if strings.Contains(string(payload), `"url"`) {
		t.Fatalf("expected b64_json payload to omit url field, got %s", payload)
	}
	if !strings.Contains(string(payload), `"b64_json"`) {
		t.Fatalf("expected b64_json field, got %s", payload)
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
	stored, ok := common.GetContextKeyType[*dto.ImageResponse](c, constant.ContextKeyImageGenerationResponse)
	if !ok || stored == nil || len(stored.Data) != 1 || stored.Data[0].Url != "https://example.com/image.png" {
		t.Fatalf("expected stream image response to be captured for drawing log, got ok=%v stored=%#v", ok, stored)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("expected SSE content-type, got %q", got)
	}
	if !strings.Contains(recorder.Body.String(), "data: [DONE]") {
		t.Fatalf("expected DONE marker in downstream SSE, got %q", recorder.Body.String())
	}
}
