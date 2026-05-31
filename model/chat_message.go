package model

import (
	"errors"
	"sort"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

type ChatMessage struct {
	Id              int    `json:"id"`
	ConversationId  int    `json:"conversation_id" gorm:"index;not null"`
	SenderId        int    `json:"sender_id" gorm:"index;not null"`
	MessageType     string `json:"message_type" gorm:"type:varchar(32);not null;default:'text';index"`
	ClientMessageId string `json:"client_message_id" gorm:"type:varchar(64);default:'';index"`
	Body            string `json:"body" gorm:"type:text;not null"`
	CreatedAt       int64  `json:"created_at" gorm:"bigint;not null;index"`
	UpdatedAt       int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (ChatMessage) TableName() string {
	return "chat_messages"
}

func (message *ChatMessage) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if message.CreatedAt == 0 {
		message.CreatedAt = now
	}
	if message.UpdatedAt == 0 {
		message.UpdatedAt = now
	}
	if message.MessageType == "" {
		message.MessageType = ChatMessageTypeText
	}
	return nil
}

func (message *ChatMessage) BeforeUpdate(tx *gorm.DB) error {
	message.UpdatedAt = common.GetTimestamp()
	return nil
}

func InsertChatMessage(conversationID int, senderID int, messageType string, body string, clientMessageID string) (*ChatMessage, error) {
	if conversationID <= 0 || senderID <= 0 {
		return nil, errors.New("conversation and sender ids are required")
	}
	if body == "" {
		return nil, errors.New("message body is required")
	}
	if messageType == "" {
		messageType = ChatMessageTypeText
	}

	message := &ChatMessage{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		if clientMessageID != "" {
			if err := tx.Where("conversation_id = ? AND client_message_id = ?", conversationID, clientMessageID).First(message).Error; err == nil {
				return nil
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}

		message.ConversationId = conversationID
		message.SenderId = senderID
		message.MessageType = messageType
		message.Body = body
		message.ClientMessageId = clientMessageID
		if err := tx.Create(message).Error; err != nil {
			return err
		}
		return updateConversationLastMessage(tx, conversationID, message.Id, message.CreatedAt)
	})
	if err != nil {
		return nil, err
	}
	return message, nil
}

func ListConversationMessages(conversationID int, limit int, beforeMessageID int) ([]*ChatMessage, error) {
	if conversationID <= 0 {
		return []*ChatMessage{}, nil
	}
	if limit <= 0 {
		limit = 50
	}

	query := DB.Where("conversation_id = ?", conversationID)
	if beforeMessageID > 0 {
		query = query.Where("id < ?", beforeMessageID)
	}

	var messages []*ChatMessage
	if err := query.Order("id DESC").Limit(limit).Find(&messages).Error; err != nil {
		return nil, err
	}

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Id < messages[j].Id
	})
	return messages, nil
}

func ListConversationMessagesAfter(conversationID int, afterMessageID int, limit int) ([]*ChatMessage, error) {
	if conversationID <= 0 {
		return []*ChatMessage{}, nil
	}
	if limit <= 0 {
		limit = 50
	}
	var messages []*ChatMessage
	query := DB.Where("conversation_id = ?", conversationID)
	if afterMessageID > 0 {
		query = query.Where("id > ?", afterMessageID)
	}
	if err := query.Order("id ASC").Limit(limit).Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}
