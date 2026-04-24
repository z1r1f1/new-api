package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

func TestValidateChannel_AllowsChatGPTImageAccessTokenOnly(t *testing.T) {
	ch := &model.Channel{
		Type: constant.ChannelTypeChatGPTImage,
		Key:  `{"access_token":"token-a","email":"a@example.com"}`,
	}
	if err := validateChannel(ch, true); err != nil {
		t.Fatalf("validateChannel returned error: %v", err)
	}
}

func TestValidateChannel_AllowsChatGPTImageRefreshTokenOnly(t *testing.T) {
	ch := &model.Channel{
		Type: constant.ChannelTypeChatGPTImage,
		Key:  `{"refresh_token":"rt-a","client_id":"app_123"}`,
	}
	if err := validateChannel(ch, true); err != nil {
		t.Fatalf("validateChannel returned error: %v", err)
	}
}
