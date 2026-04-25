package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetEndpointTypesByChannelType_ChatGPTImage(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeChatGPTImage, "gpt-image-2")
	want := []constant.EndpointType{constant.EndpointTypeImageGeneration, constant.EndpointTypeOpenAI}
	if len(got) != len(want) {
		t.Fatalf("expected endpoint types %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected endpoint types %v, got %v", want, got)
		}
	}
}
