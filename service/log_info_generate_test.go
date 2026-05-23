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

	if other["fast_service_tier"] != true {
		t.Fatalf("expected fast_service_tier=true, got %#v", other["fast_service_tier"])
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

	if other["fast_service_tier"] != false {
		t.Fatalf("expected fast_service_tier=false, got %#v", other["fast_service_tier"])
	}
	if other["request_fast"] != false {
		t.Fatalf("expected request_fast=false, got %#v", other["request_fast"])
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

	if other["request_service_tier"] != "priority" {
		t.Fatalf("expected request_service_tier=priority, got %#v", other["request_service_tier"])
	}
	if other["response_service_tier"] != "default" {
		t.Fatalf("expected response_service_tier=default, got %#v", other["response_service_tier"])
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

	if other["response_service_tier"] != "default" {
		t.Fatalf("expected response_service_tier=default from context, got %#v", other["response_service_tier"])
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

	if other["request_service_tier"] != "priority" {
		t.Fatalf("expected request_service_tier=priority, got %#v", other["request_service_tier"])
	}
	if other["request_effort"] != "high" {
		t.Fatalf("expected request_effort=high, got %#v", other["request_effort"])
	}
	if other["request_fast"] != true {
		t.Fatalf("expected request_fast=true when request_service_tier=priority, got %#v", other["request_fast"])
	}
	if other["request_fast_service_tier"] != "priority" {
		t.Fatalf("expected request_fast_service_tier=priority, got %#v", other["request_fast_service_tier"])
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

	if other["request_fast"] != true {
		t.Fatalf("expected request_fast=true, got %#v", other["request_fast"])
	}
	if other["request_fast_service_tier"] != "priority" {
		t.Fatalf("expected request_fast_service_tier=priority, got %#v", other["request_fast_service_tier"])
	}
	if other["request_service_tier"] != "priority" {
		t.Fatalf("expected request_service_tier=priority, got %#v", other["request_service_tier"])
	}
	if other["request_effort"] != "medium" {
		t.Fatalf("expected request_effort=medium, got %#v", other["request_effort"])
	}
	if other["response_service_tier"] != "default" {
		t.Fatalf("expected response_service_tier=default, got %#v", other["response_service_tier"])
	}
}
