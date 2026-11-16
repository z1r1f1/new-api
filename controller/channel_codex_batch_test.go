package controller

import (
	"net/http"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

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

	t.Run("created IDs map every batch credential", func(t *testing.T) {
		setupTaskPluginBindChannelTest(t)
		response, raw := callChannelAPI(t, AddChannel, http.MethodPost, "/api/channel", `{"mode":"batch","channel":{"type":57,"key":"[{\"access_token\":\"token-a\",\"account_id\":\"account-a\",\"email\":\"a@example.com\"},{\"access_token\":\"token-b\",\"account_id\":\"account-b\",\"email\":\"b@example.com\"}]","models":"m","group":"default"}}`)
		require.True(t, response.Success, raw)
		var data struct {
			IDs     []int `json:"ids"`
			Results []struct {
				InputIndex int `json:"input_index"`
				ChannelID  int `json:"channel_id"`
			} `json:"results"`
		}
		require.NoError(t, common.Unmarshal(response.Data, &data))
		require.Len(t, data.IDs, 2)
		require.Len(t, data.Results, 2)
		for index, id := range data.IDs {
			require.Equal(t, index, data.Results[index].InputIndex)
			require.Equal(t, id, data.Results[index].ChannelID)
			channel, err := model.GetChannelById(id, true)
			require.NoError(t, err)
			require.Equal(t, []string{"a@example.com", "b@example.com"}[index], channel.Name)
			require.JSONEq(t, keys[index], channel.Key)
		}
	})
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
