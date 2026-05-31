package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const chatReadStateBatchSize = 200

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

func ListChatReadStatesForConversation(conversationID int) (map[int]*ChatReadState, error) {
	result := make(map[int]*ChatReadState)
	if conversationID <= 0 {
		return result, nil
	}
	var states []*ChatReadState
	if err := DB.Where("conversation_id = ?", conversationID).Find(&states).Error; err != nil {
		return nil, err
	}
	for _, state := range states {
		result[state.UserId] = state
	}
	return result, nil
}

func SetChatReadState(conversationID int, userID int, lastReadMessageID int, unreadCount int) error {
	if conversationID <= 0 || userID <= 0 {
		return errors.New("conversation and user ids are required")
	}
	if lastReadMessageID < 0 {
		lastReadMessageID = 0
	}
	if unreadCount < 0 {
		unreadCount = 0
	}

	now := common.GetTimestamp()
	state := ChatReadState{
		ConversationId:    conversationID,
		UserId:            userID,
		LastReadMessageId: lastReadMessageID,
		UnreadCount:       unreadCount,
		LastReadAt:        now,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"last_read_message_id",
			"unread_count",
			"last_read_at",
			"updated_at",
		}),
	}).Create(&state).Error
}

func IncrementChatUnreadStates(conversationID int, userIDs []int) error {
	if conversationID <= 0 {
		return errors.New("conversation id is required")
	}
	ids := normalizeConversationMemberIDs(userIDs...)
	if len(ids) == 0 {
		return nil
	}

	initialUnreadByUserID, err := initialUnreadCountsForUsers(conversationID, ids)
	if err != nil {
		return err
	}

	now := common.GetTimestamp()
	states := make([]*ChatReadState, 0, len(ids))
	for _, userID := range ids {
		states = append(states, &ChatReadState{
			ConversationId:    conversationID,
			UserId:            userID,
			LastReadMessageId: 0,
			UnreadCount:       initialUnreadByUserID[userID],
			LastReadAt:        now,
			CreatedAt:         now,
			UpdatedAt:         now,
		})
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(states); start += chatReadStateBatchSize {
			end := start + chatReadStateBatchSize
			if end > len(states) {
				end = len(states)
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"unread_count": gorm.Expr(chatUnreadCountIncrementSQL(), 1),
					"updated_at":   now,
				}),
			}).Create(states[start:end]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func chatUnreadCountIncrementSQL() string {
	if common.UsingPostgreSQL {
		return `"chat_read_states"."unread_count" + ?`
	}
	return "unread_count + ?"
}

func initialUnreadCountsForUsers(conversationID int, userIDs []int) (map[int]int, error) {
	result := make(map[int]int, len(userIDs))
	for _, userID := range userIDs {
		result[userID] = 0
	}
	if conversationID <= 0 || len(userIDs) == 0 {
		return result, nil
	}

	var totalMessages int64
	if err := DB.Model(&ChatMessage{}).
		Where("conversation_id = ?", conversationID).
		Count(&totalMessages).Error; err != nil {
		return nil, err
	}

	type senderMessageCount struct {
		SenderId int
		Count    int64
	}
	var rows []senderMessageCount
	if err := DB.Model(&ChatMessage{}).
		Select("sender_id, COUNT(*) AS count").
		Where("conversation_id = ? AND sender_id IN ?", conversationID, userIDs).
		Group("sender_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	sentByUserID := make(map[int]int64, len(rows))
	for _, row := range rows {
		sentByUserID[row.SenderId] = row.Count
	}
	for _, userID := range userIDs {
		unreadCount := totalMessages - sentByUserID[userID]
		if unreadCount < 0 {
			unreadCount = 0
		}
		result[userID] = int(unreadCount)
	}
	return result, nil
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
