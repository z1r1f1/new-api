package chat

import "github.com/QuantumNous/new-api/model"

func (service *Service) refreshReadStatesAfterMessage(conversationID int, senderID int, messageID int) error {
	members, err := model.ListConversationMembers(conversationID)
	if err != nil {
		return err
	}
	for _, member := range members {
		lastReadMessageID := 0
		if member.UserId == senderID {
			lastReadMessageID = messageID
		} else {
			state, err := model.GetChatReadState(conversationID, member.UserId)
			if err != nil {
				return err
			}
			if state != nil {
				lastReadMessageID = state.LastReadMessageId
			}
		}
		if err := model.UpsertChatReadState(conversationID, member.UserId, lastReadMessageID); err != nil {
			return err
		}
	}
	return nil
}
