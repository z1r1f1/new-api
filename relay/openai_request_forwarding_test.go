package relay

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/pkg/jsplugin"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relay/requestplugin"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/config"
	"github.com/QuantumNous/new-api/setting/model_setting"
	"github.com/QuantumNous/new-api/setting/request_plugin_setting"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
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

const gatewayNormalizationPlugin = `export const meta = {apiVersion: 1, key: "rp-normalized-wire", name: "Normalized wire", version: "1.0.0", author: {name: "Test"}};
export function ingress() { return {action: "continue"}; }
export function outbound(ctx) {
  if (ctx.config.passthrough) return {action: "reject", code: "raw.changed", message: "Passthrough outbound must not run"};
  return {action: "continue", effects: [{type: "json.patch", ops: [{op: "add", path: "/prompt_cache_key", value: ctx.config.key}]}]};
}
export function egress(ctx, req) {
  const b = req.body;
  if (!b || b.prompt_cache_key !== ctx.config.want || b.service_tier !== "priority") {
    return {action: "reject", code: "wire.scalars", message: "Egress did not see normalized gateway scalars"};
  }
  if (ctx.config.websocket && (b.type !== "response.create" || b.generate !== false || b.stream_id !== "normalized-wire" || "stream" in b || "stream_options" in b || "background" in b)) {
    return {action: "reject", code: "wire.envelope", message: "Egress did not see the final WebSocket envelope"};
  }
  return {action: "continue", findings: [{rule: "normalized", category: "normalized.wire", count: 1}]};
}`

