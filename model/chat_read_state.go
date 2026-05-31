package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChatReadState struct {
	Id                int   `json:"id"`
	ConversationId    int   `json:"conversation_id" gorm:"uniqueIndex:idx_chat_read_state,priority:1;index;not null"`
	UserId            int   `json:"user_id" gorm:"uniqueIndex:idx_chat_read_state,priority:2;index;not null"`
	LastReadMessageId int   `json:"last_read_message_id" gorm:"default:0"`
	UnreadCount       int   `json:"unread_count" gorm:"default:0"`
	LastReadAt        int64 `json:"last_read_at" gorm:"bigint;default:0;index"`
	CreatedAt         int64 `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt         int64 `json:"updated_at" gorm:"bigint;not null"`
}

func (ChatReadState) TableName() string {
	return "chat_read_states"
}

func (state *ChatReadState) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if state.CreatedAt == 0 {
		state.CreatedAt = now
	}
	if state.UpdatedAt == 0 {
		state.UpdatedAt = now
	}
	if state.LastReadAt == 0 {
		state.LastReadAt = now
	}
	return nil
}

func (state *ChatReadState) BeforeUpdate(tx *gorm.DB) error {
	state.UpdatedAt = common.GetTimestamp()
	return nil
}

func GetChatReadState(conversationID int, userID int) (*ChatReadState, error) {
	if conversationID <= 0 || userID <= 0 {
		return nil, nil
	}
	var state ChatReadState
	if err := DB.Where("conversation_id = ? AND user_id = ?", conversationID, userID).First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func UpsertChatReadState(conversationID int, userID int, lastReadMessageID int) error {
	if conversationID <= 0 || userID <= 0 {
		return errors.New("conversation and user ids are required")
	}
	if lastReadMessageID < 0 {
		lastReadMessageID = 0
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		var unreadCount int64
		if err := tx.Model(&ChatMessage{}).
			Where("conversation_id = ? AND id > ? AND sender_id <> ?", conversationID, lastReadMessageID, userID).
			Count(&unreadCount).Error; err != nil {
			return err
		}

		now := common.GetTimestamp()
		state := ChatReadState{
			ConversationId:    conversationID,
			UserId:            userID,
			LastReadMessageId: lastReadMessageID,
			UnreadCount:       int(unreadCount),
			LastReadAt:        now,
			CreatedAt:         now,
			UpdatedAt:         now,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"last_read_message_id",
				"unread_count",
				"last_read_at",
				"updated_at",
			}),
		}).Create(&state).Error
	})
}
