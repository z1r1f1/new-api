package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

func TestOaiResponsesToChatStreamHandlerClaudeSendsTerminalEventsOnDone(t *testing.T) {
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/message?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		IsStream:          true,
		OriginModelName:   "gpt-5.5",
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}
	if usage == nil {
		t.Fatal("expected usage fallback to be returned")
	}

	body := recorder.Body.String()
	for _, want := range []string{
		"event: message_start",
		"event: content_block_start",
		"event: content_block_delta",
		"event: content_block_stop",
		"event: message_delta",
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestOaiResponsesToChatStreamHandlerClaudeSendsTerminalEventsOnCompletedWithoutUsage(t *testing.T) {
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/message?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"model":"gpt-5.5","created_at":123}}`,
		``,
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		``,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5","created_at":123}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		IsStream:          true,
		OriginModelName:   "gpt-5.5",
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}

	body := recorder.Body.String()
	for _, want := range []string{"event: content_block_stop", "event: message_delta", "event: message_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestOaiResponsesToChatStreamHandlerClaudeSendsTerminalEventsWithCompletedUsage(t *testing.T) {
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/message?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	usageJSON, _ := common.Marshal(dto.Usage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5})
	streamBody := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		``,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5","created_at":123,"usage":` + string(usageJSON) + `}}`,
		``,
	}, "\n")
	resp := &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(streamBody))}
	info := &relaycommon.RelayInfo{RelayFormat: types.RelayFormatClaude, IsStream: true, OriginModelName: "gpt-5.5", ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"}, ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone}}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}
	body := recorder.Body.String()
	for _, want := range []string{"event: content_block_stop", "event: message_delta", "event: message_stop"} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestOaiResponsesToChatStreamHandlerClaudeUsesMessageItemDoneText(t *testing.T) {
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"model":"gpt-5.5","created_at":123}}`,
		``,
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress","role":"assistant","content":[{"type":"output_text","text":"","annotations":[]}]}}`,
		``,
		`event: response.output_item.done`,
		`data: {"type":"response.output_item.done","output_index":0,"item":{"type":"message","id":"msg_1","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello from item done","annotations":[]}]}}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5","created_at":123}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		IsStream:          true,
		OriginModelName:   "gpt-5.5",
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}

	body := recorder.Body.String()
	for _, want := range []string{
		"event: content_block_delta",
		"Hello from item done",
		"event: message_stop",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestOaiResponsesToChatStreamHandlerClaudeKeepsToolCallsAfterText(t *testing.T) {
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`data: {"type":"response.output_text.delta","delta":"我先读取目录。"}`,
		``,
		`data: {"type":"response.output_item.added","item":{"type":"function_call","id":"item_1","call_id":"call_1","name":"list_files","arguments":""}}`,
		``,
		`data: {"type":"response.function_call_arguments.delta","item_id":"item_1","delta":"{\"path\":\".\"}"}`,
		``,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5","created_at":123,"usage":{"input_tokens":30,"output_tokens":5,"total_tokens":35}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		IsStream:          true,
		OriginModelName:   "gpt-5.5",
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}

	body := recorder.Body.String()
	for _, want := range []string{
		`"type":"text"`,
		`"type":"tool_use"`,
		`"name":"list_files"`,
		`"partial_json":"{\"path\":\".\"}"`,
		`"stop_reason":"tool_use"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestOaiResponsesToChatStreamHandlerClaudeConvertsToolCallJSONText(t *testing.T) {
	constant.StreamingTimeout = 30
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	usageJSON, _ := common.Marshal(dto.Usage{InputTokens: 30, OutputTokens: 5, TotalTokens: 35})
	streamBody := strings.Join([]string{
		`event: response.created`,
		`data: {"type":"response.created","response":{"model":"gpt-5.5-thinking","created_at":123}}`,
		``,
		`event: response.output_item.added`,
		`data: {"type":"response.output_item.added","output_index":0,"item":{"type":"message","id":"msg_1","status":"in_progress","role":"assistant","content":[{"type":"output_text","text":"","annotations":[]}]}}`,
		``,
		`event: response.output_text.delta`,
		`data: {"type":"response.output_text.delta","delta":"{\"tool_call\":{\"name\":\"exec_command\",\"arguments\":{\"cmd\":\"cat AGENTS.md\",\"workdir\":\"/Users/zrf/project/git-project/aicustomer\",\"max_output_tokens\":20000}}}"}`,
		``,
		`event: response.completed`,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5-thinking","created_at":123,"usage":` + string(usageJSON) + `}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		IsStream:          true,
		OriginModelName:   "gpt-5.5-thinking",
		Request:           &dto.ClaudeRequest{Tools: []dto.Tool{{Name: "exec_command", InputSchema: map[string]interface{}{"type": "object"}}}},
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5-thinking"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}

	body := recorder.Body.String()
	for _, want := range []string{
		`"type":"tool_use"`,
		`"name":"exec_command"`,
		`"partial_json":"{\"cmd\":\"cat AGENTS.md\",\"workdir\":\"/Users/zrf/project/git-project/aicustomer\",\"max_output_tokens\":20000}"`,
		`"stop_reason":"tool_use"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
	for _, forbidden := range []string{
		`"type":"text"`,
		`{"tool_call":{"name":"exec_command"`,
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("response body leaked tool-call JSON/text %q:\n%s", forbidden, body)
		}
	}
	if got := strings.Count(body, "event: content_block_stop"); got != 1 {
		t.Fatalf("expected exactly one tool content block stop, got %d:\n%s", got, body)
	}
}

func TestOaiResponsesToChatStreamHandlerIncludesResponseFailedDetails(t *testing.T) {
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = originalStreamingTimeout
	})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`data: {"type":"response.failed","response":{"status":"failed","error":{"message":"Invalid tool result payload","code":"invalid_tool_result"}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:       types.RelayFormatClaude,
		IsStream:          true,
		OriginModelName:   "gpt-5.5",
		ChannelMeta:       &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{LastMessagesType: relaycommon.LastMessageTypeNone},
	}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr == nil {
		t.Fatal("expected response.failed to return an API error")
	}
	errText := apiErr.Error()
	for _, want := range []string{"response.failed", "Invalid tool result payload", "invalid_tool_result"} {
		if !strings.Contains(errText, want) {
			t.Fatalf("expected error to contain %q, got %q", want, errText)
		}
	}
	if apiErr.GetErrorCode() != types.ErrorCodeBadResponse {
		t.Fatalf("error code = %q, want %q", apiErr.GetErrorCode(), types.ErrorCodeBadResponse)
	}
}

func TestOaiResponsesToChatStreamHandlerOpenAIReasoningOnlyFallsBackToContent(t *testing.T) {
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = originalStreamingTimeout
	})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`data: {"type":"response.created","response":{"model":"gpt-5.5","created_at":123}}`,
		``,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"长任务分析完成，"}`,
		``,
		`data: {"type":"response.reasoning_summary_text.delta","delta":"这是最终结果。"}`,
		``,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5","created_at":123,"usage":{"input_tokens":10,"output_tokens":8,"total_tokens":18}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		IsStream:        true,
		OriginModelName: "gpt-5.5",
		ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.5"},
	}

	usage, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}
	if usage == nil || usage.TotalTokens != 18 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, `"reasoning_content":"长任务分析完成，"`) {
		t.Fatalf("expected original reasoning summary chunk to be preserved, got:\n%s", body)
	}
	if !strings.Contains(body, `"content":"长任务分析完成，这是最终结果。"`) {
		t.Fatalf("expected reasoning-only stream to fall back to visible content, got:\n%s", body)
	}
	if !strings.Contains(body, `data: [DONE]`) {
		t.Fatalf("expected OpenAI stream terminator, got:\n%s", body)
	}
}

func TestOaiResponsesToChatStreamHandlerOpenAIHonorsThinkingToContent(t *testing.T) {
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = originalStreamingTimeout
	})
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set(common.RequestIdKey, "test-request")

	streamBody := strings.Join([]string{
		`data: {"type":"response.reasoning_summary_text.delta","delta":"推理过程"}`,
		``,
		`data: {"type":"response.completed","response":{"model":"gpt-5.5","created_at":123}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}
	info := &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAI,
		IsStream:        true,
		OriginModelName: "gpt-5.5",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-5.5",
			ChannelSetting:    dto.ChannelSettings{ThinkingToContent: true},
		},
		ThinkingContentInfo: relaycommon.ThinkingContentInfo{
			IsFirstThinkingContent: true,
		},
	}

	_, apiErr := OaiResponsesToChatStreamHandler(c, info, resp)
	if apiErr != nil {
		t.Fatalf("OaiResponsesToChatStreamHandler returned error: %v", apiErr)
	}

	body := recorder.Body.String()
	if strings.Contains(body, `"reasoning_content"`) {
		t.Fatalf("thinking_to_content should not expose reasoning_content directly, got:\n%s", body)
	}
	convertedThinking := `"content":"\u003cthink\u003e\n推理过程"`
	if !strings.Contains(body, convertedThinking) {
		t.Fatalf("expected thinking_to_content conversion in Responses bridge, got:\n%s", body)
	}
	if strings.Count(body, convertedThinking) != 1 {
		t.Fatalf("expected no fallback duplication when thinking_to_content is enabled, got:\n%s", body)
	}
}
