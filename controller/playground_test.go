package controller

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

func TestDecodePlaygroundImageDataURL(t *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(raw)

	imageBytes, contentType, err := decodePlaygroundImageDataURL(dataURL)
	if err != nil {
		t.Fatalf("decodePlaygroundImageDataURL returned error: %v", err)
	}
	if contentType != "image/png" {
		t.Fatalf("expected image/png content type, got %q", contentType)
	}
	if string(imageBytes) != string(raw) {
		t.Fatalf("decoded bytes mismatch: got %#v want %#v", imageBytes, raw)
	}
}

func TestServePlaygroundImageItemPrefersB64JSONOverURL(t *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	servePlaygroundImageItem(
		c,
		"https://chatgpt.com/backend-api/estuary/content?id=file_1&sig=x",
		base64.StdEncoding.EncodeToString(raw),
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "" {
		t.Fatalf("must not redirect when b64_json is available, got Location %q", location)
	}
	if recorder.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("expected image/png, got %q", recorder.Header().Get("Content-Type"))
	}
	if recorder.Body.String() != string(raw) {
		t.Fatalf("unexpected body bytes: %#v", recorder.Body.Bytes())
	}
}

func TestPlaygroundDebugMissIsNotHTTPError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("id", 42)
	c.Params = gin.Params{{Key: "debug_id", Value: "missing-debug-id"}}

	PlaygroundDebug(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status 200 for uncaptured debug data, got %d body=%s", recorder.Code, recorder.Body.String())
	}

	var body struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	if err := common.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}
	if body.Success {
		t.Fatalf("expected success=false for uncaptured debug data, body=%s", recorder.Body.String())
	}
	if !strings.Contains(body.Message, "not found") {
		t.Fatalf("expected not found message, got %q", body.Message)
	}
}
