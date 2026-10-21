package deepseek

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newDeepSeekClaudeRelayInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatClaude,
		RelayMode:       constant.RelayModeUnknown,
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
	require.NoError(t, err)
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "converted request type = %T, want *dto.GeneralOpenAIRequest", converted)
	assert.Equal(t, "deepseek-v4-flash-free", chatReq.Model)
	require.NotNil(t, chatReq.MaxTokens)
	assert.Equal(t, maxTokens, *chatReq.MaxTokens)
	require.Len(t, chatReq.Messages, 2)
	assert.Equal(t, "system", chatReq.Messages[0].Role)
	assert.Equal(t, "be concise", chatReq.Messages[0].StringContent())
	assert.Equal(t, "user", chatReq.Messages[1].Role)
	assert.Equal(t, "hello", chatReq.Messages[1].StringContent())
	require.NotNil(t, chatReq.StreamOptions)
	assert.True(t, chatReq.StreamOptions.IncludeUsage)
}

func TestDeepSeekRequestURL(t *testing.T) {
	for _, format := range []types.RelayFormat{
		types.RelayFormatClaude, types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses,
	} {
		for _, tc := range []struct {
			name, base, path string
			mode             int
		}{
			{name: "chat", base: "https://opencode.ai/zen", mode: constant.RelayModeChatCompletions, path: "/v1/chat/completions"},
			{name: "responses bridge", base: "https://opencode.ai/zen", mode: constant.RelayModeResponses, path: "/v1/chat/completions"},
			{name: "search", base: "https://api.deepseek.com", mode: constant.RelayModeToolsSearch, path: "/v1/tools/search"},
			{name: "fetch", base: "https://api.deepseek.com", mode: constant.RelayModeToolsFetch, path: "/v1/tools/fetch"},
			{name: "completions", base: "https://api.deepseek.com", mode: constant.RelayModeCompletions, path: "/beta/completions"},
			{name: "beta completions", base: "https://api.deepseek.com/beta", mode: constant.RelayModeCompletions, path: "/completions"},
		} {
			t.Run(string(format)+"/"+tc.name, func(t *testing.T) {
				info := &relaycommon.RelayInfo{
					RelayFormat: format,
					RelayMode:   tc.mode,
					ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: tc.base},
				}
				path := tc.path
				if format == types.RelayFormatClaude && tc.mode == constant.RelayModeCompletions {
					path = "/v1/chat/completions"
				}
				got, err := (&Adaptor{}).GetRequestURL(info)
				require.NoError(t, err)
				assert.Equal(t, tc.base+path, got)
			})
		}
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
	require.Nil(t, apiErr)
	usage := usageAny.(*dto.Usage)
	assert.Equal(t, 3, usage.PromptTokens)
	assert.Equal(t, 2, usage.CompletionTokens)
	assert.Equal(t, 5, usage.TotalTokens)

	var claudeResp dto.ClaudeResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &claudeResp))
	assert.Equal(t, "message", claudeResp.Type)
	assert.Equal(t, "assistant", claudeResp.Role)
	require.Len(t, claudeResp.Content, 1)
	assert.Equal(t, "hi there", claudeResp.Content[0].GetText())
	require.NotNil(t, claudeResp.Usage)
	assert.Equal(t, 3, claudeResp.Usage.InputTokens)
	assert.Equal(t, 2, claudeResp.Usage.OutputTokens)
}

