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
	appconstant "github.com/QuantumNous/new-api/constant"
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
		OriginModelName: "deepseek-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl:       "https://api.deepseek.com",
			SupportStreamOptions: true,
			UpstreamModelName:    "deepseek-flash",
		},
		ClaudeConvertInfo: &relaycommon.ClaudeConvertInfo{
			LastMessagesType: relaycommon.LastMessageTypeNone,
		},
	}
}

func TestDeepSeekRequestURL(t *testing.T) {
	for _, tc := range []struct {
		name, base, path string
		format           types.RelayFormat
		mode             int
	}{
		{name: "claude", base: "https://api.deepseek.com", format: types.RelayFormatClaude, mode: constant.RelayModeUnknown, path: "/anthropic/v1/messages"},
		{name: "chat", base: "https://api.deepseek.com", format: types.RelayFormatOpenAI, mode: constant.RelayModeChatCompletions, path: "/v1/chat/completions"},
		{name: "responses", base: "https://api.deepseek.com", format: types.RelayFormatOpenAIResponses, mode: constant.RelayModeResponses, path: "/responses"},
		{name: "search", base: "https://api.deepseek.com", format: types.RelayFormatOpenAI, mode: constant.RelayModeToolsSearch, path: "/v1/tools/search"},
		{name: "fetch", base: "https://api.deepseek.com", format: types.RelayFormatOpenAI, mode: constant.RelayModeToolsFetch, path: "/v1/tools/fetch"},
		{name: "completions", base: "https://api.deepseek.com", format: types.RelayFormatOpenAI, mode: constant.RelayModeCompletions, path: "/beta/completions"},
		{name: "beta completions", base: "https://api.deepseek.com/beta", format: types.RelayFormatOpenAI, mode: constant.RelayModeCompletions, path: "/completions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			info := &relaycommon.RelayInfo{
				RelayFormat: tc.format,
				RelayMode:   tc.mode,
				ChannelMeta: &relaycommon.ChannelMeta{ChannelBaseUrl: tc.base},
			}
			got, err := (&Adaptor{}).GetRequestURL(info)
			require.NoError(t, err)
			assert.Equal(t, tc.base+tc.path, got)
		})
	}
}

func TestDeepSeekClaudeNativeRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTimeout := appconstant.StreamingTimeout
	appconstant.StreamingTimeout = 30
	t.Cleanup(func() { appconstant.StreamingTimeout = originalTimeout })
	const message = `{"id":"msg_native","type":"message","role":"assistant","model":"deepseek-flash","content":[{"type":"text","text":"hi there"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2,"cache_read_input_tokens":1}}`
	const streamBody = "event: message_start\ndata: " + `{"type":"message_start","message":{"id":"msg_native","type":"message","role":"assistant","model":"deepseek-flash","content":[],"usage":{"input_tokens":3,"output_tokens":0,"cache_read_input_tokens":1}}}` + "\n\n" +
		"event: content_block_start\ndata: " + `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}` + "\n\n" +
		"event: content_block_delta\ndata: " + `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi there"}}` + "\n\n" +
		"event: content_block_stop\ndata: " + `{"type":"content_block_stop","index":0}` + "\n\n" +
		"event: message_delta\ndata: " + `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}` + "\n\n" +
		"event: message_stop\ndata: " + `{"type":"message_stop"}` + "\n\n"
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			type capturedRequest struct {
				path, authorization string
				body                []byte
				err                 error
			}
			requests := make(chan capturedRequest, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				requests <- capturedRequest{r.URL.Path, r.Header.Get("Authorization"), body, err}
				if stream {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, streamBody)
				} else {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, message)
				}
			}))
			defer upstream.Close()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(common.RequestIdKey, "test-request")
			info := newDeepSeekClaudeRelayInfo(stream)
			info.ChannelBaseUrl = upstream.URL
			info.ApiKey = "synthetic-native-key"
			info.DisablePing = true
			adaptor := &Adaptor{}
			adaptor.Init(info)
			maxTokens := uint(64)
			converted, err := adaptor.ConvertClaudeRequest(c, info, &dto.ClaudeRequest{
				Model: "deepseek-flash", System: "be concise", MaxTokens: &maxTokens, Stream: &stream,
				Messages: []dto.ClaudeMessage{{Role: "user", Content: "hello"}},
				Tools:    common.RawMessage(`[{"name":"read_file","input_schema":{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}}}}]`),
			})
			require.NoError(t, err)
			sent, err := common.Marshal(converted)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(c, info, bytes.NewReader(sent))
			require.NoError(t, err)
			captured := <-requests
			require.NoError(t, captured.err)
			assert.Equal(t, "/anthropic/v1/messages", captured.path)
			assert.Equal(t, "Bearer synthetic-native-key", captured.authorization)
			assert.Equal(t, "be concise", gjson.GetBytes(captured.body, "system").String())
			assert.Equal(t, "hello", gjson.GetBytes(captured.body, "messages.0.content").String())
			assert.Equal(t, `{"z":{"type":"string"},"a":{"type":"string"}}`, gjson.GetBytes(captured.body, "tools.0.input_schema.properties").Raw)
			assert.False(t, gjson.GetBytes(captured.body, "stream_options").Exists())
			usageAny, apiErr := adaptor.DoResponse(c, response.(*http.Response), info)
			require.Nil(t, apiErr)
			usage := usageAny.(*dto.Usage)
			assert.Equal(t, 3, usage.PromptTokens)
			assert.Equal(t, 2, usage.CompletionTokens)
			assert.Equal(t, 5, usage.TotalTokens)
			assert.Equal(t, 1, usage.PromptTokensDetails.CachedTokens)
			assert.Equal(t, types.RelayFormat(types.RelayFormatClaude), info.FinalRequestRelayFormat)
			assert.Contains(t, recorder.Body.String(), `"id":"msg_native"`)
			assert.Contains(t, recorder.Body.String(), "hi there")
			assert.NotContains(t, recorder.Body.String(), "chat.completion")
			if stream {
				assert.Contains(t, recorder.Body.String(), "event: content_block_delta")
				assert.Contains(t, recorder.Body.String(), "event: message_stop")
			} else {
				assert.JSONEq(t, message, recorder.Body.String())
			}
		})
	}
}

