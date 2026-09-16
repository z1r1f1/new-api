package common

import (
	"strings"
	"testing"
)

func TestMaskSensitiveInfoKeepsResponsesStreamEventNames(t *testing.T) {
	input := "responses stream error: response.failed; fallback event: response.error"
	got := MaskSensitiveInfo(input)
	for _, want := range []string{"response.failed", "response.error"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected masked text to preserve %q, got %q", want, got)
		}
	}
}

func TestMaskSensitiveInfoStillMasksDomainsUrlsAndIPs(t *testing.T) {
	input := "call api.openai.com via https://example.com/v1/test?key=secret and https://response.error/v1/test with api_key:sk-secret from 192.168.1.1"
	got := MaskSensitiveInfo(input)
	for _, want := range []string{"***.***.com", "https://***.com/***/***?key=***", "https://***.error/***/***", "api_key:***", "***.***.***.***"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected masked text to contain %q, got %q", want, got)
		}
	}
	for _, forbidden := range []string{"api.openai.com", "example.com/v1/test", "https://response.error/v1/test", "sk-secret", "192.168.1.1"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("expected masked text not to contain %q, got %q", forbidden, got)
		}
	}
}
