package chatgptimg

import "testing"

func TestNormalizeOAuthKeyAcceptsAccessToken(t *testing.T) {
	raw := `{"access_token":"token-a","email":"a@example.com"}`
	normalized, err := NormalizeOAuthKey(raw)
	if err != nil {
		t.Fatalf("NormalizeOAuthKey returned error: %v", err)
	}
	if normalized == "" {
		t.Fatal("expected normalized key to be non-empty")
	}
}

func TestNormalizeOAuthKeyAcceptsRefreshTokenWithoutAccessToken(t *testing.T) {
	raw := `{"refresh_token":"rt-a","client_id":"app_123"}`
	if _, err := NormalizeOAuthKey(raw); err != nil {
		t.Fatalf("NormalizeOAuthKey should accept refresh_token-only payloads: %v", err)
	}
}

func TestNormalizeOAuthKeyRejectsMissingTokens(t *testing.T) {
	raw := `{"email":"a@example.com"}`
	if _, err := NormalizeOAuthKey(raw); err == nil {
		t.Fatal("expected NormalizeOAuthKey to reject payload without any token material")
	}
}

func TestGetOAuthBatchKeys(t *testing.T) {
	keys, err := GetOAuthBatchKeys(`[
		{"access_token":"token-a","email":"a@example.com"},
		{"session_token":"session-b","email":"b@example.com"}
	]`)
	if err != nil {
		t.Fatalf("GetOAuthBatchKeys returned error: %v", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
}

func TestGetOAuthChannelNameUsesEmail(t *testing.T) {
	name := GetOAuthChannelName(`{"access_token":"token-a","email":"a@example.com"}`, "")
	if name != "a@example.com" {
		t.Fatalf("expected email-based channel name, got %q", name)
	}
}
