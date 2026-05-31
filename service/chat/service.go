package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
)

var (
	ErrForbidden      = errors.New("chat access forbidden")
	ErrInvalidRequest = errors.New("invalid chat request")
)

type Service struct {
	publisher Publisher
}

func NewService(publisher Publisher) *Service {
	if publisher == nil {
		publisher = NoopPublisher{}
	}
	return &Service{publisher: publisher}
}

func (service *Service) ListUsers(ctx context.Context, currentUserID int) ([]*model.ChatUser, error) {
	if currentUserID <= 0 {
		return nil, ErrInvalidRequest
	}
	return model.ListChatUsers(currentUserID)
}

func (service *Service) ListConversations(ctx context.Context, userID int) ([]*model.ChatConversation, error) {
	if userID <= 0 {
		return nil, ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return nil, err
	}
	return model.ListUserConversations(userID)
}

func (service *Service) CreateDirectConversation(ctx context.Context, currentUserID int, peerUserID int) (*model.ChatConversation, error) {
	if currentUserID <= 0 || peerUserID <= 0 {
		return nil, ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return nil, err
	}
	conversation, err := model.GetOrCreateDirectConversation(currentUserID, peerUserID)
	if err != nil {
		return nil, err
	}
	service.publishToUsers(ctx, []int{currentUserID, peerUserID}, Event{
		Type:           EventTypeConversationCreated,
		ConversationID: conversation.Id,
		Conversation:   conversation,
		MemberIDs:      []int{currentUserID, peerUserID},
		CreatedAt:      common.GetTimestamp(),
	})
	return conversation, nil
}

func (service *Service) CreateGroupConversation(ctx context.Context, ownerID int, title string, memberIDs []int) (*model.ChatConversation, error) {
	if ownerID <= 0 {
		return nil, ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return nil, err
	}
	conversation, err := model.CreateGroupConversation(ownerID, title, memberIDs)
	if err != nil {
		return nil, err
	}
	members, err := model.ListConversationMembers(conversation.Id)
	if err != nil {
		return nil, err
	}
	userIDs := make([]int, 0, len(members))
	for _, member := range members {
		userIDs = append(userIDs, member.UserId)
	}
	service.publishToUsers(ctx, userIDs, Event{
		Type:           EventTypeConversationCreated,
		ConversationID: conversation.Id,
		Conversation:   conversation,
		MemberIDs:      userIDs,
		CreatedAt:      common.GetTimestamp(),
	})
	return conversation, nil
}

func (service *Service) SendMessage(ctx context.Context, currentUserID int, conversationID int, body string, clientMessageID string) (*model.ChatMessage, error) {
	if currentUserID <= 0 || conversationID <= 0 || body == "" {
		return nil, ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return nil, err
	}
	if err := service.requireConversationMember(conversationID, currentUserID); err != nil {
		return nil, err
	}
	message, err := model.InsertChatMessage(conversationID, currentUserID, model.ChatMessageTypeText, body, clientMessageID)
	if err != nil {
		return nil, err
	}
	if err := service.refreshReadStatesAfterMessage(conversationID, currentUserID, message.Id); err != nil {
		return nil, err
	}
	service.publishBestEffort(ctx, ConversationChannel(conversationID), Event{
		Type:           EventTypeMessageCreated,
		ConversationID: conversationID,
		Message:        message,
		UserID:         currentUserID,
		CreatedAt:      message.CreatedAt,
	})
	return message, nil
}

func (service *Service) ListMessages(ctx context.Context, currentUserID int, conversationID int, limit int, beforeMessageID int) ([]*model.ChatMessage, error) {
	if currentUserID <= 0 || conversationID <= 0 {
		return nil, ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return nil, err
	}
	if err := service.requireConversationMember(conversationID, currentUserID); err != nil {
		return nil, err
	}
	return model.ListConversationMessages(conversationID, limit, beforeMessageID)
}

