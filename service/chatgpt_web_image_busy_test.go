package service

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestChatGPTWebImageBusyMemoryAcquireReleaseAndTTL(t *testing.T) {
	resetChatGPTWebImageBusyForTest()
	t.Cleanup(resetChatGPTWebImageBusyForTest)

	release, acquired := acquireChatGPTWebImageBusyWithTTL(1016, 40*time.Millisecond)
	require.True(t, acquired)
	require.True(t, IsChatGPTWebImageBusy(1016))

	_, acquiredAgain := acquireChatGPTWebImageBusyWithTTL(1016, time.Minute)
	require.False(t, acquiredAgain)

	release()
	require.False(t, IsChatGPTWebImageBusy(1016))

	release, acquired = acquireChatGPTWebImageBusyWithTTL(1016, 20*time.Millisecond)
	require.True(t, acquired)
	defer release()
	require.Eventually(t, func() bool {
		return !IsChatGPTWebImageBusy(1016)
	}, time.Second, 10*time.Millisecond)
}

func TestAcquireChatGPTWebImageBusyIsRequestScoped(t *testing.T) {
	resetChatGPTWebImageBusyForTest()
	t.Cleanup(resetChatGPTWebImageBusyForTest)

	oldRedisEnabled := common.RedisEnabled
	common.RedisEnabled = false
	t.Cleanup(func() {
		common.RedisEnabled = oldRedisEnabled
	})

	ctx := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	channel := &model.Channel{Id: 1016, Type: constant.ChannelTypeChatGPTImage}

	release := AcquireChatGPTWebImageBusy(ctx, channel, relayconstant.RelayModeImagesGenerations, "gpt-image-2")
	require.NotNil(t, release)
	require.True(t, IsChatGPTWebImageBusy(1016))

	require.Nil(t, AcquireChatGPTWebImageBusy(ctx, channel, relayconstant.RelayModeImagesGenerations, "gpt-image-2"))
	require.True(t, IsChatGPTWebImageBusy(1016))

	release()
	require.False(t, IsChatGPTWebImageBusy(1016))
}

func TestShouldUseChatGPTWebImageBusyAvoidance(t *testing.T) {
	require.True(t, ShouldUseChatGPTWebImageBusyAvoidance(relayconstant.RelayModeImagesGenerations, "gpt-image-2"))
	require.True(t, ShouldUseChatGPTWebImageBusyAvoidance(relayconstant.RelayModeImagesEdits, "chatgpt-image-2"))
	require.False(t, ShouldUseChatGPTWebImageBusyAvoidance(relayconstant.RelayModeImagesGenerations, "gpt-5.5"))
	require.False(t, ShouldUseChatGPTWebImageBusyAvoidance(0, "gpt-image-2"))
}

func TestCacheGetRandomSatisfiedChannelAvoidsBusyChatGPTWebImageChannel(t *testing.T) {
	resetChatGPTWebImageBusyForTest()
	t.Cleanup(resetChatGPTWebImageBusyForTest)

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM abilities").Error
		_ = model.DB.Exec("DELETE FROM channels").Error
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		if oldMemoryCacheEnabled {
			model.InitChannelCache()
		}
	})

	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)

	insertBusySelectionChannel(t, 1119, 10, 100)
	insertBusySelectionChannel(t, 1016, 5, 100)
	model.InitChannelCache()
	release, acquired := acquireChatGPTWebImageBusyWithTTL(1119, time.Minute)
	require.True(t, acquired)
	defer release()

	ch, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		TokenGroup: "svip",
		ModelName:  "gpt-image-2",
		RelayMode:  relayconstant.RelayModeImagesGenerations,
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.Equal(t, "svip", group)
	require.NotNil(t, ch)
	require.Equal(t, 1016, ch.Id)
}

func TestCacheGetRandomSatisfiedChannelAvoidsPreviouslyUsedChatGPTWebImageChannel(t *testing.T) {
	resetChatGPTWebImageBusyForTest()
	t.Cleanup(resetChatGPTWebImageBusyForTest)

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM abilities").Error
		_ = model.DB.Exec("DELETE FROM channels").Error
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		if oldMemoryCacheEnabled {
			model.InitChannelCache()
		}
	})

	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)

	insertBusySelectionChannel(t, 1119, 10, 100)
	insertBusySelectionChannel(t, 1016, 5, 100)
	model.InitChannelCache()

	ctx := gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New())
	ctx.Set("use_channel", []string{"1119"})

	ch, group, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        ctx,
		TokenGroup: "svip",
		ModelName:  "gpt-image-2",
		RelayMode:  relayconstant.RelayModeImagesGenerations,
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.Equal(t, "svip", group)
	require.NotNil(t, ch)
	require.Equal(t, 1016, ch.Id)
}

func TestCacheGetRandomSatisfiedChannelFallsBackWhenAllChatGPTWebImageChannelsBusy(t *testing.T) {
	resetChatGPTWebImageBusyForTest()
	t.Cleanup(resetChatGPTWebImageBusyForTest)

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM abilities").Error
		_ = model.DB.Exec("DELETE FROM channels").Error
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
		if oldMemoryCacheEnabled {
			model.InitChannelCache()
		}
	})

	require.NoError(t, model.DB.Exec("DELETE FROM abilities").Error)
	require.NoError(t, model.DB.Exec("DELETE FROM channels").Error)

	insertBusySelectionChannel(t, 1119, 10, 100)
	insertBusySelectionChannel(t, 1016, 5, 100)
	model.InitChannelCache()
	release1, acquired := acquireChatGPTWebImageBusyWithTTL(1119, time.Minute)
	require.True(t, acquired)
	defer release1()
	release2, acquired := acquireChatGPTWebImageBusyWithTTL(1016, time.Minute)
	require.True(t, acquired)
	defer release2()

	ch, _, err := CacheGetRandomSatisfiedChannel(&RetryParam{
		Ctx:        gin.CreateTestContextOnly(httptest.NewRecorder(), gin.New()),
		TokenGroup: "svip",
		ModelName:  "gpt-image-2",
		RelayMode:  relayconstant.RelayModeImagesGenerations,
		Retry:      common.GetPointer(0),
	})

	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 1119, ch.Id)
}

func insertBusySelectionChannel(t *testing.T, id int, priority int64, weight uint) {
	t.Helper()
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:       id,
		Type:     constant.ChannelTypeChatGPTImage,
		Key:      "test-key",
		Status:   common.ChannelStatusEnabled,
		Name:     "chatgpt-web",
		Priority: &priority,
		Weight:   &weight,
		Models:   "gpt-image-2",
		Group:    "svip",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "svip",
		Model:     "gpt-image-2",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}
