package controller

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetCodexBatchKeysFromJSONArray(t *testing.T) {
	keys, err := getCodexBatchKeys(`[
		{"access_token":"token-a","account_id":"account-a","email":"a@example.com"},
		{"access_token":"token-b","account_id":"account-b","email":"b@example.com"}
	]`)
	require.NoError(t, err)
	require.Len(t, keys, 2)
	require.Contains(t, keys[0], `"access_token":"token-a"`)
	require.Contains(t, keys[0], `"account_id":"account-a"`)
	require.Contains(t, keys[1], `"access_token":"token-b"`)
	require.Contains(t, keys[1], `"account_id":"account-b"`)
}

func TestGetCodexBatchKeysRejectsJSONObjectInput(t *testing.T) {
	_, err := getCodexBatchKeys(`{"access_token":"token-a","account_id":"account-a"}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "JsonArray")
}

func TestGetCodexBatchKeysRejectsMissingAccountID(t *testing.T) {
	_, err := getCodexBatchKeys(`[{"access_token":"token-a"}]`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "account_id")
}

func TestGetCodexChannelNameUsesEmailWhenNameEmpty(t *testing.T) {
	name := getCodexChannelName(`{"access_token":"token-a","account_id":"account-a","email":"a@example.com"}`, "")
	require.Equal(t, "a@example.com", name)
}

func TestGetCodexChannelNamePreservesExplicitName(t *testing.T) {
	name := getCodexChannelName(`{"access_token":"token-a","account_id":"account-a","email":"a@example.com"}`, "manual-name")
	require.Equal(t, "manual-name", name)
}

func TestGetCodexChannelNameFallsBackToRandomCodexPrefix(t *testing.T) {
	name := getCodexChannelName(`{"access_token":"token-a","account_id":"account-a"}`, "")
	require.Regexp(t, `^codex-[a-z0-9]{6}$`, name)
}
