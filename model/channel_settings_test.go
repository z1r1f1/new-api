package model

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
)

func TestGetOtherSettingsDefaultsAllowServiceTierForCodex(t *testing.T) {
	channel := &Channel{Type: constant.ChannelTypeCodex, OtherSettings: "{}"}

	settings := channel.GetOtherSettings()

	if !settings.AllowServiceTier {
		t.Fatal("expected Codex channels to allow service_tier passthrough by default")
	}
}

func TestGetOtherSettingsDoesNotDefaultAllowServiceTierForNonCodex(t *testing.T) {
	channel := &Channel{Type: constant.ChannelTypeOpenAI, OtherSettings: "{}"}

	settings := channel.GetOtherSettings()

	if settings.AllowServiceTier {
		t.Fatal("expected non-Codex channels to keep service_tier passthrough disabled by default")
	}
}
