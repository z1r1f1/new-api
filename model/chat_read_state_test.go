package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatReadStateIncrementUnreadStates(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := CreateGroupConversation(1, "team", []int{1, 2, 3})
	require.NoError(t, err)

	first, err := InsertChatMessage(conv.Id, 1, ChatMessageTypeText, "first", "client-first")
	require.NoError(t, err)
	require.NoError(t, SetChatReadState(conv.Id, 2, first.Id, 0))

	require.NoError(t, IncrementChatUnreadStates(conv.Id, []int{2, 3}))
	require.NoError(t, IncrementChatUnreadStates(conv.Id, []int{2, 3}))

	stateByUserID, err := ListChatReadStatesForConversation(conv.Id)
	require.NoError(t, err)

	require.NotNil(t, stateByUserID[2])
	assert.Equal(t, first.Id, stateByUserID[2].LastReadMessageId)
	assert.Equal(t, 2, stateByUserID[2].UnreadCount)

	require.NotNil(t, stateByUserID[3])
	assert.Equal(t, 0, stateByUserID[3].LastReadMessageId)
	assert.Equal(t, 2, stateByUserID[3].UnreadCount)
}

func TestChatReadStateIncrementInitializesMissingUnreadFromHistory(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := CreateGroupConversation(1, "team", []int{1, 2, 3})
	require.NoError(t, err)

	_, err = InsertChatMessage(conv.Id, 2, ChatMessageTypeText, "from bob", "client-bob")
	require.NoError(t, err)
	_, err = InsertChatMessage(conv.Id, 1, ChatMessageTypeText, "from alice", "client-alice")
	require.NoError(t, err)

	require.NoError(t, IncrementChatUnreadStates(conv.Id, []int{2, 3}))

	stateByUserID, err := ListChatReadStatesForConversation(conv.Id)
	require.NoError(t, err)

	require.NotNil(t, stateByUserID[2])
	assert.Equal(t, 1, stateByUserID[2].UnreadCount)

	require.NotNil(t, stateByUserID[3])
	assert.Equal(t, 2, stateByUserID[3].UnreadCount)
}

func TestChatUnreadCountIncrementSQLQualifiesPostgreSQLTable(t *testing.T) {
	previousUsingPostgreSQL := common.UsingPostgreSQL
	t.Cleanup(func() {
		common.UsingPostgreSQL = previousUsingPostgreSQL
	})

	common.UsingPostgreSQL = true
	assert.Equal(t, `"chat_read_states"."unread_count" + ?`, chatUnreadCountIncrementSQL())

	common.UsingPostgreSQL = false
	assert.Equal(t, "unread_count + ?", chatUnreadCountIncrementSQL())
}

func TestChatReadStateIncrementUnreadStatesBatchesLargeGroups(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	recipientIDs := make([]int, 0, chatReadStateBatchSize*2+5)
	for userID := 2; userID < chatReadStateBatchSize*2+7; userID++ {
		recipientIDs = append(recipientIDs, userID)
	}

	conv, err := CreateGroupConversation(1, "large team", recipientIDs)
	require.NoError(t, err)

	_, err = InsertChatMessage(conv.Id, 1, ChatMessageTypeText, "hello large group", "client-large-group")
	require.NoError(t, err)

	require.NoError(t, IncrementChatUnreadStates(conv.Id, recipientIDs))

	var stateCount int64
	require.NoError(t, DB.Model(&ChatReadState{}).
		Where("conversation_id = ? AND user_id IN ?", conv.Id, recipientIDs).
		Count(&stateCount).Error)
	assert.Equal(t, int64(len(recipientIDs)), stateCount)

	state, err := GetChatReadState(conv.Id, recipientIDs[len(recipientIDs)-1])
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, 1, state.UnreadCount)
}