func (service *Service) MarkRead(ctx context.Context, currentUserID int, conversationID int, lastReadMessageID int) error {
	if currentUserID <= 0 || conversationID <= 0 {
		return ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return err
	}
	if err := service.requireConversationMember(conversationID, currentUserID); err != nil {
		return err
	}
	if err := model.UpsertChatReadState(conversationID, currentUserID, lastReadMessageID); err != nil {
		return err
	}
	service.publishBestEffort(ctx, ConversationChannel(conversationID), Event{
		Type:              EventTypeMessageRead,
		ConversationID:    conversationID,
		UserID:            currentUserID,
		LastReadMessageID: lastReadMessageID,
		CreatedAt:         common.GetTimestamp(),
	})
	return nil
}

func (service *Service) AddMember(ctx context.Context, currentUserID int, conversationID int, targetUserID int) error {
	if currentUserID <= 0 || conversationID <= 0 || targetUserID <= 0 {
		return ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return err
	}
	if err := service.requireConversationMember(conversationID, currentUserID); err != nil {
		return err
	}
	if err := service.requireGroupOwner(conversationID, currentUserID); err != nil {
		return err
	}
	if err := model.AddConversationMember(conversationID, targetUserID, model.ChatConversationMemberRoleMember); err != nil {
		return err
	}
	service.publishBestEffort(ctx, ConversationChannel(conversationID), Event{
		Type:           EventTypeMemberAdded,
		ConversationID: conversationID,
		UserID:         targetUserID,
		CreatedAt:      common.GetTimestamp(),
	})
	service.publishBestEffort(ctx, UserChannel(targetUserID), Event{
		Type:           EventTypeMemberAdded,
		ConversationID: conversationID,
		UserID:         targetUserID,
		CreatedAt:      common.GetTimestamp(),
	})
	return nil
}

func (service *Service) RemoveMember(ctx context.Context, currentUserID int, conversationID int, targetUserID int) error {
	if currentUserID <= 0 || conversationID <= 0 || targetUserID <= 0 {
		return ErrInvalidRequest
	}
	if err := model.EnsureChatTables(); err != nil {
		return err
	}
	if err := service.requireConversationMember(conversationID, currentUserID); err != nil {
		return err
	}
	if targetUserID == currentUserID {
		return ErrInvalidRequest
	}
	if err := service.requireGroupOwner(conversationID, currentUserID); err != nil {
		return err
	}
	if err := model.RemoveConversationMember(conversationID, targetUserID); err != nil {
		return err
	}
	service.publishBestEffort(ctx, ConversationChannel(conversationID), Event{
		Type:           EventTypeMemberRemoved,
		ConversationID: conversationID,
		UserID:         targetUserID,
		CreatedAt:      common.GetTimestamp(),
	})
	return nil
}

func (service *Service) requireGroupOwner(conversationID int, userID int) error {
	conversation, err := model.GetConversationByID(conversationID)
	if err != nil {
		return err
	}
	if conversation.Type != model.ChatConversationTypeGroup {
		return ErrInvalidRequest
	}
	if conversation.OwnerId != userID {
		return ErrForbidden
	}
	return nil
}

func (service *Service) requireConversationMember(conversationID int, userID int) error {
	member, err := model.IsConversationMember(conversationID, userID)
	if err != nil {
		return err
	}
	if !member {
		return ErrForbidden
	}
	return nil
}

func (service *Service) publishToUsers(ctx context.Context, userIDs []int, event Event) {
	for _, userID := range userIDs {
		if userID <= 0 {
			continue
		}
		service.publishBestEffort(ctx, UserChannel(userID), event)
	}
}

func (service *Service) publishBestEffort(ctx context.Context, channel string, event Event) {
	if service == nil || service.publisher == nil {
		return
	}
	if err := service.publisher.Publish(ctx, channel, event); err != nil {
		logger.LogWarn(ctx, fmt.Sprintf("chat realtime publish failed: channel=%s error=%s", channel, common.LocalLogPreview(err.Error())))
	}
}
