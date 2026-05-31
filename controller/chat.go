package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	chatservice "github.com/QuantumNous/new-api/service/chat"
	"github.com/centrifugal/centrifuge"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type createDirectChatRequest struct {
	UserId int `json:"user_id"`
}

type createGroupChatRequest struct {
	Title     string `json:"title"`
	MemberIds []int  `json:"member_ids"`
}

type sendChatMessageRequest struct {
	Body            string `json:"body"`
	ClientMessageId string `json:"client_message_id"`
}

type markChatReadRequest struct {
	LastReadMessageId int `json:"last_read_message_id"`
}

type addChatMemberRequest struct {
	UserId int `json:"user_id"`
}

func ListChatConversations(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	conversations, err := service.ListConversations(c.Request.Context(), userId)
	if err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", conversations)
}

func CreateDirectChatConversation(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	var req createDirectChatRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		chatJSON(c, http.StatusBadRequest, false, "invalid request", nil)
		return
	}
	conversation, err := service.CreateDirectConversation(c.Request.Context(), userId, req.UserId)
	if err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", conversation)
}

func CreateGroupChatConversation(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	var req createGroupChatRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		chatJSON(c, http.StatusBadRequest, false, "invalid request", nil)
		return
	}
	conversation, err := service.CreateGroupConversation(c.Request.Context(), userId, req.Title, req.MemberIds)
	if err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", conversation)
}

func ListChatMessages(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	conversationId, ok := parseChatPathInt(c, "id")
	if !ok {
		chatJSON(c, http.StatusBadRequest, false, "invalid conversation id", nil)
		return
	}
	limit := parseChatQueryInt(c, "limit", 50)
	before := parseChatQueryInt(c, "before", 0)
	messages, err := service.ListMessages(c.Request.Context(), userId, conversationId, limit, before)
	if err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", messages)
}

func SendChatMessage(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	conversationId, ok := parseChatPathInt(c, "id")
	if !ok {
		chatJSON(c, http.StatusBadRequest, false, "invalid conversation id", nil)
		return
	}
	var req sendChatMessageRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		chatJSON(c, http.StatusBadRequest, false, "invalid request", nil)
		return
	}
	message, err := service.SendMessage(c.Request.Context(), userId, conversationId, req.Body, req.ClientMessageId)
	if err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", message)
}

func MarkChatRead(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	conversationId, ok := parseChatPathInt(c, "id")
	if !ok {
		chatJSON(c, http.StatusBadRequest, false, "invalid conversation id", nil)
		return
	}
	var req markChatReadRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		chatJSON(c, http.StatusBadRequest, false, "invalid request", nil)
		return
	}
	if err := service.MarkRead(c.Request.Context(), userId, conversationId, req.LastReadMessageId); err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", nil)
}

func AddChatMember(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	conversationId, ok := parseChatPathInt(c, "id")
	if !ok {
		chatJSON(c, http.StatusBadRequest, false, "invalid conversation id", nil)
		return
	}
	var req addChatMemberRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		chatJSON(c, http.StatusBadRequest, false, "invalid request", nil)
		return
	}
	if err := service.AddMember(c.Request.Context(), userId, conversationId, req.UserId); err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", nil)
}

func RemoveChatMember(c *gin.Context) {
	service, ok := getChatService(c)
	if !ok {
		return
	}
	userId, ok := currentChatUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	conversationId, ok := parseChatPathInt(c, "id")
	if !ok {
		chatJSON(c, http.StatusBadRequest, false, "invalid conversation id", nil)
		return
	}
	targetUserId, ok := parseChatPathInt(c, "user_id")
	if !ok {
		chatJSON(c, http.StatusBadRequest, false, "invalid user id", nil)
		return
	}
	if err := service.RemoveMember(c.Request.Context(), userId, conversationId, targetUserId); err != nil {
		writeChatError(c, err)
		return
	}
	chatJSON(c, http.StatusOK, true, "", nil)
}

func ChatWebSocket(c *gin.Context) {
	userId, ok := currentChatSessionUserID(c)
	if !ok {
		chatJSON(c, http.StatusUnauthorized, false, "not logged in", nil)
		return
	}
	server, err := chatservice.DefaultRealtimeServer()
	if err != nil {
		chatJSON(c, http.StatusInternalServerError, false, "chat realtime unavailable", nil)
		return
	}
	request := c.Request.WithContext(centrifuge.SetCredentials(c.Request.Context(), &centrifuge.Credentials{
		UserID: strconv.Itoa(userId),
	}))
	server.ServeHTTP(c.Writer, request)
}

func getChatService(c *gin.Context) (*chatservice.Service, bool) {
	service, err := chatservice.DefaultService()
	if err != nil {
		chatJSON(c, http.StatusInternalServerError, false, "chat service unavailable", nil)
		return nil, false
	}
	return service, true
}

func currentChatUserID(c *gin.Context) (int, bool) {
	userId := c.GetInt("id")
	if userId > 0 {
		return userId, true
	}
	value, exists := c.Get("id")
	if !exists {
		return 0, false
	}
	return normalizeChatUserID(value)
}

func currentChatSessionUserID(c *gin.Context) (int, bool) {
	session := sessions.Default(c)
	status, ok := normalizeChatUserID(session.Get("status"))
	if !ok || status != common.UserStatusEnabled {
		return 0, false
	}
	return normalizeChatUserID(session.Get("id"))
}

func normalizeChatUserID(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, typed > 0
	case int64:
		return int(typed), typed > 0
	case float64:
		id := int(typed)
		return id, id > 0
	case string:
		id, err := strconv.Atoi(typed)
		return id, err == nil && id > 0
	default:
		return 0, false
	}
}

func parseChatPathInt(c *gin.Context, key string) (int, bool) {
	value, err := strconv.Atoi(c.Param(key))
	return value, err == nil && value > 0
}

func parseChatQueryInt(c *gin.Context, key string, fallback int) int {
	value := c.Query(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func writeChatError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, chatservice.ErrInvalidRequest):
		chatJSON(c, http.StatusBadRequest, false, err.Error(), nil)
	case errors.Is(err, chatservice.ErrForbidden):
		chatJSON(c, http.StatusForbidden, false, err.Error(), nil)
	case errors.Is(err, gorm.ErrRecordNotFound):
		chatJSON(c, http.StatusNotFound, false, "not found", nil)
	default:
		chatJSON(c, http.StatusInternalServerError, false, "chat operation failed", nil)
	}
}

func chatJSON(c *gin.Context, status int, success bool, message string, data any) {
	c.JSON(status, gin.H{
		"success": success,
		"message": message,
		"data":    data,
	})
}
