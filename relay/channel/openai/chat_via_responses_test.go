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
