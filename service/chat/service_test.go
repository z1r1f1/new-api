package chat

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type capturedPublish struct {
	Channel string
	Event   Event
}

type fakePublisher struct {
	items []capturedPublish
}

func (publisher *fakePublisher) Publish(_ context.Context, channel string, event Event) error {
	publisher.items = append(publisher.items, capturedPublish{Channel: channel, Event: event})
	return nil
}

func (publisher *fakePublisher) reset() {
	publisher.items = nil
}

func setupChatServiceTestDB(t *testing.T) {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(
		&model.ChatConversation{},
		&model.ChatConversationMember{},
		&model.ChatMessage{},
		&model.ChatReadState{},
	))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
	})
}

func setupChatServiceTestDBWithoutChatTables(t *testing.T) {
	t.Helper()

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL
	previousRedisEnabled := common.RedisEnabled

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	model.DB = db
	model.LOG_DB = db

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		common.RedisEnabled = previousRedisEnabled
	})
}

func TestServiceListConversationsCreatesMissingChatTables(t *testing.T) {
	setupChatServiceTestDBWithoutChatTables(t)

	svc := NewService(&fakePublisher{})

	conversations, err := svc.ListConversations(context.Background(), 1)
	require.NoError(t, err)
	assert.Empty(t, conversations)
	assert.True(t, model.DB.Migrator().HasTable(&model.ChatConversation{}))
	assert.True(t, model.DB.Migrator().HasTable(&model.ChatConversationMember{}))
	assert.True(t, model.DB.Migrator().HasTable(&model.ChatMessage{}))
	assert.True(t, model.DB.Migrator().HasTable(&model.ChatReadState{}))
}

func TestServiceRejectsNonMemberSend(t *testing.T) {
	setupChatServiceTestDB(t)

	conv, err := model.GetOrCreateDirectConversation(1, 2)
	require.NoError(t, err)
	publisher := &fakePublisher{}
	svc := NewService(publisher)

	_, err = svc.SendMessage(context.Background(), 3, conv.Id, "not allowed", "client-1")
	require.ErrorIs(t, err, ErrForbidden)
	assert.Empty(t, publisher.items)

	messages, err := model.ListConversationMessages(conv.Id, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func TestServiceCreatesDirectConversationOnce(t *testing.T) {
	setupChatServiceTestDB(t)

	svc := NewService(&fakePublisher{})
	first, err := svc.CreateDirectConversation(context.Background(), 7, 8)
	require.NoError(t, err)
	second, err := svc.CreateDirectConversation(context.Background(), 8, 7)
	require.NoError(t, err)

	assert.Equal(t, first.Id, second.Id)
	members, err := model.ListConversationMembers(first.Id)
	require.NoError(t, err)
	assert.Len(t, members, 2)
}

func TestServicePersistsMessageBeforePublish(t *testing.T) {
	setupChatServiceTestDB(t)

	publisher := &fakePublisher{}
	svc := NewService(publisher)
	conv, err := svc.CreateDirectConversation(context.Background(), 1, 2)
	require.NoError(t, err)
	publisher.reset()

	msg, err := svc.SendMessage(context.Background(), 1, conv.Id, "hello", "client-hello")
	require.NoError(t, err)
	require.NotNil(t, msg)

	messages, err := model.ListConversationMessages(conv.Id, 10, 0)
	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, msg.Id, messages[0].Id)

	require.Len(t, publisher.items, 1)
	assert.Equal(t, ConversationChannel(conv.Id), publisher.items[0].Channel)
	assert.Equal(t, EventTypeMessageCreated, publisher.items[0].Event.Type)
	assert.Equal(t, msg.Id, publisher.items[0].Event.Message.Id)
}

func TestServiceUpdatesUnreadAndReadState(t *testing.T) {
	setupChatServiceTestDB(t)

	svc := NewService(&fakePublisher{})
	conv, err := svc.CreateDirectConversation(context.Background(), 1, 2)
	require.NoError(t, err)
	msg, err := svc.SendMessage(context.Background(), 1, conv.Id, "hello", "client-hello")
	require.NoError(t, err)

	recipientState, err := model.GetChatReadState(conv.Id, 2)
	require.NoError(t, err)
	require.NotNil(t, recipientState)
	assert.Equal(t, 1, recipientState.UnreadCount)

	require.NoError(t, svc.MarkRead(context.Background(), 2, conv.Id, msg.Id))
	updatedState, err := model.GetChatReadState(conv.Id, 2)
	require.NoError(t, err)
	require.NotNil(t, updatedState)
	assert.Equal(t, msg.Id, updatedState.LastReadMessageId)
	assert.Equal(t, 0, updatedState.UnreadCount)
}

func TestServiceGroupMemberManagementRequiresOwner(t *testing.T) {
	setupChatServiceTestDB(t)

	svc := NewService(&fakePublisher{})
	conv, err := svc.CreateGroupConversation(context.Background(), 1, "team", []int{2})
	require.NoError(t, err)

	require.ErrorIs(t, svc.AddMember(context.Background(), 2, conv.Id, 3), ErrForbidden)
	require.NoError(t, svc.AddMember(context.Background(), 1, conv.Id, 3))

	member, err := model.IsConversationMember(conv.Id, 3)
	require.NoError(t, err)
	assert.True(t, member)

	require.ErrorIs(t, svc.RemoveMember(context.Background(), 2, conv.Id, 3), ErrForbidden)
	require.ErrorIs(t, svc.RemoveMember(context.Background(), 1, conv.Id, 1), ErrInvalidRequest)
	require.NoError(t, svc.RemoveMember(context.Background(), 1, conv.Id, 3))
}

func TestServiceRejectsMemberManagementOnDirectConversation(t *testing.T) {
	setupChatServiceTestDB(t)

	svc := NewService(&fakePublisher{})
	conv, err := svc.CreateDirectConversation(context.Background(), 1, 2)
	require.NoError(t, err)

	require.ErrorIs(t, svc.AddMember(context.Background(), 1, conv.Id, 3), ErrInvalidRequest)
	require.ErrorIs(t, svc.RemoveMember(context.Background(), 1, conv.Id, 2), ErrInvalidRequest)
}
