package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/router"
	authservice "github.com/QuantumNous/new-api/service"
	chatservice "github.com/QuantumNous/new-api/service/chat"
	"github.com/centrifugal/protocol"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type chatAPIResponse struct {
	Success bool            `json:"success"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func setupChatSmokeTestDB(t *testing.T) *gorm.DB {
	return setupChatSmokeTestDBWithMigration(t, true)
}

func setupChatSmokeTestDBWithoutChatTables(t *testing.T) *gorm.DB {
	return setupChatSmokeTestDBWithMigration(t, false)
}

func setupChatSmokeTestDBWithMigration(t *testing.T, migrateChatTables bool) *gorm.DB {
	t.Helper()

	gin.SetMode(gin.TestMode)

	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousMainDatabaseType := common.MainDatabaseType()
	previousRedisEnabled := common.RedisEnabled
	previousGlobalAPIRateLimitEnabled := common.GlobalApiRateLimitEnable
	previousSessionSecret := common.SessionSecret

	common.SetMainDatabaseType(common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.GlobalApiRateLimitEnable = false
	common.SessionSecret = "chat-smoke-test-secret"

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)

	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}))
	if migrateChatTables {
		require.NoError(t, db.AutoMigrate(
			&model.ChatConversation{},
			&model.ChatConversationMember{},
			&model.ChatMessage{},
			&model.ChatMessageReaction{},
			&model.ChatReadState{},
		))
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.SetMainDatabaseType(previousMainDatabaseType)
		common.RedisEnabled = previousRedisEnabled
		common.GlobalApiRateLimitEnable = previousGlobalAPIRateLimitEnabled
		common.SessionSecret = previousSessionSecret
	})

	return db
}

func TestChatUsersRouteListsEnabledPeers(t *testing.T) {
	setupChatSmokeTestDB(t)
	seedChatSmokeUser(t, 1, "alice", "Alice", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatSmokeUser(t, 2, "bob", "Bob", common.RoleAdminUser, common.UserStatusEnabled)
	seedChatSmokeUser(t, 3, "charlie", "Charlie", common.RoleAdminUser, common.UserStatusDisabled)
	seedChatSmokeUser(t, 4, "dave", "Dave", common.RoleCommonUser, common.UserStatusEnabled)
	accessToken := issueChatSmokeAccessToken(t, 1)

	gin.SetMode(gin.TestMode)
	root := gin.New()

	apiRouter := root.Group("/api")
	router.SetChatRouter(root, apiRouter)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/users", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()

	root.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)

	var apiResp chatAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &apiResp))
	require.True(t, apiResp.Success, apiResp.Message)

	var users []model.ChatUser
	require.NoError(t, common.Unmarshal(apiResp.Data, &users))
	require.Len(t, users, 1)
	require.Equal(t, 2, users[0].Id)
	require.Equal(t, "bob", users[0].Username)
	require.Equal(t, "Bob", users[0].DisplayName)
}

func TestChatConversationsRouteCreatesMissingChatTables(t *testing.T) {
	db := setupChatSmokeTestDBWithoutChatTables(t)
	seedChatSmokeUser(t, 1, "smoke-user-1", "Smoke User", common.RoleCommonUser, common.UserStatusEnabled)
	accessToken := issueChatSmokeAccessToken(t, 1)

	gin.SetMode(gin.TestMode)
	root := gin.New()

	apiRouter := root.Group("/api")
	router.SetChatRouter(root, apiRouter)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/conversations", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	recorder := httptest.NewRecorder()

	root.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusOK, recorder.Code)

	var apiResp chatAPIResponse
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &apiResp))
	require.True(t, apiResp.Success, apiResp.Message)
	var conversations []chatservice.ConversationResponse
	require.NoError(t, common.Unmarshal(apiResp.Data, &conversations))
	require.Len(t, conversations, 1)
	require.True(t, conversations[0].IsDefault)
	require.Equal(t, "__default_group__", conversations[0].DirectKey)
	require.True(t, db.Migrator().HasTable(&model.ChatConversation{}))
	require.True(t, db.Migrator().HasTable(&model.ChatConversationMember{}))
	require.True(t, db.Migrator().HasTable(&model.ChatMessage{}))
	require.True(t, db.Migrator().HasTable(&model.ChatMessageReaction{}))
	require.True(t, db.Migrator().HasTable(&model.ChatReadState{}))
}

func seedChatSmokeUser(t *testing.T, id int, username string, displayName string, role int, status int) {
	t.Helper()

	require.NoError(t, model.DB.Create(&model.User{
		Id:          id,
		Username:    username,
		Password:    "password-" + username,
		DisplayName: displayName,
		Role:        role,
		Status:      status,
		AuthVersion: 1,
		AffCode:     username + "-aff",
	}).Error)
}

func issueChatSmokeAccessToken(t *testing.T, userID int) string {
	t.Helper()

	bundle, err := authservice.CreateLoginSession(userID, "test", "127.0.0.1", "chat-smoke-test")
	require.NoError(t, err)
	require.NotEmpty(t, bundle.AccessToken)
	return bundle.AccessToken
}

func initializeChatSmokeTestRuntime(t *testing.T) *chatservice.RealtimeServer {
	t.Helper()

	server, err := chatservice.DefaultRealtimeServer()
	require.NoError(t, err)
	_, err = chatservice.DefaultService()
	require.NoError(t, err)

	t.Cleanup(func() {
		if server != nil && server.Node() != nil {
			_ = server.Node().Shutdown(context.Background())
		}
	})

	return server
}

func TestChatBrowserAndWebSocketSmoke(t *testing.T) {
	setupChatSmokeTestDB(t)
	seedChatSmokeUser(t, 1, "smoke-user-1", "Smoke User", common.RoleCommonUser, common.UserStatusEnabled)
	seedChatSmokeUser(t, 2, "smoke-admin", "Smoke Admin", common.RoleAdminUser, common.UserStatusEnabled)
	accessToken := issueChatSmokeAccessToken(t, 1)
	initializeChatSmokeTestRuntime(t)

	gin.SetMode(gin.TestMode)
	root := gin.New()

	apiRouter := root.Group("/api")
	router.SetChatRouter(root, apiRouter)

	server := httptest.NewServer(root)
	t.Cleanup(server.Close)

	httpClient := server.Client()
	unauthenticatedWS := dialChatWebSocket(t, server.URL)
	sendChatWSCommand(t, unauthenticatedWS, 1, map[string]any{
		"connect": map[string]any{},
	})
	unauthenticatedReply := readChatWSReply(t, unauthenticatedWS)
	require.NotNil(t, unauthenticatedReply.Error)
	require.EqualValues(t, 101, unauthenticatedReply.Error.Code)
	require.NoError(t, unauthenticatedWS.Close())

	wsConn := dialChatWebSocket(t, server.URL)
	t.Cleanup(func() {
		_ = wsConn.Close()
	})

	sendChatWSCommand(t, wsConn, 1, map[string]any{
		"connect": map[string]any{"token": accessToken},
	})
	connectReply := readChatWSReply(t, wsConn)
	require.NotNil(t, connectReply.Connect)
	require.Nil(t, connectReply.Error)

	sendChatWSCommand(t, wsConn, 2, map[string]any{
		"subscribe": map[string]any{
			"channel": chatservice.UserChannel(1),
		},
	})
	subscribeReply := readChatWSReply(t, wsConn)
	require.NotNil(t, subscribeReply.Subscribe)
	require.Nil(t, subscribeReply.Error)

	conversation := createChatConversationViaHTTP(t, httpClient, server.URL, accessToken, 2)
	require.NotZero(t, conversation.Id)

	userPublication := readChatWSReply(t, wsConn)
	require.NotNil(t, userPublication.Push)
	require.Equal(t, chatservice.UserChannel(1), userPublication.Push.Channel)
	require.NotNil(t, userPublication.Push.Pub)

	var conversationCreated chatservice.Event
	require.NoError(t, common.Unmarshal(userPublication.Push.Pub.Data, &conversationCreated))
	require.Equal(t, chatservice.EventTypeConversationCreated, conversationCreated.Type)
	require.NotNil(t, conversationCreated.Conversation)
	require.Equal(t, conversation.Id, conversationCreated.ConversationID)
	require.Equal(t, conversation.Id, conversationCreated.Conversation.Id)

	sendChatWSCommand(t, wsConn, 3, map[string]any{
		"subscribe": map[string]any{
			"channel": chatservice.ConversationChannel(conversation.Id),
		},
	})
	conversationSubscribeReply := readChatWSReply(t, wsConn)
	require.NotNil(t, conversationSubscribeReply.Subscribe)
	require.Nil(t, conversationSubscribeReply.Error)

	message := sendChatMessageViaHTTP(t, httpClient, server.URL, accessToken, conversation.Id, "hello from smoke test")
	require.NotZero(t, message.Id)
	require.Equal(t, "hello from smoke test", message.Body)

	messagePublication := readChatWSReply(t, wsConn)
	require.NotNil(t, messagePublication.Push)
	require.Equal(t, chatservice.ConversationChannel(conversation.Id), messagePublication.Push.Channel)
	require.NotNil(t, messagePublication.Push.Pub)

	var messageCreated chatservice.Event
	require.NoError(t, common.Unmarshal(messagePublication.Push.Pub.Data, &messageCreated))
	require.Equal(t, chatservice.EventTypeMessageCreated, messageCreated.Type)
	require.NotNil(t, messageCreated.Message)
	require.Equal(t, message.Id, messageCreated.Message.Id)
	require.Equal(t, message.Body, messageCreated.Message.Body)

	reacted := reactChatMessageViaHTTP(t, httpClient, server.URL, accessToken, conversation.Id, message.Id, "👍")
	require.Len(t, reacted.Reactions, 1)
	require.Equal(t, "👍", reacted.Reactions[0].Emoji)
	require.Equal(t, 1, reacted.Reactions[0].Count)
	require.True(t, reacted.Reactions[0].ReactedByMe)

	reactionPublication := readChatWSReply(t, wsConn)
	require.NotNil(t, reactionPublication.Push)
	require.Equal(t, chatservice.ConversationChannel(conversation.Id), reactionPublication.Push.Channel)
	require.NotNil(t, reactionPublication.Push.Pub)

	var messageReactionUpdated chatservice.Event
	require.NoError(t, common.Unmarshal(reactionPublication.Push.Pub.Data, &messageReactionUpdated))
	require.Equal(t, chatservice.EventTypeMessageReactionUpdated, messageReactionUpdated.Type)
	require.Equal(t, message.Id, messageReactionUpdated.Message.Id)
	require.Equal(t, "👍", messageReactionUpdated.ReactionEmoji)
	require.NotNil(t, messageReactionUpdated.ReactionActive)
	require.True(t, *messageReactionUpdated.ReactionActive)
	require.Len(t, messageReactionUpdated.Message.Reactions, 1)
	require.Equal(t, 1, messageReactionUpdated.Message.Reactions[0].Count)
	require.False(t, messageReactionUpdated.Message.Reactions[0].ReactedByMe)

	revoked := revokeChatMessageViaHTTP(t, httpClient, server.URL, accessToken, conversation.Id, message.Id)
	require.Equal(t, message.Id, revoked.Id)
	require.Empty(t, revoked.Body)
	require.Greater(t, revoked.RevokedAt, int64(0))
	require.Equal(t, 1, revoked.RevokedBy)

	revokedPublication := readChatWSReply(t, wsConn)
	require.NotNil(t, revokedPublication.Push)
	require.Equal(t, chatservice.ConversationChannel(conversation.Id), revokedPublication.Push.Channel)
	require.NotNil(t, revokedPublication.Push.Pub)

	var messageRevoked chatservice.Event
	require.NoError(t, common.Unmarshal(revokedPublication.Push.Pub.Data, &messageRevoked))
	require.Equal(t, chatservice.EventTypeMessageRevoked, messageRevoked.Type)
	require.NotNil(t, messageRevoked.Message)
	require.Equal(t, message.Id, messageRevoked.Message.Id)
	require.Empty(t, messageRevoked.Message.Body)
}

func createChatConversationViaHTTP(t *testing.T, client *http.Client, baseURL string, accessToken string, peerUserID int) *chatservice.ConversationResponse {
	t.Helper()

	payload, err := common.Marshal(map[string]any{"user_id": peerUserID})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat/conversations/direct", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { _ = resp.Body.Close() }()

	var apiResp chatAPIResponse
	require.NoError(t, common.DecodeJson(resp.Body, &apiResp))
	require.True(t, apiResp.Success, apiResp.Message)

	var conversation chatservice.ConversationResponse
	require.NoError(t, common.Unmarshal(apiResp.Data, &conversation))
	return &conversation
}

func sendChatMessageViaHTTP(t *testing.T, client *http.Client, baseURL string, accessToken string, conversationID int, body string) *chatservice.MessageResponse {
	t.Helper()

	payload, err := common.Marshal(map[string]any{
		"body":              body,
		"client_message_id": "smoke-client-message-1",
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat/conversations/"+strconv.Itoa(conversationID)+"/messages", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { _ = resp.Body.Close() }()

	var apiResp chatAPIResponse
	require.NoError(t, common.DecodeJson(resp.Body, &apiResp))
	require.True(t, apiResp.Success, apiResp.Message)

	var message chatservice.MessageResponse
	require.NoError(t, common.Unmarshal(apiResp.Data, &message))
	return &message
}

func reactChatMessageViaHTTP(t *testing.T, client *http.Client, baseURL string, accessToken string, conversationID int, messageID int, emoji string) *chatservice.MessageResponse {
	t.Helper()

	payload, err := common.Marshal(map[string]any{
		"emoji": emoji,
	})
	require.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat/conversations/"+strconv.Itoa(conversationID)+"/messages/"+strconv.Itoa(messageID)+"/reactions", bytes.NewReader(payload))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { _ = resp.Body.Close() }()

	var apiResp chatAPIResponse
	require.NoError(t, common.DecodeJson(resp.Body, &apiResp))
	require.True(t, apiResp.Success, apiResp.Message)

	var message chatservice.MessageResponse
	require.NoError(t, common.Unmarshal(apiResp.Data, &message))
	return &message
}

func revokeChatMessageViaHTTP(t *testing.T, client *http.Client, baseURL string, accessToken string, conversationID int, messageID int) *chatservice.MessageResponse {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/chat/conversations/"+strconv.Itoa(conversationID)+"/messages/"+strconv.Itoa(messageID)+"/revoke", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	defer func() { _ = resp.Body.Close() }()

	var apiResp chatAPIResponse
	require.NoError(t, common.DecodeJson(resp.Body, &apiResp))
	require.True(t, apiResp.Success, apiResp.Message)

	var message chatservice.MessageResponse
	require.NoError(t, common.Unmarshal(apiResp.Data, &message))
	return &message
}

func dialChatWebSocket(t *testing.T, baseURL string) *websocket.Conn {
	t.Helper()

	wsURL := "ws" + strings.TrimPrefix(baseURL, "http") + "/api/chat/ws"
	dialer := websocket.Dialer{
		Subprotocols: []string{"centrifuge-json"},
	}

	conn, resp, err := dialer.Dial(wsURL, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	return conn
}

func sendChatWSCommand(t *testing.T, conn *websocket.Conn, id uint32, body map[string]any) {
	t.Helper()

	command := map[string]any{
		"id": id,
	}
	for key, value := range body {
		command[key] = value
	}
	payload, err := common.Marshal(command)
	require.NoError(t, err)
	require.NoError(t, conn.WriteMessage(websocket.TextMessage, payload))
}

func readChatWSReply(t *testing.T, conn *websocket.Conn) protocol.Reply {
	t.Helper()

	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, payload, err := conn.ReadMessage()
	require.NoError(t, err)
	payload = bytes.TrimSpace(payload)
	require.NotEmpty(t, payload)

	var reply protocol.Reply
	if err := common.Unmarshal(payload, &reply); err == nil {
		return reply
	}

	for _, part := range bytes.Split(payload, []byte("\n")) {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		if err := common.Unmarshal(part, &reply); err == nil {
			return reply
		}
	}

	t.Fatalf("failed to decode websocket reply: %s", string(payload))
	return protocol.Reply{}
}
