package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestChatMessageOrdering(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := GetOrCreateDirectConversation(100, 200)
	require.NoError(t, err)

	first, err := InsertChatMessage(conv.Id, 100, ChatMessageTypeText, "first", "client-a")
	require.NoError(t, err)
	second, err := InsertChatMessage(conv.Id, 200, ChatMessageTypeText, "second", "client-b")
	require.NoError(t, err)
	third, err := InsertChatMessage(conv.Id, 100, ChatMessageTypeText, "third", "client-c")
	require.NoError(t, err)

	messages, err := ListConversationMessages(conv.Id, 10, 0)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	assert.Equal(t, []int{first.Id, second.Id, third.Id}, []int{messages[0].Id, messages[1].Id, messages[2].Id})
	assert.Equal(t, []string{"first", "second", "third"}, []string{messages[0].Body, messages[1].Body, messages[2].Body})
}

func TestChatMessageRevokeClearsBodyAndPersistsState(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := GetOrCreateDirectConversation(100, 200)
	require.NoError(t, err)

	message, err := InsertChatMessage(conv.Id, 100, ChatMessageTypeText, "secret", "client-revoke")
	require.NoError(t, err)
	require.Equal(t, int64(0), message.RevokedAt)
	require.Equal(t, 0, message.RevokedBy)

	revoked, err := RevokeChatMessage(conv.Id, message.Id, 100)
	require.NoError(t, err)
	require.NotNil(t, revoked)
	assert.Empty(t, revoked.Body)
	assert.Greater(t, revoked.RevokedAt, int64(0))
	assert.Equal(t, 100, revoked.RevokedBy)

	again, err := RevokeChatMessage(conv.Id, message.Id, 100)
	require.NoError(t, err)
	assert.Equal(t, revoked.RevokedAt, again.RevokedAt)
	assert.Equal(t, revoked.RevokedBy, again.RevokedBy)

	messages, err := ListConversationMessages(conv.Id, 10, 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Empty(t, messages[0].Body)
	assert.Equal(t, revoked.RevokedAt, messages[0].RevokedAt)
	assert.Equal(t, 100, messages[0].RevokedBy)
}

func TestGetChatMessageByIDRequiresConversationMatch(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	firstConversation, err := GetOrCreateDirectConversation(100, 200)
	require.NoError(t, err)
	secondConversation, err := GetOrCreateDirectConversation(100, 300)
	require.NoError(t, err)

	message, err := InsertChatMessage(firstConversation.Id, 100, ChatMessageTypeText, "hello", "client-lookup")
	require.NoError(t, err)

	_, err = GetChatMessageByID(secondConversation.Id, message.Id)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)

	found, err := GetChatMessageByID(firstConversation.Id, message.Id)
	require.NoError(t, err)
	assert.Equal(t, message.Id, found.Id)
}
