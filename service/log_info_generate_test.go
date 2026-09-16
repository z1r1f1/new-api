package service

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func TestGenerateTextOtherInfoRecordsFastServiceTierOnWhenRequestHasFast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"service_tier":"fast","model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{StartTime: now, FirstResponseTime: now, ChannelMeta: &relaycommon.ChannelMeta{}}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["fast_service_tier"] != true {
		t.Fatalf("expected fast_service_tier=true, got %#v", snapshot["fast_service_tier"])
	}
}

func TestGenerateTextOtherInfoRecordsFastServiceTierOffWhenRequestDoesNotHaveFast(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"service_tier":"auto","model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{StartTime: now, FirstResponseTime: now, ChannelMeta: &relaycommon.ChannelMeta{}}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["fast_service_tier"] != false {
		t.Fatalf("expected fast_service_tier=false, got %#v", snapshot["fast_service_tier"])
	}
	if snapshot["request_fast"] != false {
		t.Fatalf("expected request_fast=false, got %#v", snapshot["request_fast"])
	}
}

func TestGenerateTextOtherInfoRecordsRequestServiceTierFromFastAlias(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/message", strings.NewReader(`{"model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"model":"gpt-5.5","fast":true,"output_config":{"effort":"medium"}}`),
		},
	}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["request_service_tier"] != "priority" {
		t.Fatalf("expected fast alias to log request_service_tier=priority, got %#v", snapshot["request_service_tier"])
	}
	if snapshot["request_fast"] != true {
		t.Fatalf("expected request_fast=true, got %#v", snapshot["request_fast"])
	}
	if snapshot["request_fast_service_tier"] != "priority" {
		t.Fatalf("expected request_fast_service_tier=priority, got %#v", snapshot["request_fast_service_tier"])
	}
	if snapshot["request_effort"] != "medium" {
		t.Fatalf("expected request_effort=medium, got %#v", snapshot["request_effort"])
	}
}

func TestGenerateTextOtherInfoRecordsResponseServiceTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"service_tier":"priority","model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(ginKeyChannelAffinityLogInfo, map[string]interface{}{
		"request_debug": map[string]interface{}{
			"service_tier": "priority",
		},
		"final_request_debug": map[string]interface{}{
			"service_tier": "priority",
		},
		"response_debug": map[string]interface{}{
			"response": map[string]interface{}{
				"service_tier": "default",
			},
		},
	})
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{StartTime: now, FirstResponseTime: now, ChannelMeta: &relaycommon.ChannelMeta{}}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["request_service_tier"] != "priority" {
		t.Fatalf("expected request_service_tier=priority, got %#v", snapshot["request_service_tier"])
	}
	if snapshot["response_service_tier"] != "default" {
		t.Fatalf("expected response_service_tier=default, got %#v", snapshot["response_service_tier"])
	}
}

func TestGenerateTextOtherInfoRecordsResponseServiceTierFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"service_tier":"priority","model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(ginKeyUpstreamResponseServiceTier, "default")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{StartTime: now, FirstResponseTime: now, ChannelMeta: &relaycommon.ChannelMeta{}}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["response_service_tier"] != "default" {
		t.Fatalf("expected response_service_tier=default from context, got %#v", snapshot["response_service_tier"])
	}
}

func TestGenerateTextOtherInfoRecordsRequestEffortAndServiceTierFromBillingInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"model":"gpt-5.5","service_tier":"priority","reasoning":{"effort":"high"}}`),
		},
	}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["request_service_tier"] != "priority" {
		t.Fatalf("expected request_service_tier=priority, got %#v", snapshot["request_service_tier"])
	}
	if snapshot["request_effort"] != "high" {
		t.Fatalf("expected request_effort=high, got %#v", snapshot["request_effort"])
	}
	if snapshot["request_fast"] != true {
		t.Fatalf("expected request_fast=true when request_service_tier=priority, got %#v", snapshot["request_fast"])
	}
	if snapshot["request_fast_service_tier"] != "priority" {
		t.Fatalf("expected request_fast_service_tier=priority, got %#v", snapshot["request_fast_service_tier"])
	}
}

