package relay

import (
	"strings"
	"testing"
)

func TestSanitizedRequestBodyForLogRedactsDataURL(t *testing.T) {
	body := []byte(`{"prompt":"see ![](data:image/png;base64,` + strings.Repeat("A", 4096) + `)","image":"data:image/jpeg;base64,` + strings.Repeat("B", 4096) + `"}`)

	got := sanitizedRequestBodyForLog(body)

	if strings.Contains(got, strings.Repeat("A", 256)) || strings.Contains(got, strings.Repeat("B", 256)) {
		t.Fatalf("sanitized log still contains long base64 payload: %s", got)
	}
	if !strings.Contains(got, "data:image/png;base64,[redacted") || !strings.Contains(got, "data:image/jpeg;base64,[redacted") {
		t.Fatalf("sanitized log did not include redaction markers: %s", got)
	}
}

func TestSanitizedRequestBodyForLogCapsLargeBody(t *testing.T) {
	body := []byte(`{"prompt":"` + strings.Repeat("x", maxRequestBodyLogBytes*2) + `"}`)

	got := sanitizedRequestBodyForLog(body)

	if len(got) > maxRequestBodyLogBytes+128 {
		t.Fatalf("sanitized log too large: %d", len(got))
	}
	if !strings.Contains(got, "truncated request body") {
		t.Fatalf("expected truncation marker, got: %s", got)
	}
}
