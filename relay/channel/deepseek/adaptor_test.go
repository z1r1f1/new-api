package deepseek

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func newDeepSeekClaudeRelayInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		RelayMode:       relayconstant.RelayModeUnknown,
		IsStream:        stream,
		OriginModelName: "deepseek-v4-flash-free",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       "https://opencode.ai/zen",
			SupportStreamOptions: true,
			UpstreamModelName:    "deepseek-v4-flash-free",
		},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{
			LastMessagesType: relaycommon.LastMessageTypeNone,
		},
	}
}

func TestDeepSeekClaudeMessagesConvertToOpenAIChatRequest(t *testing.T) {
	stream := true
	maxTokens := uint(64)
	info := newDeepSeekClaudeRelayInfo(stream)
	req := &dto.ClaudeRequest{
		Model:     "deepseek-v4-flash-free",
		System:    "be concise",
		MaxTokens: &maxTokens,
		Stream:    &stream,
		Messages: []dto.ClaudeMessage{
			{Role: "user", Content: "hello"},
		},
	}

	converted, err := (&Adaptor{}).ConvertClaudeRequest(nil, info, req)
	if err != nil {
		t.Fatalf("ConvertClaudeRequest returned error: %v", err)
	}

	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	if !ok {
		t.Fatalf("converted request type = %T, want *dto.GeneralOpenAIRequest", converted)
	}
	if chatReq.Model != "deepseek-v4-flash-free" {
		t.Fatalf("model = %q, want deepseek-v4-flash-free", chatReq.Model)
	}
	if chatReq.MaxTokens == nil || *chatReq.MaxTokens != maxTokens {
		t.Fatalf("max_tokens = %+v, want %d", chatReq.MaxTokens, maxTokens)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2: %+v", len(chatReq.Messages), chatReq.Messages)
	}
	if chatReq.Messages[0].Role != "system" || chatReq.Messages[0].StringContent() != "be concise" {
		t.Fatalf("unexpected system message: %+v", chatReq.Messages[0])
	}
	if chatReq.Messages[1].Role != "user" || chatReq.Messages[1].StringContent() != "hello" {
		t.Fatalf("unexpected user message: %+v", chatReq.Messages[1])
	}
	if chatReq.StreamOptions == nil || !chatReq.StreamOptions.IncludeUsage {
		t.Fatalf("expected stream_options.include_usage=true, got %+v", chatReq.StreamOptions)
	}
}

func TestDeepSeekClaudeMessagesUseChatCompletionsURL(t *testing.T) {
	info := newDeepSeekClaudeRelayInfo(false)

	got, err := (&Adaptor{}).GetRequestURL(info)
	if err != nil {
		t.Fatalf("GetRequestURL returned error: %v", err)
	}
	want := "https://opencode.ai/zen/v1/chat/completions"
	if got != want {
		t.Fatalf("request url = %q, want %q", got, want)
	}
}

func TestDeepSeekClaudeMessagesResponseConvertsOpenAIChatToClaude(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages?beta=true", nil)
	c.Set(common.RequestIdKey, "test-request")
	info := newDeepSeekClaudeRelayInfo(false)

	openAIChatBody := `{"id":"chatcmpl_1","object":"chat.completion","created":123,"model":"deepseek-v4-flash-free","choices":[{"index":0,"message":{"role":"assistant","content":"hi there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
	usageAny, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(openAIChatBody)),
	}, info)
	if apiErr != nil {
		t.Fatalf("DoResponse returned error: %v", apiErr)
	}

	usage := usageAny.(*dto.Usage)
	if usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	var claudeResp dto.ClaudeResponse
	if err := common.Unmarshal(recorder.Body.Bytes(), &claudeResp); err != nil {
		t.Fatalf("failed to decode Claude response: %v\n%s", err, recorder.Body.String())
	}
	if claudeResp.Type != "message" || claudeResp.Role != "assistant" {
		t.Fatalf("unexpected Claude response metadata: %+v", claudeResp)
	}
	if len(claudeResp.Content) != 1 || claudeResp.Content[0].GetText() != "hi there" {
		t.Fatalf("unexpected Claude response content: %+v body=%s", claudeResp.Content, recorder.Body.String())
	}
	if claudeResp.Usage == nil || claudeResp.Usage.InputTokens != 3 || claudeResp.Usage.OutputTokens != 2 {
		t.Fatalf("unexpected Claude usage: %+v", claudeResp.Usage)
	}
}
