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
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newDeepSeekResponsesRelayInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RelayMode:       relayconstant.RelayModeResponses,
		IsStream:        stream,
		DisablePing:     true,
		OriginModelName: "deepseek-flash",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-flash",
		},
	}
}

func TestDeepSeekResponsesNativeRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalTimeout })
	const functionCall = `{"id":"fc_native","type":"function_call","status":"completed","namespace":"mcp__fs__","name":"list","call_id":"call_1","arguments":"{\"path\":\".\"}"}`
	const output = `[{"id":"msg_native","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"hi there","annotations":[]}]},` + functionCall + `,{"id":"ws_native","type":"web_search_call","status":"completed","action":{"type":"search","queries":["deepseek"]}},{"id":"wf_native","type":"web_search_call","status":"completed","action":{"type":"open_page","url":"https://example.com/"}}]`
	const responseBody = `{"id":"resp_native","object":"response","created_at":123,"model":"deepseek-flash","status":"completed","output":` + output + `,"usage":{"input_tokens":3,"output_tokens":2,"total_tokens":5,"input_tokens_details":{"cached_tokens":1}}}`
	const streamBody = "event: response.created\ndata: " + `{"type":"response.created","response":{"id":"resp_native","object":"response","model":"deepseek-flash","status":"in_progress","output":[]}}` + "\n\n" +
		"event: response.output_text.delta\ndata: " + `{"type":"response.output_text.delta","item_id":"msg_native","output_index":0,"content_index":0,"delta":"hi there"}` + "\n\n" +
		"event: response.output_item.added\ndata: " + `{"type":"response.output_item.added","output_index":1,"item":{"id":"fc_native","type":"function_call","namespace":"mcp__fs__","name":"list","call_id":"call_1","arguments":""}}` + "\n\n" +
		"event: response.function_call_arguments.delta\ndata: " + `{"type":"response.function_call_arguments.delta","output_index":1,"item_id":"fc_native","delta":"{\"path\":\".\"}"}` + "\n\n" +
		"event: response.output_item.done\ndata: " + `{"type":"response.output_item.done","output_index":1,"item":` + functionCall + `}` + "\n\n" +
		"event: response.completed\ndata: " + `{"type":"response.completed","response":` + responseBody + `}` + "\n\n"
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
					_, _ = io.WriteString(w, responseBody)
				}
			}))
			defer upstream.Close()
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("Content-Type", "application/json")
			c.Set(common.RequestIdKey, "test-request")
			info := newDeepSeekResponsesRelayInfo(stream)
			info.ChannelBaseUrl = upstream.URL
			info.ApiKey = "synthetic-native-key"
			adaptor := &Adaptor{}
			adaptor.Init(info)
			request := dto.OpenAIResponsesRequest{
				Model: "deepseek-flash", Input: []byte(`"continue"`), Instructions: []byte(`"be concise"`),
				PreviousResponseID: "resp_upstream_previous", Stream: &stream,
				Tools:      []byte(`[{"type":"namespace","name":"mcp__fs__","tools":[{"type":"function","name":"list","parameters":{"type":"object","properties":{"z":{"type":"string"},"a":{"type":"string"}}}}]},{"type":"web_search_preview"}]`),
				ToolChoice: []byte(`{"type":"web_search_preview"}`),
			}
			converted, err := adaptor.ConvertOpenAIResponsesRequest(c, info, request)
			require.NoError(t, err)
			sent, err := common.Marshal(converted)
			require.NoError(t, err)
			response, err := adaptor.DoRequest(c, info, bytes.NewReader(sent))
			require.NoError(t, err)
			captured := <-requests
			require.NoError(t, captured.err)
			assert.Equal(t, "/responses", captured.path)
			assert.Equal(t, "Bearer synthetic-native-key", captured.authorization)
			wire := gjson.ParseBytes(captured.body)
			assert.Equal(t, "resp_upstream_previous", wire.Get("previous_response_id").String())
			assert.Equal(t, "continue", wire.Get("input").String())
			assert.Equal(t, "be concise", wire.Get("instructions").String())
			assert.Equal(t, `{"z":{"type":"string"},"a":{"type":"string"}}`, wire.Get("tools.0.tools.0.parameters.properties").Raw)
			assert.JSONEq(t, string(request.ToolChoice), wire.Get("tool_choice").Raw)
			assert.False(t, wire.Get("messages").Exists())
			usageAny, apiErr := adaptor.DoResponse(c, response.(*http.Response), info)
			require.Nil(t, apiErr)
			usage := usageAny.(*dto.Usage)
			assert.Equal(t, 3, usage.PromptTokens)
			assert.Equal(t, 2, usage.CompletionTokens)
			assert.Equal(t, 5, usage.TotalTokens)
			assert.Equal(t, 1, usage.PromptTokensDetails.CachedTokens)
			counts := toolCounts(info)
			assert.Equal(t, 1, counts[SearchToolPriceKey])
			assert.Equal(t, 1, counts[FetchToolPriceKey])
			body := recorder.Body.String()
			assert.Contains(t, body, `"id":"resp_native"`)
			assert.Contains(t, body, `"namespace":"mcp__fs__"`)
			assert.Contains(t, body, `"call_id":"call_1"`)
			assert.Contains(t, body, `"arguments":"{\"path\":\".\"}"`)
			assert.Contains(t, body, "hi there")
			assert.NotContains(t, body, "chat.completion")
			if stream {
				assert.Contains(t, body, "event: response.output_text.delta")
				assert.Contains(t, body, "event: response.function_call_arguments.delta")
				assert.Contains(t, body, "event: response.completed")
			} else {
				assert.JSONEq(t, responseBody, body)
			}

			// Continuation belongs to the upstream: forward its ID and only the
			// client's new input, without synthesizing local conversation history.
			next, err := adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
				Model: "deepseek-flash", Input: []byte(`"next"`), PreviousResponseID: "resp_native",
			})
			require.NoError(t, err)
			native := next.(dto.OpenAIResponsesRequest)
			assert.Equal(t, "resp_native", native.PreviousResponseID)
			assert.Equal(t, `"next"`, string(native.Input))
			assert.False(t, strings.Contains(string(native.Input), "hi there"))
		})
	}
}
