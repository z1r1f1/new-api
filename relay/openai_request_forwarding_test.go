package relay

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAINativeRequestsForwardCodexEffortAndServiceTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service.InitHttpClient()
	settings := model_setting.GetGlobalSettings()
	previousPassThrough := settings.PassThroughRequestEnabled
	settings.PassThroughRequestEnabled = false
	t.Cleanup(func() { settings.PassThroughRequestEnabled = previousPassThrough })

	for _, route := range []struct {
		name, endpoint string
		channelType    int
	}{
		{name: "OpenAI Responses", endpoint: "/v1/responses", channelType: constant.ChannelTypeOpenAI},
		{name: "OpenAI Chat Completions", endpoint: "/v1/chat/completions", channelType: constant.ChannelTypeOpenAI},
		{name: "Codex Responses", endpoint: "/v1/responses", channelType: constant.ChannelTypeCodex},
	} {
		t.Run(route.name, func(t *testing.T) {
			endpoint := route.endpoint
			for _, tc := range []struct {
				name, requestedTier, upstreamTier string
				allowTier, passThrough, stream    bool
			}{
				{name: "ultra survives default tier filtering", requestedTier: "priority", stream: true},
				{name: "priority is forwarded when allowed", requestedTier: "priority", upstreamTier: "priority", allowTier: true, stream: true},
				{name: "fast alias becomes priority", requestedTier: "fast", upstreamTier: "priority", allowTier: true},
				{name: "passthrough keeps the client body", requestedTier: "priority", upstreamTier: "priority", passThrough: true},
				{name: "absent tier stays absent", allowTier: true},
			} {
				t.Run(tc.name, func(t *testing.T) {
					captured := make(chan []byte, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, err := io.ReadAll(r.Body)
						if err != nil {
							http.Error(w, err.Error(), http.StatusBadRequest)
							return
						}
						captured <- body
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadGateway)
						_, _ = io.WriteString(w, `{"error":{"message":"fixture upstream failure","type":"upstream_error"}}`)
					}))
					t.Cleanup(upstream.Close)
					body := map[string]any{"model": "gpt-6.1-sol"}
					if tc.stream {
						body["stream"] = true
					}
					effortPath := "reasoning.effort"
					if endpoint == "/v1/responses" {
						body["input"] = "hello"
						body["reasoning"] = map[string]any{"effort": "ultra", "summary": "auto"}
					} else {
						body["messages"] = []map[string]any{{"role": "user", "content": "hello"}}
						body["reasoning_effort"] = "ultra"
						effortPath = "reasoning_effort"
					}
					if tc.requestedTier != "" {
						body["service_tier"] = tc.requestedTier
					}
					raw, err := common.Marshal(body)
					require.NoError(t, err)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(string(raw)))
					c.Request.Header.Set("Content-Type", "application/json")
					common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-6.1-sol")
					common.SetContextKey(c, constant.ContextKeyChannelType, route.channelType)
					common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
					key := "test-key"
					if route.channelType == constant.ChannelTypeCodex {
						key = `{"access_token":"test-access","account_id":"test-account"}`
					}
					common.SetContextKey(c, constant.ContextKeyChannelKey, key)
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: tc.passThrough})
					common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AllowServiceTier: tc.allowTier})
					if endpoint == "/v1/responses" {
						request, err := helper.GetAndValidateResponsesRequest(c)
						require.NoError(t, err)
						info := relaycommon.GenRelayInfoResponses(c, request)
						apiErr := ResponsesHelper(c, info)
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					} else {
						request, err := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
						require.NoError(t, err)
						info := relaycommon.GenRelayInfoOpenAI(c, request)
						apiErr := TextHelper(c, info)
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusBadGateway, apiErr.StatusCode)
					}
					require.Len(t, captured, 1)
					sent := <-captured
					assert.Equal(t, "ultra", gjson.GetBytes(sent, effortPath).String())
					if tc.stream {
						assert.True(t, gjson.GetBytes(sent, "stream").Bool())
					}
					if tc.upstreamTier == "" {
						assert.False(t, gjson.GetBytes(sent, "service_tier").Exists())
					} else {
						assert.Equal(t, tc.upstreamTier, gjson.GetBytes(sent, "service_tier").String())
					}
					if tc.passThrough {
						assert.JSONEq(t, string(raw), string(sent))
					}
				})
			}
		})
	}
}
