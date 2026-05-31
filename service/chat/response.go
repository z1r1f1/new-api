package chat

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

func (service *Service) decorateConversations(conversations []*model.ChatConversation, currentUserID int) ([]*ConversationResponse, error) {
	responses := make([]*ConversationResponse, 0, len(conversations))
	for _, conversation := range conversations {
		response, err := service.decorateConversation(conversation, currentUserID)
		if err != nil {
			return nil, err
		}
		responses = append(responses, response)
	}
	return responses, nil
}

func (service *Service) decorateConversation(conversation *model.ChatConversation, currentUserID int) (*ConversationResponse, error) {
	if conversation == nil {
		return nil, ErrInvalidRequest
	}
	members, err := model.ListConversationMembers(conversation.Id)
	if err != nil {
		return nil, err
	}
	memberIDs := make([]int, 0, len(members))
	for _, member := range members {
		memberIDs = append(memberIDs, member.UserId)
	}
	users, err := model.ListChatUsersByIDs(memberIDs)
	if err != nil {
		return nil, err
	}
	userByID := toUserSummaryMap(users)
	memberSummaries := make([]*UserSummary, 0, len(memberIDs))
	var peer *UserSummary
	for _, memberID := range memberIDs {
		user := userByID[memberID]
		if user == nil {
			continue
		}
		memberSummaries = append(memberSummaries, user)
		if memberID != currentUserID && peer == nil {
			peer = user
		}
	}

	readState, err := model.GetChatReadState(conversation.Id, currentUserID)
	if err != nil {
		return nil, err
	}
	unreadCount := 0
	lastReadMessageID := 0
	if readState != nil {
		unreadCount = readState.UnreadCount
		lastReadMessageID = readState.LastReadMessageId
	}

	return &ConversationResponse{
		Id:                conversation.Id,
		Type:              conversation.Type,
		Title:             conversation.Title,
		OwnerId:           conversation.OwnerId,
		DirectKey:         conversation.DirectKey,
		LastMessageId:     conversation.LastMessageId,
		LastMessageAt:     conversation.LastMessageAt,
		CreatedAt:         conversation.CreatedAt,
		UpdatedAt:         conversation.UpdatedAt,
		UnreadCount:       unreadCount,
		LastReadMessageId: lastReadMessageID,
		IsDefault:         conversation.DirectKey == model.ChatDefaultGroupDirectKey,
		Peer:              peer,
		Members:           memberSummaries,
	}, nil
}

func (service *Service) decorateMessages(messages []*model.ChatMessage) ([]*MessageResponse, error) {
	responses := make([]*MessageResponse, 0, len(messages))
	if len(messages) == 0 {
		return responses, nil
	}
	contexts := make(map[int]*messageDecorationContext)
	messagesByConversation := make(map[int][]*model.ChatMessage)
	for _, message := range messages {
		if message == nil {
			return nil, ErrInvalidRequest
		}
		messagesByConversation[message.ConversationId] = append(messagesByConversation[message.ConversationId], message)
	}
	for conversationID, conversationMessages := range messagesByConversation {
		decoration, err := newMessageDecorationContext(conversationID, conversationMessages)
		if err != nil {
			return nil, err
		}
		contexts[conversationID] = decoration
	}
	for _, message := range messages {
		responses = append(responses, decorateMessageWithContext(message, contexts[message.ConversationId]))
	}
	return responses, nil
}

func (service *Service) decorateMessage(message *model.ChatMessage) (*MessageResponse, error) {
	if message == nil {
		return nil, ErrInvalidRequest
	}
	decoration, err := newMessageDecorationContext(message.ConversationId, []*model.ChatMessage{message})
	if err != nil {
		return nil, err
	}
	return decorateMessageWithContext(message, decoration), nil
}

type messageDecorationContext struct {
	memberIDs         []int
	userByID          map[int]*UserSummary
	readStateByUserID map[int]*model.ChatReadState
}

func newMessageDecorationContext(conversationID int, messages []*model.ChatMessage) (*messageDecorationContext, error) {
	memberIDs, err := model.ConversationMemberIDsForChat(conversationID)
	if err != nil {
		return nil, err
	}
	for _, message := range messages {
		if message != nil && !containsPositiveID(memberIDs, message.SenderId) {
			memberIDs = append(memberIDs, message.SenderId)
		}
	}
	users, err := model.ListChatUsersByIDs(memberIDs)
	if err != nil {
		return nil, err
	}
	userByID := toUserSummaryMap(users)
	readStateByUserID, err := model.ListChatReadStatesForConversation(conversationID)
	if err != nil {
		return nil, err
	}
	return &messageDecorationContext{
		memberIDs:         memberIDs,
		userByID:          userByID,
		readStateByUserID: readStateByUserID,
	}, nil
}

func decorateMessageWithContext(message *model.ChatMessage, decoration *messageDecorationContext) *MessageResponse {
	readBy := make([]*UserSummary, 0, len(decoration.memberIDs))
	for _, memberID := range decoration.memberIDs {
		state := decoration.readStateByUserID[memberID]
		if state == nil || state.LastReadMessageId < message.Id {
			continue
		}
		user := decoration.userByID[memberID]
		if user != nil {
			readBy = append(readBy, user)
		}
	}

	sender := decoration.userByID[message.SenderId]
	senderUsername := ""
	senderDisplayName := ""
	if sender != nil {
		senderUsername = sender.Username
		senderDisplayName = sender.DisplayName
	}

	return &MessageResponse{
		Id:                message.Id,
		ConversationId:    message.ConversationId,
		SenderId:          message.SenderId,
		SenderUsername:    senderUsername,
		SenderDisplayName: senderDisplayName,
		MessageType:       message.MessageType,
		ClientMessageId:   message.ClientMessageId,
		Body:              message.Body,
		CreatedAt:         message.CreatedAt,
		UpdatedAt:         message.UpdatedAt,
		RevokedAt:         message.RevokedAt,
		RevokedBy:         message.RevokedBy,
		ReadBy:            readBy,
	}
}

func (service *Service) requireDirectConversationAllowed(currentUserID int, peerUserID int) error {
	if currentUserID == peerUserID {
		return ErrInvalidRequest
	}
	currentUser, err := model.GetUserById(currentUserID, false)
	if err != nil {
		return err
	}
	peerUser, err := model.GetUserById(peerUserID, false)
	if err != nil {
		return err
	}
	if currentUser.Status != common.UserStatusEnabled || peerUser.Status != common.UserStatusEnabled {
		return ErrForbidden
	}
	if currentUser.Role >= common.RoleAdminUser || peerUser.Role >= common.RoleAdminUser {
		return nil
	}
	return ErrForbidden
}

func containsPositiveID(ids []int, target int) bool {
	if target <= 0 {
		return false
	}
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
