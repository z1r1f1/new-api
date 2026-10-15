package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOaiResponsesHandlerNormalizesNilOutputForSDKClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_test","object":"response","status":"completed","model":"gpt-5.5","output":null,"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`,
		)),
	}

	usage, err := OaiResponsesHandler(c, &relaycommon.RelayInfo{}, resp)
	if err != nil {
		t.Fatalf("OaiResponsesHandler returned error: %v", err)
	}
	if usage == nil || usage.TotalTokens != 2 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"output":[]`) {
		t.Fatalf("expected response output to be normalized to an empty array, got:\n%s", body)
	}
	if strings.Contains(body, `"output":null`) {
		t.Fatalf("expected response output not to be null, got:\n%s", body)
	}
}

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

func TestOaiResponsesStreamHandlerNormalizesCompletedOutputForSDKClients(t *testing.T) {
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
		`data: {"type":"response.output_text.delta","delta":"Hi"}`,
		`data: {"type":"response.completed","response":{"id":"resp_test","object":"response","status":"completed","model":"gpt-5.5","output":null,"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}}`,
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
	if usage == nil || usage.TotalTokens != 12 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	out := recorder.Body.String()
	if !strings.Contains(out, `"output":[]`) {
		t.Fatalf("expected completed response output to be normalized to an empty array, got:\n%s", out)
	}
	if strings.Contains(out, `"output":null`) {
		t.Fatalf("expected completed response output not to be null, got:\n%s", out)
	}
}

func TestOaiResponsesStreamHandlerIncludesResponseFailedDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalStreamingTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = originalStreamingTimeout
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, "https://private.example.com")
	common.SetContextKey(c, constant.ContextKeyUpstreamRequestOrigin, "https://route.example.net")

	body := strings.Join([]string{
		`data: {"type":"response.failed","response":{"status":"failed","error":{"message":"Upstream rejected the tool call at https://private.example.com/v1/responses; lookup route.example.net; see https://docs.example.org/errors","code":"tool_call_invalid","param":"https://route.example.net/private?key=secret"}}}`,
		``,
	}, "\n")
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	info := &relaycommon.RelayInfo{IsStream: true}

	_, err := OaiResponsesStreamHandler(c, info, resp)
	require.Error(t, err)
	errText := err.Error()
	for _, want := range []string{"response.failed", "Upstream rejected the tool call", "tool_call_invalid", "https://***.com/***/***", "lookup ***.***.net", "param=https://***.net/***?key=***", "https://docs.example.org/errors"} {
		assert.Contains(t, errText, want)
	}
	for _, forbidden := range []string{"private.example.com", "route.example.net", "secret"} {
		assert.NotContains(t, errText, forbidden)
	}
	require.NotNil(t, info.StreamStatus)
	if info.StreamStatus.EndError != nil {
		assert.Equal(t, errText, info.StreamStatus.EndError.Error())
	}
	require.NotEmpty(t, info.StreamStatus.Errors)
	for _, diagnostic := range info.StreamStatus.Errors {
		assert.NotContains(t, diagnostic.Message, "private.example.com")
		assert.NotContains(t, diagnostic.Message, "route.example.net")
	}
}
