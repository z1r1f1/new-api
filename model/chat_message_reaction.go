package model

import (
	"errors"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const ChatMessageReactionEmojiMaxLength = 16

type ChatMessageReaction struct {
	Id             int    `json:"id"`
	ConversationId int    `json:"conversation_id" gorm:"index;not null;uniqueIndex:idx_chat_message_reaction_unique"`
	MessageId      int    `json:"message_id" gorm:"index;not null;uniqueIndex:idx_chat_message_reaction_unique"`
	UserId         int    `json:"user_id" gorm:"index;not null;uniqueIndex:idx_chat_message_reaction_unique"`
	Emoji          string `json:"emoji" gorm:"type:varchar(32);not null;uniqueIndex:idx_chat_message_reaction_unique"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;not null"`
}

type ChatMessageReactionSummary struct {
	MessageId            int
	Emoji                string
	Count                int
	ReactedByCurrentUser bool
}

func (ChatMessageReaction) TableName() string {
	return "chat_message_reactions"
}

func (reaction *ChatMessageReaction) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if reaction.CreatedAt == 0 {
		reaction.CreatedAt = now
	}
	if reaction.UpdatedAt == 0 {
		reaction.UpdatedAt = now
	}
	return nil
}

func (reaction *ChatMessageReaction) BeforeUpdate(tx *gorm.DB) error {
	reaction.UpdatedAt = common.GetTimestamp()
	return nil
}

func NormalizeChatMessageReactionEmoji(emoji string) (string, error) {
	normalized := strings.TrimSpace(emoji)
	if normalized == "" {
		return "", errors.New("reaction emoji is required")
	}
	if utf8.RuneCountInString(normalized) > ChatMessageReactionEmojiMaxLength {
		return "", errors.New("reaction emoji is too long")
	}
	for _, r := range normalized {
		if unicode.IsControl(r) || unicode.IsSpace(r) {
			return "", errors.New("reaction emoji is invalid")
		}
	}
	return normalized, nil
}

func ToggleChatMessageReaction(conversationID int, messageID int, userID int, emoji string) (bool, error) {
	if conversationID <= 0 || messageID <= 0 || userID <= 0 {
		return false, errors.New("conversation, message, and user ids are required")
	}
	normalizedEmoji, err := NormalizeChatMessageReactionEmoji(emoji)
	if err != nil {
		return false, err
	}

	active := false
	err = DB.Transaction(func(tx *gorm.DB) error {
		var message ChatMessage
		if err := tx.Where("conversation_id = ? AND id = ? AND revoked_at = ?", conversationID, messageID, 0).
			First(&message).Error; err != nil {
			return err
		}

		var reaction ChatMessageReaction
		err := tx.Where(
			"conversation_id = ? AND message_id = ? AND user_id = ? AND emoji = ?",
			conversationID,
			messageID,
			userID,
			normalizedEmoji,
		).First(&reaction).Error
		if err == nil {
			active = false
			return tx.Delete(&reaction).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		active = true
		return tx.Create(&ChatMessageReaction{
			ConversationId: conversationID,
			MessageId:      messageID,
			UserId:         userID,
			Emoji:          normalizedEmoji,
		}).Error
	})
	return active, err
}

func ListChatMessageReactionSummaries(messageIDs []int, currentUserID int) (map[int][]*ChatMessageReactionSummary, error) {
	ids := normalizeChatMessageReactionIDs(messageIDs)
	result := make(map[int][]*ChatMessageReactionSummary, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	type countRow struct {
		MessageId int
		Emoji     string
		Count     int64
	}

	var rows []countRow
	if err := DB.Model(&ChatMessageReaction{}).
		Select("message_id, emoji, COUNT(*) AS count").
		Where("message_id IN ?", ids).
		Group("message_id, emoji").
		Order("message_id ASC, count DESC, emoji ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	reactedByUser := make(map[int]map[string]bool)
	if currentUserID > 0 {
		type userReactionRow struct {
			MessageId int
			Emoji     string
		}
		var userRows []userReactionRow
		if err := DB.Model(&ChatMessageReaction{}).
			Select("message_id, emoji").
			Where("message_id IN ? AND user_id = ?", ids, currentUserID).
			Scan(&userRows).Error; err != nil {
			return nil, err
		}
		for _, row := range userRows {
			if reactedByUser[row.MessageId] == nil {
				reactedByUser[row.MessageId] = make(map[string]bool)
			}
			reactedByUser[row.MessageId][row.Emoji] = true
		}
	}

	for _, row := range rows {
		if row.Count <= 0 {
			continue
		}
		summary := &ChatMessageReactionSummary{
			MessageId:            row.MessageId,
			Emoji:                row.Emoji,
			Count:                int(row.Count),
			ReactedByCurrentUser: reactedByUser[row.MessageId][row.Emoji],
		}
		result[row.MessageId] = append(result[row.MessageId], summary)
	}
	return result, nil
}

func deleteChatMessageReactions(conversationID int) error {
	if conversationID <= 0 {
		return nil
	}
	return DB.Where("conversation_id = ?", conversationID).Delete(&ChatMessageReaction{}).Error
}

func normalizeChatMessageReactionIDs(ids []int) []int {
	seen := make(map[int]struct{}, len(ids))
	result := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Ints(result)
	return result
}
