package relay

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestServeDataURLImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)

	payload := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	serveDataURLImage(ctx, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(payload))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "image/png", recorder.Header().Get("Content-Type"))
	require.Equal(t, payload, recorder.Body.Bytes())
}
