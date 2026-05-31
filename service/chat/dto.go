package chat

import "github.com/QuantumNous/new-api/model"

type UserSummary struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
}

type ConversationResponse struct {
	Id                int                        `json:"id"`
	Type              model.ChatConversationType `json:"type"`
	Title             string                     `json:"title"`
	OwnerId           int                        `json:"owner_id"`
	DirectKey         string                     `json:"direct_key"`
	LastMessageId     int                        `json:"last_message_id"`
	LastMessageAt     int64                      `json:"last_message_at"`
	CreatedAt         int64                      `json:"created_at"`
	UpdatedAt         int64                      `json:"updated_at"`
	UnreadCount       int                        `json:"unread_count"`
	LastReadMessageId int                        `json:"last_read_message_id"`
	IsDefault         bool                       `json:"is_default"`
	Peer              *UserSummary               `json:"peer,omitempty"`
	Members           []*UserSummary             `json:"members"`
}

type MessageResponse struct {
	Id                int            `json:"id"`
	ConversationId    int            `json:"conversation_id"`
	SenderId          int            `json:"sender_id"`
	SenderUsername    string         `json:"sender_username"`
	SenderDisplayName string         `json:"sender_display_name"`
	MessageType       string         `json:"message_type"`
	ClientMessageId   string         `json:"client_message_id"`
	Body              string         `json:"body"`
	CreatedAt         int64          `json:"created_at"`
	UpdatedAt         int64          `json:"updated_at"`
	RevokedAt         int64          `json:"revoked_at"`
	RevokedBy         int            `json:"revoked_by"`
	ReadBy            []*UserSummary `json:"read_by"`
}

func toUserSummary(user *model.ChatUser) *UserSummary {
	if user == nil {
		return nil
	}
	return &UserSummary{
		Id:          user.Id,
		Username:    user.Username,
		DisplayName: user.DisplayName,
		Role:        user.Role,
	}
}

func toUserSummaryMap(users []*model.ChatUser) map[int]*UserSummary {
	result := make(map[int]*UserSummary, len(users))
	for _, user := range users {
		result[user.Id] = toUserSummary(user)
	}
	return result
}
