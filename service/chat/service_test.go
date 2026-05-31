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
		&model.User{},
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
	require.NoError(t, db.AutoMigrate(&model.User{}))

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

func seedChatServiceUser(t *testing.T, id int, username string, displayName string, role int, status int) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.User{
		Id:          id,
		Username:    username,
		Password:    "password-" + username,
		DisplayName: displayName,
		Role:        role,
		Status:      status,
		AffCode:     username + "-aff",
	}).Error)
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

func TestServiceListUsersReturnsEnabledPeers(t *testing.T) {
	setupChatServiceTestDB(t)
	seedChatServiceUser(t, 1, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "bob", "Bob", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 3, "charlie", "Charlie", common.RoleAdminUser, common.UserStatusDisabled)
	seedChatServiceUser(t, 4, "dave", "Dave", common.RoleCommonUser, common.UserStatusEnabled)

	svc := NewService(&fakePublisher{})

	users, err := svc.ListUsers(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, users, 1)
	assert.Equal(t, 2, users[0].Id)
	assert.Equal(t, "bob", users[0].Username)
	assert.Equal(t, "Bob", users[0].DisplayName)
}

func TestServiceListUsersForAdminReturnsAllEnabledPeers(t *testing.T) {
	setupChatServiceTestDB(t)
	seedChatServiceUser(t, 1, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 3, "bob", "Bob", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 4, "disabled", "Disabled", common.RoleCommonUser, common.UserStatusDisabled)

	svc := NewService(&fakePublisher{})

	users, err := svc.ListUsers(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, users, 2)
	assert.Equal(t, []int{2, 3}, []int{users[0].Id, users[1].Id})
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
	seedChatServiceUser(t, 7, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 8, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)

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
	seedChatServiceUser(t, 1, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)

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
	seedChatServiceUser(t, 1, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)

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

func TestServiceRejectsCommonUserDirectConversationWithCommonUser(t *testing.T) {
	setupChatServiceTestDB(t)
	seedChatServiceUser(t, 1, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "bob", "Bob", common.RoleCommonUser, common.UserStatusEnabled)

	svc := NewService(&fakePublisher{})

	_, err := svc.CreateDirectConversation(context.Background(), 1, 2)
	require.ErrorIs(t, err, ErrForbidden)
}

func TestServiceAllowsCommonUserDirectConversationWithAdmin(t *testing.T) {
	setupChatServiceTestDB(t)
	seedChatServiceUser(t, 1, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)

	svc := NewService(&fakePublisher{})

	conversation, err := svc.CreateDirectConversation(context.Background(), 1, 2)
	require.NoError(t, err)
	require.NotNil(t, conversation)
	assert.Equal(t, "admin", conversation.Peer.Username)
}

func TestServiceDefaultGroupIncludesAllEnabledUsers(t *testing.T) {
	setupChatServiceTestDB(t)
	seedChatServiceUser(t, 1, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "bob", "Bob", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 3, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 4, "disabled", "Disabled", common.RoleCommonUser, common.UserStatusDisabled)

	svc := NewService(&fakePublisher{})

	conversations, err := svc.ListConversations(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, conversations, 1)
	defaultGroup := conversations[0]
	require.True(t, defaultGroup.IsDefault)
	require.Equal(t, model.ChatConversationTypeGroup, defaultGroup.Type)
	require.Len(t, defaultGroup.Members, 3)
	assert.Equal(t, []int{1, 2, 3}, []int{
		defaultGroup.Members[0].Id,
		defaultGroup.Members[1].Id,
		defaultGroup.Members[2].Id,
	})
}

func TestServiceUnreadCountOnDefaultGroupResponse(t *testing.T) {
	setupChatServiceTestDB(t)
	seedChatServiceUser(t, 1, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "bob", "Bob", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 3, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)

	svc := NewService(&fakePublisher{})
	conversations, err := svc.ListConversations(context.Background(), 1)
	require.NoError(t, err)
	require.Len(t, conversations, 1)
	defaultGroupID := conversations[0].Id

	message, err := svc.SendMessage(context.Background(), 1, defaultGroupID, "hello group", "client-group")
	require.NoError(t, err)
	assert.Equal(t, "alice", message.SenderUsername)

	bobConversations, err := svc.ListConversations(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, bobConversations, 1)
	assert.Equal(t, 1, bobConversations[0].UnreadCount)

	require.NoError(t, svc.MarkRead(context.Background(), 2, defaultGroupID, message.Id))
	bobConversations, err = svc.ListConversations(context.Background(), 2)
	require.NoError(t, err)
	require.Len(t, bobConversations, 1)
	assert.Equal(t, 0, bobConversations[0].UnreadCount)
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
	seedChatServiceUser(t, 1, "admin", "Admin", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatServiceUser(t, 2, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)

	svc := NewService(&fakePublisher{})
	conv, err := svc.CreateDirectConversation(context.Background(), 1, 2)
	require.NoError(t, err)

	require.ErrorIs(t, svc.AddMember(context.Background(), 1, conv.Id, 3), ErrInvalidRequest)
	require.ErrorIs(t, svc.RemoveMember(context.Background(), 1, conv.Id, 2), ErrInvalidRequest)
}
