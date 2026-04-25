package common

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetEndpointTypesByChannelType_ChatGPTImageImageModel(t *testing.T) {
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

func TestGetEndpointTypesByChannelType_ChatGPTImageTextModel(t *testing.T) {
	got := GetEndpointTypesByChannelType(constant.ChannelTypeChatGPTImage, "gpt-5.4-pro")
	want := []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeImageGeneration}
	if len(got) != len(want) {
		t.Fatalf("expected endpoint types %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected endpoint types %v, got %v", want, got)
		}
	}
}