func TestGatewayNormalizationPrecedesEgressAndMatchesHTTPAndWSWire(t *testing.T) {
	service.InitHttpClient()
	require.NoError(t, i18n.Init())
	plugin, err := jsplugin.DefaultRegistry.Register(gatewayNormalizationPlugin, jsplugin.Options{})
	require.NoError(t, err)
	t.Cleanup(func() { _ = jsplugin.DefaultRegistry.Unregister(plugin.Meta.Key) })
	settings := model_setting.GetGlobalSettings()
	previousPassthrough := settings.PassThroughRequestEnabled
	previousDisk := common.GetDiskCacheConfig()
	t.Cleanup(func() {
		settings.PassThroughRequestEnabled = previousPassthrough
		common.SetDiskCacheConfig(previousDisk)
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: "[]"}))
	})
	const schema = `{"type":"object","properties":{"z":{"const":9007199254740993},"a":{"const":0}}}`
	for _, disk := range []bool{false, true} {
		for _, route := range []string{"chat", "responses", "websocket"} {
			for _, mode := range []string{"plugin", "override", "channel passthrough", "global passthrough"} {
				t.Run(fmt.Sprintf("disk=%t/%s/%s", disk, route, mode), func(t *testing.T) {
					common.SetDiskCacheConfig(common.DiskCacheConfig{Enabled: disk, ThresholdMB: 0, MaxSizeMB: 16, Path: t.TempDir()})
					settings.PassThroughRequestEnabled = mode == "global passthrough"
					passthrough := mode == "channel passthrough" || settings.PassThroughRequestEnabled
					clientKey, pluginKey, overrideKey := strings.Repeat("c", 70), strings.Repeat("p", 70), strings.Repeat("o", 70)
					finalKey := pluginKey
					if passthrough {
						finalKey = clientKey
					} else if mode == "override" {
						finalKey = overrideKey
					}
					wantKey := fmt.Sprintf("%x", sha256.Sum256([]byte(finalKey)))
					instances, err := common.Marshal([]any{map[string]any{
						"id": "wire", "name": "Wire", "plugin": plugin.Meta.Key, "enabled": true, "global": true,
						"config": map[string]any{"key": pluginKey, "want": wantKey, "passthrough": passthrough, "websocket": route == "websocket"},
					}})
					require.NoError(t, err)
					require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{request_plugin_setting.InstancesOption: string(instances)}))
					endpoint := "/v1/responses"
					members := `"input":"hello","tools":[{"type":"function","name":"lookup","parameters":` + schema + `}]`
					var format types.RelayFormat = types.RelayFormatOpenAIResponses
					if route == "chat" {
						endpoint = "/v1/chat/completions"
						members = `"messages":[{"role":"user","content":"hello"}],"tools":[{"type":"function","function":{"name":"lookup","parameters":` + schema + `}}]`
						format = types.RelayFormatOpenAI
					}
					// Reasoning-only models intentionally remove temperature; this model accepts explicit zero.
					raw := fmt.Sprintf(` { "model":"gpt-4o-mini", %s, "service_tier":"fast", "prompt_cache_key":%q, "temperature":0, "store":false } `, members, clientKey)
					captured := make(chan []byte, 1)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						body, readErr := io.ReadAll(r.Body)
						if readErr != nil {
							http.Error(w, "fixture body read failed", http.StatusBadRequest)
							return
						}
						captured <- body
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusBadGateway)
						_, _ = io.WriteString(w, `{"error":{"message":"fixture upstream failure","type":"upstream_error"}}`)
					}))
					t.Cleanup(upstream.Close)
					c, _ := gin.CreateTestContext(httptest.NewRecorder())
					c.Request = httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(raw))
					c.Request.Header.Set("Content-Type", "application/json")
					t.Cleanup(func() { common.CleanupBodyStorage(c) })
					common.SetContextKey(c, constant.ContextKeyOriginalModel, "gpt-4o-mini")
					common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeOpenAI)
					common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
					common.SetContextKey(c, constant.ContextKeyChannelKey, "fixture-key")
					common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{PassThroughBodyEnabled: mode == "channel passthrough"})
					common.SetContextKey(c, constant.ContextKeyChannelOtherSetting, dto.ChannelOtherSettings{AllowServiceTier: true})
					if mode == "override" {
						common.SetContextKey(c, constant.ContextKeyChannelParamOverride, map[string]any{"operations": []any{
							map[string]any{"path": "prompt_cache_key", "mode": "set", "value": overrideKey},
							map[string]any{"path": "service_tier", "mode": "set", "value": "fast"},
						}})
					}
					steps, record, apiErr := requestplugin.Ingress(c, format)
					require.Nil(t, apiErr)
					var info *relaycommon.RelayInfo
					var sent []byte
					if route == "chat" {
						request, parseErr := helper.GetAndValidateTextRequest(c, relayconstant.RelayModeChatCompletions)
						require.NoError(t, parseErr)
						info = relaycommon.GenRelayInfoOpenAI(c, request)
						info.SetRequestPluginIngress(steps, record)
						apiErr = TextHelper(c, info)
					} else {
						request, parseErr := helper.GetAndValidateResponsesRequest(c)
						require.NoError(t, parseErr)
						info = relaycommon.GenRelayInfoResponses(c, request)
						info.SetRequestPluginIngress(steps, record)
						if route == "websocket" {
							payload, buildErr := buildResponsesWSCreatePayload(c, info, *request, common.RawMessage(`false`), "normalized-wire")
							require.Nil(t, buildErr)
							client, peer, cleanup := newTestWebSocketPair(t)
							defer cleanup()
							require.NoError(t, client.WriteMessage(websocket.TextMessage, payload))
							kind, wire, readErr := peer.ReadMessage()
							require.NoError(t, readErr)
							assert.Equal(t, websocket.TextMessage, kind)
							assert.Equal(t, payload, wire)
							sent = wire
						} else {
							apiErr = ResponsesHelper(c, info)
						}
					}
					if route != "websocket" {
						require.NotNil(t, apiErr)
						require.Equal(t, http.StatusBadGateway, apiErr.StatusCode, "normalized egress must permit the actual upstream request")
						require.Len(t, captured, 1)
						sent = <-captured
					}
					assert.Equal(t, "priority", gjson.GetBytes(sent, "service_tier").String())
					assert.Equal(t, wantKey, gjson.GetBytes(sent, "prompt_cache_key").String())
					assert.Contains(t, string(sent), `"properties":{"z":{"const":9007199254740993},"a":{"const":0}}`)
					assert.Equal(t, "0", gjson.GetBytes(sent, "temperature").Raw)
					assert.Equal(t, "false", gjson.GetBytes(sent, "store").Raw)
					egressRuns := 0
					for _, step := range info.RequestPluginSteps {
						if step.Stage == requestplugin.StageEgress {
							egressRuns++
							assert.Equal(t, "continue", step.Action)
							require.Len(t, step.Findings, 1)
							assert.Equal(t, "normalized.wire", step.Findings[0].Category)
						}
					}
					assert.Equal(t, 1, egressRuns, "inspect the final wire body exactly once")
					if passthrough {
						uncheckedOutbound := false
						for _, entry := range info.RequestPluginRecord {
							uncheckedOutbound = uncheckedOutbound || entry.Stage == requestplugin.StageOutbound && entry.Unchecked
						}
						assert.True(t, uncheckedOutbound, "passthrough does not grant outbound mutation permission")
						if route != "websocket" {
							want := strings.Replace(raw, `"fast"`, `"priority"`, 1)
							want = strings.Replace(want, clientKey, wantKey, 1)
							assert.Equal(t, want, string(sent), "raw bytes change only at gateway-owned scalars")
						}
					}
					storage, storageErr := common.GetBodyStorage(c)
					require.NoError(t, storageErr)
					assert.Equal(t, disk, storage.IsDisk())
					original, storageErr := storage.Bytes()
					require.NoError(t, storageErr)
					assert.Equal(t, raw, string(original), "outbound lifetime and edits must not change the incoming retry body")
				})
			}
		}
	}
}
