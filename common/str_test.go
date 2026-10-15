package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMaskSensitiveInfoUsesUpstreamContext(t *testing.T) {
	tests := []struct {
		name         string
		upstreamURLs []string
		input        string
		want         string
	}{
		{
			name:  "no context preserves domains and URLs",
			input: "call api.openai.com via https://example.com/v1/test and https://response.error/v1/test",
			want:  "call api.openai.com via https://example.com/v1/test and https://response.error/v1/test",
		},
		{
			name:         "only configured upstreams are masked",
			upstreamURLs: []string{"https://private.example.com", "wss://route.example.net"},
			input:        "call https://private.example.com/v1/responses?key=secret; lookup route.example.net; see https://docs.example.org/errors and support@example.org",
			want:         "call https://***.com/***/***?key=***; lookup ***.***.net; see https://docs.example.org/errors and support@example.org",
		},
		{
			name:  "credentials and IPs are masked without context",
			input: "https://alice:secret@docs.example.org/help?token=secret api_key:sk-secret from 192.168.1.1 and [2001:db8::1]:443",
			want:  "https://***@docs.example.org/help?token=*** api_key:*** from ***.***.***.*** and [***]:443",
		},
		{
			name:         "Responses event names survive contextual masking",
			upstreamURLs: []string{"https://private.example.com"},
			input:        "responses stream error: response.failed; fallback event: response.error; upstream private.example.com",
			want:         "responses stream error: response.failed; fallback event: response.error; upstream ***.***.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, MaskSensitiveInfo(tt.input, tt.upstreamURLs...))
		})
	}
}
