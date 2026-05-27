package model_setting

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/config"
)

func TestGlobalSettingsHTTPToWebsocketConversionDefaultAndConfigRoundTrip(t *testing.T) {
	settings := GetGlobalSettings()
	original := settings.HTTPToWebsocketConversionEnabled
	settings.HTTPToWebsocketConversionEnabled = false
	t.Cleanup(func() { settings.HTTPToWebsocketConversionEnabled = original })

	exported, err := config.ConfigToMap(settings)
	if err != nil {
		t.Fatalf("ConfigToMap returned error: %v", err)
	}
	if got := exported["http_to_websocket_conversion_enabled"]; got != "false" {
		t.Fatalf("expected default http_to_websocket_conversion_enabled=false, got %q", got)
	}

	if err := config.UpdateConfigFromMap(settings, map[string]string{
		"http_to_websocket_conversion_enabled": "true",
	}); err != nil {
		t.Fatalf("UpdateConfigFromMap returned error: %v", err)
	}
	if !settings.HTTPToWebsocketConversionEnabled {
		t.Fatal("expected global websocket conversion setting to update to true")
	}
}
