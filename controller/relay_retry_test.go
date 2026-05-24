package controller

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/dto"
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
