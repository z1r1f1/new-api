package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