func TestGenerateTextOtherInfoRecordsRequestEffortFromOutputConfig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/messages", strings.NewReader(`{"model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"model":"gpt-5.5","output_config":{"effort":"xhigh"}}`),
		},
	}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["request_effort"] != "xhigh" {
		t.Fatalf("expected request_effort=xhigh, got %#v", snapshot["request_effort"])
	}
}

func TestGenerateTextOtherInfoRecordsFastConversionAndResponseTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set(headerClaudeCodeProxyFast, "true")
	ctx.Request.Header.Set(headerClaudeCodeProxyFastServiceTier, "priority")
	ctx.Set(ginKeyUpstreamResponseServiceTier, "default")
	now := time.Now()
	relayInfo := &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"model":"gpt-5.5","service_tier":"priority","reasoning":{"effort":"medium"}}`),
		},
	}

	other := GenerateTextOtherInfo(ctx, relayInfo, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	if snapshot["request_fast"] != true {
		t.Fatalf("expected request_fast=true, got %#v", snapshot["request_fast"])
	}
	if snapshot["request_fast_service_tier"] != "priority" {
		t.Fatalf("expected request_fast_service_tier=priority, got %#v", snapshot["request_fast_service_tier"])
	}
	if snapshot["request_service_tier"] != "priority" {
		t.Fatalf("expected request_service_tier=priority, got %#v", snapshot["request_service_tier"])
	}
	if snapshot["request_effort"] != "medium" {
		t.Fatalf("expected request_effort=medium, got %#v", snapshot["request_effort"])
	}
	if snapshot["response_service_tier"] != "default" {
		t.Fatalf("expected response_service_tier=default, got %#v", snapshot["response_service_tier"])
	}
}

func TestGenerateTextOtherInfoRecordsRequestProtocol(t *testing.T) {
	gin.SetMode(gin.TestMode)
	now := time.Now()

	httpCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	httpCtx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.5"}`))
	httpCtx.Request.Header.Set("Content-Type", "application/json")
	httpOther := GenerateTextOtherInfo(httpCtx, &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}, 1, 1, 1, 0, 0, 0, -1)
	httpSnapshot := httpOther.Snapshot()
	if httpSnapshot["request_protocol"] != "http" {
		t.Fatalf("expected request_protocol=http, got %#v", httpSnapshot["request_protocol"])
	}

	wsCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
	wsCtx.Request = httptest.NewRequest("GET", "/v1/responses", nil)
	wsCtx.Request.Header.Set("Connection", "Upgrade")
	wsCtx.Request.Header.Set("Upgrade", "websocket")
	wsCtx.Request.Header.Set("Sec-WebSocket-Key", "test")
	wsOther := GenerateTextOtherInfo(wsCtx, &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}, 1, 1, 1, 0, 0, 0, -1)
	wsSnapshot := wsOther.Snapshot()
	if wsSnapshot["request_protocol"] != "websocket" {
		t.Fatalf("expected request_protocol=websocket, got %#v", wsSnapshot["request_protocol"])
	}
}

func TestGenerateTextOtherInfoRecordsSafeRequestHeadersInAdminInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ctx.Request = httptest.NewRequest("POST", "/v1/responses", strings.NewReader(`{"model":"gpt-5.5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Request.Header.Set("User-Agent", "codex-test")
	ctx.Request.Header.Set("X-Client-Request-Id", "req-1")
	ctx.Request.Header.Set("Authorization", "Bearer secret")
	ctx.Request.Header.Set("Cookie", "sid=secret")
	ctx.Request.Header.Set("Api-Key", "secret")
	ctx.Request.Header.Set("X-Session-Token", "secret")
	now := time.Now()

	other := GenerateTextOtherInfo(ctx, &relaycommon.RelayInfo{
		StartTime:         now,
		FirstResponseTime: now,
		ChannelMeta:       &relaycommon.ChannelMeta{},
	}, 1, 1, 1, 0, 0, 0, -1)
	snapshot := other.Snapshot()

	adminInfo, ok := snapshot["admin_info"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected admin_info map, got %#v", snapshot["admin_info"])
	}
	headers, ok := adminInfo["request_headers"].(map[string]string)
	if !ok {
		t.Fatalf("expected admin request_headers map, got %#v", adminInfo["request_headers"])
	}
	if headers["Content-Type"] != "application/json" {
		t.Fatalf("expected Content-Type header, got %#v", headers)
	}
	if headers["User-Agent"] != "codex-test" {
		t.Fatalf("expected User-Agent header, got %#v", headers)
	}
	if headers["X-Client-Request-Id"] != "req-1" {
		t.Fatalf("expected X-Client-Request-Id header, got %#v", headers)
	}
	for _, sensitive := range []string{"Authorization", "Cookie", "Api-Key", "X-Session-Token"} {
		if _, ok := headers[sensitive]; ok {
			t.Fatalf("expected sensitive header %s to be filtered, got %#v", sensitive, headers)
		}
	}
	if _, ok := snapshot["request_headers"]; ok {
		t.Fatalf("request_headers must stay under admin_info, got top-level %#v", snapshot["request_headers"])
	}
}
