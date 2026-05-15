package service

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
