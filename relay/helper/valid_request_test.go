package helper

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func TestGetAndValidOpenAIImageRequestPreservesMultipartResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "chatgpt-image-2")
	_ = writer.WriteField("prompt", "edit this image")
	_ = writer.WriteField("response_format", "b64_json")
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	req, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesEdits)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if req.ResponseFormat != "b64_json" {
		t.Fatalf("expected response_format b64_json, got %q", req.ResponseFormat)
	}
}

func TestGetAndValidOpenAIImageRequestPreservesMultipartExtraFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("model", "gpt-image-2")
	_ = writer.WriteField("prompt", "edit this image")
	_ = writer.WriteField("conversation_id", "conv-123")
	_ = writer.WriteField("fallback_prompt", "full edit context")
	_ = writer.WriteField("fallback_reference_images", `["https://example.test/ref-a.png","https://example.test/ref-b.png"]`)
	_ = writer.WriteField("prompt_cache_key", "client-session-123")
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", &body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())

	req, err := GetAndValidOpenAIImageRequest(ctx, relayconstant.RelayModeImagesEdits)
	if err != nil {
		t.Fatalf("GetAndValidOpenAIImageRequest returned error: %v", err)
	}
	if req.Extra == nil {
		t.Fatal("expected multipart extra fields to be preserved")
	}
	for _, key := range []string{"conversation_id", "fallback_prompt", "fallback_reference_images", "prompt_cache_key"} {
		if len(req.Extra[key]) == 0 {
			t.Fatalf("expected extra field %q to be preserved, got %#v", key, req.Extra)
		}
	}
	var conversationID string
	if err := common.Unmarshal(req.Extra["conversation_id"], &conversationID); err != nil {
		t.Fatalf("conversation_id extra is not a JSON string: %v", err)
	}
	if conversationID != "conv-123" {
		t.Fatalf("expected conversation_id conv-123, got %q", conversationID)
	}
	var fallbackRefs []string
	if err := common.Unmarshal(req.Extra["fallback_reference_images"], &fallbackRefs); err != nil {
		t.Fatalf("fallback_reference_images extra should preserve JSON array: %v", err)
	}
	if len(fallbackRefs) != 2 || fallbackRefs[0] != "https://example.test/ref-a.png" {
		t.Fatalf("unexpected fallback refs: %#v", fallbackRefs)
	}
}
