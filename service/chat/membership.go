package chat

import "github.com/QuantumNous/new-api/model"

func (service *Service) refreshReadStatesAfterMessage(conversationID int, senderID int, messageID int) error {
	members, err := model.ListConversationMembers(conversationID)
	if err != nil {
		return err
	}
	recipientIDs := make([]int, 0, len(members))
	for _, member := range members {
		if member.UserId == senderID {
			continue
		}
		recipientIDs = append(recipientIDs, member.UserId)
	}
	if err := model.SetChatReadState(conversationID, senderID, messageID, 0); err != nil {
		return err
	}
	return model.IncrementChatUnreadStates(conversationID, recipientIDs)
}
