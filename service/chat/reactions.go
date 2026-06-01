package chat

import (
	"context"
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"gorm.io/gorm"
)

var allowedMessageReactionEmojis = map[string]struct{}{
	"👍": {}, "👎": {}, "❤️": {}, "😂": {}, "😮": {}, "😢": {}, "😡": {}, "🎉": {},
	"🔥": {}, "🚀": {}, "👏": {}, "🙌": {}, "🙏": {}, "🤝": {}, "💯": {}, "✨": {},
	"✅": {}, "❌": {}, "👀": {}, "🤔": {}, "😎": {}, "🥰": {}, "😍": {}, "🤩": {},
	"😅": {}, "🤣": {}, "😭": {}, "😤": {}, "😱": {}, "🫡": {}, "💪": {}, "🧠": {},
	"💡": {}, "⭐": {}, "🌟": {}, "🏆": {}, "📌": {}, "📝": {}, "🍻": {}, "☕": {},
	"🐶": {}, "🐱": {}, "🐼": {}, "🐧": {}, "🌈": {}, "⚡": {}, "💎": {}, "🎯": {},
}

func (service *Service) ToggleMessageReaction(ctx context.Context, currentUserID int, conversationID int, messageID int, emoji string) (*MessageResponse, bool, error) {
	if currentUserID <= 0 || conversationID <= 0 || messageID <= 0 {
		return nil, false, ErrInvalidRequest
	}
	normalizedEmoji, err := normalizeMessageReactionEmoji(emoji)
	if err != nil {
		return nil, false, err
	}
	if err := model.EnsureChatTables(); err != nil {
		return nil, false, err
	}
	if err := service.requireConversationMember(conversationID, currentUserID); err != nil {
		return nil, false, err
	}
	message, err := model.GetChatMessageByID(conversationID, messageID)
	if err != nil {
		return nil, false, err
	}
	if message.RevokedAt > 0 {
		return nil, false, ErrInvalidRequest
	}
	active, err := model.ToggleChatMessageReaction(conversationID, messageID, currentUserID, normalizedEmoji)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, err
		}
		return nil, false, err
	}
	updatedMessage, err := model.GetChatMessageByID(conversationID, messageID)
	if err != nil {
		return nil, false, err
	}
	response, err := service.decorateMessage(updatedMessage, currentUserID)
	if err != nil {
		return nil, false, err
	}
	eventMessage, err := service.decorateMessage(updatedMessage, 0)
	if err != nil {
		return nil, false, err
	}
	reactionActive := active
	service.publishBestEffort(ctx, ConversationChannel(conversationID), Event{
		Type:           EventTypeMessageReactionUpdated,
		ConversationID: conversationID,
		Message:        eventMessage,
		UserID:         currentUserID,
		ReactionEmoji:  normalizedEmoji,
		ReactionActive: &reactionActive,
		CreatedAt:      common.GetTimestamp(),
	})
	return response, active, nil
}

func normalizeMessageReactionEmoji(emoji string) (string, error) {
	normalizedEmoji, err := model.NormalizeChatMessageReactionEmoji(emoji)
	if err != nil {
		return "", ErrInvalidRequest
	}
	if _, ok := allowedMessageReactionEmojis[normalizedEmoji]; !ok {
		return "", ErrInvalidRequest
	}
	return normalizedEmoji, nil
}
