package controller

import (
	"encoding/base64"
	"testing"
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
