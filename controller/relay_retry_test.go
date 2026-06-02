package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestShouldRetryRetriesRequestTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	err := types.NewOpenAIError(
		errors.New("upstream returned request timeout"),
		types.ErrorCodeBadResponseStatusCode,
		http.StatusRequestTimeout,
	)

	require.True(t, shouldRetry(ctx, err, 1))
}

func TestShouldRetryTaskRelayRetriesRequestTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	taskErr := &dto.TaskError{StatusCode: http.StatusRequestTimeout}

	require.True(t, shouldRetryTaskRelay(ctx, 1, taskErr, 1))
}

func TestRelayRejectsGPTImage2OutsideImageEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(
		http.MethodPost,
		"/v1/chat/completions",
		strings.NewReader(`{"model":"gpt-image-2","messages":[{"role":"user","content":"draw a cat"}]}`),
	)
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Set(common.RequestIdKey, "test-request-id")
	common.SetContextKey(ctx, constant.ContextKeyOriginalModel, "gpt-image-2")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
	common.SetContextKey(ctx, constant.ContextKeyTokenGroup, "vip")

	Relay(ctx, types.RelayFormatOpenAI)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Code    string `json:"code"`
		} `json:"error"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &payload))
	require.Contains(t, payload.Error.Message, "gpt-image-2")
	require.Contains(t, payload.Error.Message, "/v1/images")
	require.Equal(t, string(types.ErrorCodeInvalidRequest), payload.Error.Code)
}

func TestGPTImage2EndpointValidationAllowsImageEndpoints(t *testing.T) {
	require.NoError(t, validateGPTImage2Endpoint(relayconstant.RelayModeImagesGenerations, "gpt-image-2"))
	require.NoError(t, validateGPTImage2Endpoint(relayconstant.RelayModeImagesEdits, "gpt-image-2"))
	require.NoError(t, validateGPTImage2Endpoint(relayconstant.RelayModeChatCompletions, "gpt-5.5-thinking"))
}

func TestGPTImage2EndpointValidationRejectsNonImageEndpoints(t *testing.T) {
	for _, relayMode := range []int{
		relayconstant.RelayModeChatCompletions,
		relayconstant.RelayModeResponses,
		relayconstant.RelayModeResponsesCompact,
		relayconstant.RelayModeCompletions,
	} {
		require.Error(t, validateGPTImage2Endpoint(relayMode, "gpt-image-2"))
	}
}
