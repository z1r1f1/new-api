package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
)

func TestOaiResponsesStreamHandlerMarksCompletedAsDone(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = originalStreamingTimeout
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	body := strings.Join([]string{
		`data: {"type":"response.completed","response":{"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12,"input_tokens_details":{"cached_tokens":4}}}}`,
		`data: {"type":"response.output_text.delta","delta":"should not be scanned after completed"}`,
	}, "\n") + "\n"
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	info := &relaycommon.RelayInfo{IsStream: true}

	usage, err := OaiResponsesStreamHandler(c, info, resp)
	if err != nil {
		t.Fatalf("OaiResponsesStreamHandler returned error: %v", err)
	}
	if usage.PromptTokens != 10 {
		t.Fatalf("PromptTokens = %d, want 10", usage.PromptTokens)
	}
	if usage.CompletionTokens != 2 {
		t.Fatalf("CompletionTokens = %d, want 2", usage.CompletionTokens)
	}
	if usage.PromptTokensDetails.CachedTokens != 4 {
		t.Fatalf("CachedTokens = %d, want 4", usage.PromptTokensDetails.CachedTokens)
	}
	if info.StreamStatus == nil {
		t.Fatal("StreamStatus is nil")
	}
	if info.StreamStatus.EndReason != relaycommon.StreamEndReasonDone {
		t.Fatalf("EndReason = %q, want %q", info.StreamStatus.EndReason, relaycommon.StreamEndReasonDone)
	}
	if recorder.Body.String() == "" {
		t.Fatal("expected completed event to be forwarded")
	}
}