// All three client formats leave the adaptor as Chat Completions. Exercise
// the public conversion and HTTP request path, not unused native-format helpers.
func TestThinkingControlsUseChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := model_setting.GetGlobalSettings()
	original := append([]string(nil), settings.ThinkingModelBlacklist...)
	t.Cleanup(func() { settings.ThinkingModelBlacklist = original })
	settings.ThinkingModelBlacklist = append(original, "deepseek-v4-pro-max")
	for _, model := range []string{"deepseek-flash", "deepseek-flash-none", "deepseek-flash-low", "deepseek-flash-max", "deepseek-v4-pro-low"} {
		assert.Contains(t, (&Adaptor{}).GetModelList(), model)
	}

	type client struct{ chat, claude, responses string }
	tests := []struct {
		name, model, upstream string
		client                client
		thinking, effort      string // thinking "" = no DeepSeek override
		logEffort             string
	}{
		{name: "bare flash", model: "deepseek-flash", upstream: "deepseek-flash"},
		{name: "flash none", model: "deepseek-flash-none", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none"},
		{name: "flash low", model: "deepseek-flash-low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "flash max", model: "deepseek-flash-max", upstream: "deepseek-flash", thinking: "enabled", effort: "max", logEffort: "max"},
		{name: "v4 pro none", model: "deepseek-v4-pro-none", upstream: "deepseek-v4-pro", thinking: "disabled", logEffort: "none"},
		{name: "v4 pro low", model: "deepseek-v4-pro-low", upstream: "deepseek-v4-pro", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "blacklisted tail", model: "deepseek-v4-pro-max", upstream: "deepseek-v4-pro-max"},
		{name: "v4 flash low", model: "deepseek-v4-flash-low", upstream: "deepseek-v4-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "thinking on", model: "deepseek-flash@thinking:on", upstream: "deepseek-flash", thinking: "enabled", logEffort: "high"},
		{name: "thinking off", model: "deepseek-flash@thinking:off", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none"},
		{name: "thinking zero budget", model: "deepseek-flash@thinking:0", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none"},
		{name: "effort none", model: "deepseek-flash@effort:none", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none"},
		{name: "effort minimal", model: "deepseek-flash@effort:minimal", upstream: "deepseek-flash", thinking: "enabled", effort: "minimal", logEffort: "low"},
		{name: "effort low", model: "deepseek-flash@effort:low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "effort medium", model: "deepseek-flash@effort:medium", upstream: "deepseek-flash", thinking: "enabled", effort: "medium", logEffort: "high"},
		{name: "effort xhigh", model: "deepseek-flash@effort:xhigh", upstream: "deepseek-flash", thinking: "enabled", effort: "xhigh", logEffort: "high"},
		{name: "effort max on pro", model: "deepseek-v4-pro@effort:max", upstream: "deepseek-v4-pro", thinking: "enabled", effort: "max", logEffort: "max"},
		{name: "budget maps to an effort", model: "deepseek-flash@thinking:8192", upstream: "deepseek-flash", thinking: "enabled", effort: "medium", logEffort: "high"},
		{name: "adaptive", model: "deepseek-flash@thinking:adaptive", upstream: "deepseek-flash", thinking: "enabled", logEffort: "high"},
		{name: "temperature with effort", model: "deepseek-flash@temperature:0.3@effort:low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "effort overrides client thinking off", model: "deepseek-flash@effort:max", upstream: "deepseek-flash", thinking: "enabled", effort: "max", logEffort: "max",
			client: client{chat: `"thinking":{"type":"disabled"}`, claude: `"thinking":{"type":"disabled"}`, responses: `"reasoning":{"effort":"none"}`}},
		{name: "thinking off overrides client thinking", model: "deepseek-flash@thinking:off", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none",
			client: client{chat: `"thinking":{"type":"enabled"},"reasoning_effort":"high"`,
				claude:    `"thinking":{"type":"enabled","budget_tokens":2048},"output_config":{"effort":"high","format":{"type":"json_schema"}}`,
				responses: `"reasoning":{"effort":"high"}`}},
		{name: "budget overrides client effort none", model: "deepseek-flash@thinking:8192", upstream: "deepseek-flash", thinking: "enabled", effort: "medium", logEffort: "high",
			client: client{responses: `"reasoning":{"effort":"none"}`}},
		{name: "effort wins over max tail", model: "deepseek-flash-max@effort:low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "effort wins over none tail", model: "deepseek-flash-none@effort:low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "thinking off wins over low tail", model: "deepseek-flash-low@thinking:off", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none"},
		{name: "no modifier preserves or projects client controls", model: "deepseek-flash", upstream: "deepseek-flash",
			client: client{chat: `"thinking":{"type":"disabled"},"reasoning_effort":"low"`, claude: `"thinking":{"type":"adaptive"},"output_config":{"effort":"low"}`, responses: `"reasoning":{"effort":"low"}`}},
	}
	extra := func(members string) string {
		if members == "" {
			return ""
		}
		return "," + members
	}
	type capturedRequest struct {
		path string
		body []byte
		err  error
	}
	requests := make(chan capturedRequest, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		requests <- capturedRequest{path: r.URL.Path, body: body, err: err}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bodies := map[types.RelayFormat]string{
				types.RelayFormatOpenAI:          fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"hi"}]%s}`, tt.model, extra(tt.client.chat)),
				types.RelayFormatClaude:          fmt.Sprintf(`{"model":%q,"max_tokens":64,"temperature":0.7,"messages":[{"role":"user","content":"hi"}]%s}`, tt.model, extra(tt.client.claude)),
				types.RelayFormatOpenAIResponses: fmt.Sprintf(`{"model":%q,"input":"hi"%s}`, tt.model, extra(tt.client.responses)),
			}
			for format, body := range bodies {
				t.Run(string(format), func(t *testing.T) {
					info := &relaycommon.RelayInfo{
						OriginModelName: tt.model,
						RelayFormat:     format,
						RelayMode:       constant.RelayModeChatCompletions,
						ChannelMeta:     &relaycommon.ChannelMeta{UpstreamModelName: tt.model, ChannelBaseUrl: upstream.URL},
					}
					if format == types.RelayFormatOpenAIResponses {
						info.RelayMode = constant.RelayModeResponses
					}
					var converted any
					var err error
					switch format {
					case types.RelayFormatOpenAI:
						original, outbound := &dto.GeneralOpenAIRequest{}, &dto.GeneralOpenAIRequest{}
						require.NoError(t, common.UnmarshalJsonStr(body, original))
						require.NoError(t, common.UnmarshalJsonStr(body, outbound))
						info.Request = original
						require.NoError(t, helper.ApplyReasoningModelSuffix(nil, info, outbound))
						converted, err = (&Adaptor{}).ConvertOpenAIRequest(nil, info, outbound)
					case types.RelayFormatClaude:
						original, outbound := &dto.ClaudeRequest{}, &dto.ClaudeRequest{}
						require.NoError(t, common.UnmarshalJsonStr(body, original))
						require.NoError(t, common.UnmarshalJsonStr(body, outbound))
						info.Request = original
						require.NoError(t, helper.ApplyReasoningModelSuffix(nil, info, outbound))
						converted, err = (&Adaptor{}).ConvertClaudeRequest(nil, info, outbound)
					default:
						original, outbound := &dto.OpenAIResponsesRequest{}, &dto.OpenAIResponsesRequest{}
						require.NoError(t, common.UnmarshalJsonStr(body, original))
						require.NoError(t, common.UnmarshalJsonStr(body, outbound))
						info.Request = original
						require.NoError(t, helper.ApplyReasoningModelSuffix(nil, info, outbound))
						converted, err = (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, *outbound)
					}
					require.NoError(t, err)
					require.IsType(t, &dto.GeneralOpenAIRequest{}, converted)
					sent, err := common.Marshal(converted)
					require.NoError(t, err)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, "/", nil)
					c.Request.Header.Set("Content-Type", "application/json")
					response, err := (&Adaptor{}).DoRequest(c, info, bytes.NewReader(sent))
					require.NoError(t, err)
					require.NoError(t, response.(*http.Response).Body.Close())
					captured := <-requests
					require.NoError(t, captured.err)
					assert.Equal(t, "/v1/chat/completions", captured.path)
					wire := gjson.ParseBytes(captured.body)
					assert.Equal(t, tt.upstream, wire.Get("model").String())
					assert.Equal(t, tt.upstream, info.UpstreamModelName)
					assert.Equal(t, "hi", wire.Get("messages.0.content").String())
					assert.False(t, wire.Get("input").Exists())
					assert.False(t, wire.Get("output_config").Exists())
					assert.False(t, wire.Get("reasoning").Exists())
					if format == types.RelayFormatClaude {
						assert.Equal(t, int64(64), wire.Get("max_tokens").Int())
						wantTemperature := 0.7
						if strings.Contains(tt.model, "@temperature:0.3") {
							wantTemperature = 0.3
						}
						assert.Equal(t, wantTemperature, wire.Get("temperature").Float())
					}
					if tt.thinking == "" {
						if tt.client.chat != "" {
							assert.Equal(t, "low", wire.Get("reasoning_effort").String())
							if format == types.RelayFormatOpenAI {
								assert.Equal(t, gjson.Get(body, "thinking").Raw, wire.Get("thinking").Raw)
							} else {
								assert.False(t, wire.Get("thinking").Exists())
							}
						} else {
							assert.False(t, wire.Get("thinking").Exists())
							assert.False(t, wire.Get("reasoning_effort").Exists())
						}
						return
					}
					assert.JSONEq(t, `{"type":"`+tt.thinking+`"}`, wire.Get("thinking").Raw)
					assert.Equal(t, tt.effort, wire.Get("reasoning_effort").String())
					assert.Equal(t, tt.logEffort, info.ReasoningEffort)
				})
			}
		})
	}
}

// This verifies pricing metadata and its public accounting API, not a native
// Responses transport: the local Chat bridge does not execute hosted web tools.
func TestServerSideSearchBillingKeys(t *testing.T) {
	for _, tc := range []struct {
		format types.RelayFormat
		mode   int
	}{
		{types.RelayFormatClaude, constant.RelayModeUnknown},
		{types.RelayFormatOpenAI, constant.RelayModeChatCompletions},
		{types.RelayFormatOpenAIResponses, constant.RelayModeResponses},
		{types.RelayFormatOpenAI, constant.RelayModeToolsSearch},
		{types.RelayFormatOpenAI, constant.RelayModeToolsFetch},
	} {
		info := &relaycommon.RelayInfo{RelayFormat: tc.format, RelayMode: tc.mode, ChannelMeta: &relaycommon.ChannelMeta{}}
		(&Adaptor{}).Init(info)
		assert.Equal(t, SearchToolPriceKey, info.WebSearchBillingKey, tc.format)
		assert.Equal(t, FetchToolPriceKey, info.WebFetchBillingKey, tc.format)
		assert.Equal(t, tc.mode == constant.RelayModeResponses, info.VendorToolUsage != nil, tc.format)
	}

	const output = `[
		{"type":"web_search_call","status":"completed","action":{"type":"search","queries":["deepseek","ws_call_id=call_01"]}},
		{"type":"web_search_call","status":"completed","action":{"type":"open_page","url":"https://example.com/"}},
		{"type":"web_search_call","status":"failed","action":{"type":"open_page","url":"https://api-docs.deepseek.com/"}},
		{"type":"message","status":"completed"}]`
	for _, raw := range []string{
		`{"status":"completed","output":` + output + `}`,
		`{"type":"response.completed","response":{"status":"completed","output":` + output + `}}`,
	} {
		info := &relaycommon.RelayInfo{
			OriginModelName: "gpt-5.1-codex",
			RelayFormat:     types.RelayFormatOpenAIResponses,
			RelayMode:       constant.RelayModeResponses,
			ChannelMeta:     &relaycommon.ChannelMeta{},
		}
		(&Adaptor{}).Init(info)
		for range 3 {
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		}
		assert.Equal(t, map[string]int{SearchToolPriceKey: 3}, toolCounts(info))
		info.ApplyVendorToolUsage([]byte(`{"type":"response.failed","response":{"status":"failed","output":[]}}`))
		assert.Equal(t, map[string]int{SearchToolPriceKey: 3}, toolCounts(info), "an empty output keeps the previous count")
		info.ApplyVendorToolUsage([]byte(raw))
		assert.Equal(t, map[string]int{SearchToolPriceKey: 1, FetchToolPriceKey: 1}, toolCounts(info), raw)
	}
}

func toolCounts(info *relaycommon.RelayInfo) map[string]int {
	counts := map[string]int{}
	for name, tool := range info.ResponsesUsageInfo.BuiltInTools {
		counts[name] = tool.CallCount
	}
	return counts
}
