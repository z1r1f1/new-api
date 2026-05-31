package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newChatGPTWebSessionChannelAffinityTestContext(target string, body string) *gin.Context {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(ctx, constant.ContextKeyUsingGroup, "vip")
	ctx.Set("original_model", "gpt-5.5-thinking")
	return ctx
}

func TestChatGPTWebSessionChannelAffinityUsesExplicitPromptCacheKey(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	sessionKey := fmt.Sprintf("session-channel-affinity-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"model":"gpt-5.5-thinking","prompt_cache_key":%q}`, sessionKey)
	first := newChatGPTWebSessionChannelAffinityTestContext("/v1/chat/completions", body)

	preferred, found := GetPreferredChatGPTWebSessionChannelByAffinity(first, "gpt-5.5-thinking", "vip")
	require.False(t, found)
	require.Zero(t, preferred)

	RecordChatGPTWebSessionChannelAffinity(first, &model.Channel{
		Id:     2468,
		Type:   constant.ChannelTypeChatGPTImage,
		Status: common.ChannelStatusEnabled,
	})
	t.Cleanup(func() { ClearCurrentChatGPTWebSessionChannelAffinity(first) })

	second := newChatGPTWebSessionChannelAffinityTestContext("/v1/chat/completions", body)
	preferred, found = GetPreferredChatGPTWebSessionChannelByAffinity(second, "gpt-5.5-thinking", "vip")
	require.True(t, found)
	require.Equal(t, 2468, preferred)
}

func TestChatGPTWebSessionChannelAffinityRequiresExplicitSessionKey(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	body := `{"model":"gpt-5.5-thinking","messages":[{"role":"user","content":"same opening text"}]}`
	first := newChatGPTWebSessionChannelAffinityTestContext("/v1/chat/completions", body)
	RecordChatGPTWebSessionChannelAffinity(first, &model.Channel{
		Id:     2469,
		Type:   constant.ChannelTypeChatGPTImage,
		Status: common.ChannelStatusEnabled,
	})

	second := newChatGPTWebSessionChannelAffinityTestContext("/v1/chat/completions", body)
	preferred, found := GetPreferredChatGPTWebSessionChannelByAffinity(second, "gpt-5.5-thinking", "vip")
	require.False(t, found)
	require.Zero(t, preferred)
}

func TestChatGPTWebSessionChannelAffinityDoesNotConflictWithConfiguredChannelAffinity(t *testing.T) {
	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() { common.RedisEnabled = oldRedisEnabled })

	sessionKey := fmt.Sprintf("session-channel-affinity-conflict-%d", time.Now().UnixNano())
	body := fmt.Sprintf(`{"model":"gpt-5.5-thinking","prompt_cache_key":%q}`, sessionKey)
	rule := operation_setting.ChannelAffinityRule{
		Name:       "configured-affinity-test",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/chat/completions"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "gjson", Path: "prompt_cache_key"},
		},
		IncludeRuleName:   true,
		IncludeUsingGroup: true,
	}
	genericSuffix := buildChannelAffinityCacheKeySuffix(rule, "gpt-5.5-thinking", "vip", sessionKey)
	genericCache := getChannelAffinityCache()
	require.NoError(t, genericCache.SetWithTTL(genericSuffix, 1111, time.Minute))
	t.Cleanup(func() { _, _ = genericCache.DeleteMany([]string{genericSuffix}) })

	setting := operation_setting.GetChannelAffinitySetting()
	originalRules := setting.Rules
	setting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() { setting.Rules = originalRules })

	chatGPTCtx := newChatGPTWebSessionChannelAffinityTestContext("/v1/chat/completions", body)
	RecordChatGPTWebSessionChannelAffinity(chatGPTCtx, &model.Channel{
		Id:     2222,
		Type:   constant.ChannelTypeChatGPTImage,
		Status: common.ChannelStatusEnabled,
	})
	t.Cleanup(func() { ClearCurrentChatGPTWebSessionChannelAffinity(chatGPTCtx) })

	ctx := newChatGPTWebSessionChannelAffinityTestContext("/v1/chat/completions", body)
	genericPreferred, genericFound := GetPreferredChannelByAffinity(ctx, "gpt-5.5-thinking", "vip")
	require.True(t, genericFound)
	require.Equal(t, 1111, genericPreferred)

	chatGPTPreferred, chatGPTFound := GetPreferredChatGPTWebSessionChannelByAffinity(ctx, "gpt-5.5-thinking", "vip")
	require.True(t, chatGPTFound)
	require.Equal(t, 2222, chatGPTPreferred)
}
