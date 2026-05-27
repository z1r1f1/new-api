package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func newGetModelRequestTestContext(method, target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx
}

func TestGetModelRequestReadsResponsesWebSocketCompatibilityModelFromQuery(t *testing.T) {
	ctx := newGetModelRequestTestContext(http.MethodGet, "/v1/responses?model=gpt-4o-realtime-preview")

	req, shouldSelectChannel, err := getModelRequest(ctx)
	if err != nil {
		t.Fatalf("expected responses websocket compatibility request to parse, got error: %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("expected responses websocket compatibility request to select a channel")
	}
	if req.Model != "gpt-4o-realtime-preview" {
		t.Fatalf("expected model from query, got %q", req.Model)
	}
}

func newChannelAffinityRecordTestContext(status int, info *relaycommon.RelayInfo) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if status != 0 {
		ctx.Status(status)
	}
	if info != nil {
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)
	}
	return ctx
}

func TestShouldRecordChannelAffinityAllowsMissingRelayInfo(t *testing.T) {
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, nil)

	if !shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record without relay info")
	}
}

func TestShouldRecordChannelAffinityRejectsHTTPErrorStatus(t *testing.T) {
	ctx := newChannelAffinityRecordTestContext(http.StatusInternalServerError, nil)

	if shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record to be skipped for HTTP error status")
	}
}

func TestShouldRecordChannelAffinityAllowsNormalStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: status,
	})

	if !shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record for normal stream")
	}
}

func TestShouldRecordChannelAffinityRejectsAbnormalStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonScannerErr, fmt.Errorf("stream ID 1: INTERNAL_ERROR"))
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: status,
	})

	if shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record to be skipped for scanner_error stream")
	}
}

func TestShouldRecordChannelAffinityRejectsSoftErrorStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	status.RecordError("chunk parse error")
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: status,
	})

	if shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record to be skipped for stream with soft errors")
	}
}
