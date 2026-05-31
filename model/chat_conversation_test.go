package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func truncateChatTables(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		DB.Exec("DELETE FROM chat_read_states")
		DB.Exec("DELETE FROM chat_messages")
		DB.Exec("DELETE FROM chat_conversation_members")
		DB.Exec("DELETE FROM chat_conversations")
	})
}

func setupChatTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	previousDB := DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false
	initCol()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	DB = db
	require.NoError(t, db.AutoMigrate(
		&ChatConversation{},
		&ChatConversationMember{},
		&ChatMessage{},
		&ChatReadState{},
	))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		DB = previousDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
		initCol()
	})

	return db
}

func TestChatDirectConversationReuse(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := GetOrCreateDirectConversation(10, 20)
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, ChatConversationTypeDirect, conv.Type)
	assert.NotZero(t, conv.Id)

	reused, err := GetOrCreateDirectConversation(20, 10)
	require.NoError(t, err)
	require.NotNil(t, reused)
	assert.Equal(t, conv.Id, reused.Id)
	assert.Equal(t, conv.DirectKey, reused.DirectKey)

	members, err := ListConversationMembers(conv.Id)
	require.NoError(t, err)
	assert.Len(t, members, 2)
	assert.ElementsMatch(t, []int{10, 20}, []int{members[0].UserId, members[1].UserId})
}

func TestChatGroupConversationMembers(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := CreateGroupConversation(1, "Alpha Team", []int{1, 2, 3})
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, ChatConversationTypeGroup, conv.Type)
	assert.Equal(t, "Alpha Team", conv.Title)

	members, err := ListConversationMembers(conv.Id)
	require.NoError(t, err)
	assert.Len(t, members, 3)
	assert.ElementsMatch(t, []int{1, 2, 3}, []int{members[0].UserId, members[1].UserId, members[2].UserId})
}

func TestChatConversationListForMember(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	direct, err := GetOrCreateDirectConversation(8, 9)
	require.NoError(t, err)
	group, err := CreateGroupConversation(8, "Ops", []int{8, 11})
	require.NoError(t, err)

	conversations, err := ListUserConversations(8)
	require.NoError(t, err)
	require.Len(t, conversations, 2)
	assert.ElementsMatch(t, []int{direct.Id, group.Id}, []int{conversations[0].Id, conversations[1].Id})
}

func TestChatReadStateUpsert(t *testing.T) {
	setupChatTestDB(t)
	truncateChatTables(t)

	conv, err := GetOrCreateDirectConversation(5, 6)
	require.NoError(t, err)

	msg, err := InsertChatMessage(conv.Id, 5, ChatMessageTypeText, "hello", "client-1")
	require.NoError(t, err)
	require.NotNil(t, msg)

	require.NoError(t, UpsertChatReadState(conv.Id, 6, msg.Id))
	require.NoError(t, UpsertChatReadState(conv.Id, 6, msg.Id))

	state, err := GetChatReadState(conv.Id, 6)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, msg.Id, state.LastReadMessageId)
	assert.Equal(t, 0, state.UnreadCount)
}
