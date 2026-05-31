package chat

import (
	"context"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/centrifugal/centrifuge"
)

const (
	EventTypeConversationCreated = "conversation.created"
	EventTypeConversationUpdated = "conversation.updated"
	EventTypeMessageCreated      = "message.created"
	EventTypeMessageRead         = "message.read"
	EventTypeMessageRevoked      = "message.revoked"
	EventTypeMemberAdded         = "member.added"
	EventTypeMemberRemoved       = "member.removed"
	EventTypePresenceUpdated     = "presence.updated"
	EventTypeTypingUpdated       = "typing.updated"
)

type Event struct {
	Type              string                `json:"type"`
	ConversationID    int                   `json:"conversation_id,omitempty"`
	Conversation      *ConversationResponse `json:"conversation,omitempty"`
	Message           *MessageResponse      `json:"message,omitempty"`
	UserID            int                   `json:"user_id,omitempty"`
	MemberIDs         []int                 `json:"member_ids,omitempty"`
	LastReadMessageID int                   `json:"last_read_message_id,omitempty"`
	CreatedAt         int64                 `json:"created_at,omitempty"`
}

type Publisher interface {
	Publish(ctx context.Context, channel string, event Event) error
}

type NoopPublisher struct{}

func (NoopPublisher) Publish(_ context.Context, _ string, _ Event) error {
	return nil
}

type CentrifugePublisher struct {
	node *centrifuge.Node
}

func NewCentrifugePublisher(node *centrifuge.Node) *CentrifugePublisher {
	return &CentrifugePublisher{node: node}
}

func (publisher *CentrifugePublisher) Publish(_ context.Context, channel string, event Event) error {
	if publisher == nil || publisher.node == nil {
		return nil
	}
	payload, err := common.Marshal(event)
	if err != nil {
		return err
	}
	_, err = publisher.node.Publish(channel, payload)
	return err
}

func ConversationChannel(conversationID int) string {
	return fmt.Sprintf("conversation:%d", conversationID)
}

func UserChannel(userID int) string {
	return fmt.Sprintf("user:%d", userID)
}
