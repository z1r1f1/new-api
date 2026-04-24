package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetEndpointTypesByChannelType_ChatGPTImage(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeChatGPTImage, "gpt-image-2")
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 endpoint type, got %v", got)
	}
	if got[0] != constant.EndpointTypeImageGeneration {
		t.Fatalf("expected image generation endpoint, got %v", got[0])
	}
}