// Exercise each native protocol through public conversion and HTTP routing.
func TestThinkingControlsUseNativeProtocols(t *testing.T) {
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
				claude:    `"thinking":{"type":"enabled","budget_tokens":2048},"output_config":{"effort":"high","format":{"type":"json_schema","schema":{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}}}}}`,
				responses: `"reasoning":{"effort":"high"}`}},
		{name: "budget overrides client effort none", model: "deepseek-flash@thinking:8192", upstream: "deepseek-flash", thinking: "enabled", effort: "medium", logEffort: "high",
			client: client{responses: `"reasoning":{"effort":"none"}`}},
		{name: "effort wins over max tail", model: "deepseek-flash-max@effort:low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "effort wins over none tail", model: "deepseek-flash-none@effort:low", upstream: "deepseek-flash", thinking: "enabled", effort: "low", logEffort: "low"},
		{name: "thinking off wins over low tail", model: "deepseek-flash-low@thinking:off", upstream: "deepseek-flash", thinking: "disabled", logEffort: "none"},
		{name: "no modifier preserves client controls", model: "deepseek-flash", upstream: "deepseek-flash",
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
						state := info.ReasoningConversion
						converted, err = (&Adaptor{}).ConvertClaudeRequest(nil, info, outbound)
						assert.Same(t, state, info.ReasoningConversion)
					default:
						original, outbound := &dto.OpenAIResponsesRequest{}, &dto.OpenAIResponsesRequest{}
						require.NoError(t, common.UnmarshalJsonStr(body, original))
						require.NoError(t, common.UnmarshalJsonStr(body, outbound))
						info.Request = original
						require.NoError(t, helper.ApplyReasoningModelSuffix(nil, info, outbound))
						converted, err = (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, *outbound)
					}
					require.NoError(t, err)
					switch format {
					case types.RelayFormatClaude:
						require.IsType(t, &dto.ClaudeRequest{}, converted)
					case types.RelayFormatOpenAIResponses:
						require.IsType(t, dto.OpenAIResponsesRequest{}, converted)
					default:
						require.IsType(t, &dto.GeneralOpenAIRequest{}, converted)
					}
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
					paths := map[types.RelayFormat]string{
						types.RelayFormatOpenAI: "/v1/chat/completions", types.RelayFormatClaude: "/anthropic/v1/messages", types.RelayFormatOpenAIResponses: "/responses",
					}
					assert.Equal(t, paths[format], captured.path)
					wire := gjson.ParseBytes(captured.body)
					assert.Equal(t, tt.upstream, wire.Get("model").String())
					assert.Equal(t, tt.upstream, info.UpstreamModelName)
					if format == types.RelayFormatOpenAIResponses {
						assert.Equal(t, "hi", wire.Get("input").String())
						assert.False(t, wire.Get("messages").Exists())
					} else {
						assert.Equal(t, "hi", wire.Get("messages.0.content").String())
						assert.False(t, wire.Get("input").Exists())
					}
					if format == types.RelayFormatClaude {
						assert.Equal(t, int64(64), wire.Get("max_tokens").Int())
						wantTemperature := 0.7
						if strings.Contains(tt.model, "@temperature:0.3") {
							wantTemperature = 0.3
						}
						assert.Equal(t, wantTemperature, wire.Get("temperature").Float())
					}
					if tt.thinking == "" {
						for _, field := range []string{"thinking", "reasoning_effort", "reasoning", "output_config"} {
							assert.Equal(t, gjson.Get(body, field).Raw, wire.Get(field).Raw, field)
						}
						if format == types.RelayFormatOpenAIResponses && tt.client.responses != "" {
							assert.Equal(t, "low", info.ReasoningEffort)
						}
						return
					}
					switch format {
					case types.RelayFormatOpenAIResponses:
						wantEffort := tt.effort
						if tt.thinking == "disabled" {
							wantEffort = "none"
						}
						assert.Equal(t, wantEffort, wire.Get("reasoning.effort").String())
						assert.False(t, wire.Get("thinking").Exists())
						assert.False(t, wire.Get("reasoning_effort").Exists())
					case types.RelayFormatClaude:
						assert.JSONEq(t, `{"type":"`+tt.thinking+`"}`, wire.Get("thinking").Raw)
						assert.Equal(t, tt.effort, wire.Get("output_config.effort").String())
						assert.Equal(t, gjson.Get(body, "output_config.format").Raw, wire.Get("output_config.format").Raw)
						assert.False(t, wire.Get("reasoning_effort").Exists())
						assert.False(t, wire.Get("reasoning").Exists())
					default:
						assert.JSONEq(t, `{"type":"`+tt.thinking+`"}`, wire.Get("thinking").Raw)
						assert.Equal(t, tt.effort, wire.Get("reasoning_effort").String())
						assert.False(t, wire.Get("reasoning").Exists())
					}
					assert.Equal(t, tt.logEffort, info.ReasoningEffort)
				})
			}
		})
	}
}

// Verify pricing metadata and native Responses vendor tool accounting.
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
