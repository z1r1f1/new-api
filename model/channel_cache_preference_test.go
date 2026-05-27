package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/require"
)

func withMemoryChannelCache(t *testing.T, group string, modelName string, channels []*Channel) {
	t.Helper()

	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldGroup2model2channels := group2model2channels
	oldChannelsIDM := channelsIDM

	common.MemoryCacheEnabled = true
	channelIDs := make([]int, 0, len(channels))
	channelMap := make(map[int]*Channel, len(channels))
	for _, ch := range channels {
		channelIDs = append(channelIDs, ch.Id)
		channelMap[ch.Id] = ch
	}

	channelSyncLock.Lock()
	group2model2channels = map[string]map[string][]int{
		group: {
			modelName: channelIDs,
		},
	}
	channelsIDM = channelMap
	channelSyncLock.Unlock()

	t.Cleanup(func() {
		channelSyncLock.Lock()
		group2model2channels = oldGroup2model2channels
		channelsIDM = oldChannelsIDM
		channelSyncLock.Unlock()
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
	})
}

func testChannel(id int, priority int64, weight uint) *Channel {
	return &Channel{
		Id:       id,
		Type:     constant.ChannelTypeChatGPTImage,
		Status:   common.ChannelStatusEnabled,
		Priority: &priority,
		Weight:   &weight,
	}
}

func TestGetRandomSatisfiedChannelWithPreferencePrefersIdleAcrossPriorities(t *testing.T) {
	const group = "svip"
	const modelName = "gpt-image-2"
	withMemoryChannelCache(t, group, modelName, []*Channel{
		testChannel(1, 10, 100),
		testChannel(2, 5, 100),
	})

	ch, err := GetRandomSatisfiedChannelWithPreference(group, modelName, 0, func(ch *Channel) bool {
		return ch.Id == 2
	})

	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 2, ch.Id)
}

func TestGetRandomSatisfiedChannelWithPreferenceFallsBackWhenAllRejected(t *testing.T) {
	const group = "svip"
	const modelName = "gpt-image-2"
	withMemoryChannelCache(t, group, modelName, []*Channel{
		testChannel(1, 10, 100),
		testChannel(2, 5, 100),
	})

	ch, err := GetRandomSatisfiedChannelWithPreference(group, modelName, 0, func(ch *Channel) bool {
		return false
	})

	require.NoError(t, err)
	require.NotNil(t, ch)
	require.Equal(t, 1, ch.Id)
}
