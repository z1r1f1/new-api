package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/require"
)

func TestSyncCodexChannelAccountTypeFromUsagePersistsPlanChange(t *testing.T) {
	db := setupModelListControllerTestDB(t)

	usageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/wham/usage", r.URL.Path)
		require.Equal(t, "Bearer access-token", r.Header.Get("Authorization"))
		require.Equal(t, "account-123", r.Header.Get("chatgpt-account-id"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"plan_type":"plus"}`))
	}))
	t.Cleanup(usageServer.Close)

	keyBytes, err := common.Marshal(map[string]string{
		"access_token": "access-token",
		"account_id":   "account-123",
	})
	require.NoError(t, err)

	otherInfoBytes, err := common.Marshal(map[string]any{
		"codex_account_type": "free",
	})
	require.NoError(t, err)

	channel := &model.Channel{
		Name:      "codex-account",
		Type:      constant.ChannelTypeCodex,
		Key:       string(keyBytes),
		BaseURL:   common.GetPointer(usageServer.URL),
		Models:    "gpt-5.3-codex",
		Status:    common.ChannelStatusEnabled,
		OtherInfo: string(otherInfoBytes),
	}
	require.NoError(t, db.Create(channel).Error)

	require.NoError(t, syncCodexChannelAccountTypeFromUsage(context.Background(), channel))

	var stored model.Channel
	require.NoError(t, db.First(&stored, channel.Id).Error)
	storedOtherInfo := stored.GetOtherInfo()
	require.Equal(t, "plus", storedOtherInfo["codex_account_type"])
	require.NotZero(t, storedOtherInfo["codex_account_type_updated_at"])
	require.Equal(t, "plus", channel.GetOtherInfo()["codex_account_type"])
}

func TestSyncCodexChannelAccountTypeFromUsageIgnoresNonCodexChannels(t *testing.T) {
	channel := &model.Channel{
		Type: constant.ChannelTypeOpenAI,
		Key:  "sk-test",
	}

	require.NoError(t, syncCodexChannelAccountTypeFromUsage(context.Background(), channel))
}
