package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChatConversationMember struct {
	Id             int    `json:"id"`
	ConversationId int    `json:"conversation_id" gorm:"uniqueIndex:idx_chat_conversation_member,priority:1;index;not null"`
	UserId         int    `json:"user_id" gorm:"uniqueIndex:idx_chat_conversation_member,priority:2;index;not null"`
	MemberRole     string `json:"member_role" gorm:"type:varchar(16);not null;default:'member'"`
	Active         bool   `json:"active" gorm:"default:true;index"`
	JoinedAt       int64  `json:"joined_at" gorm:"bigint;not null"`
	LeftAt         int64  `json:"left_at" gorm:"bigint;default:0"`
	CreatedAt      int64  `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt      int64  `json:"updated_at" gorm:"bigint;not null"`
}

func (ChatConversationMember) TableName() string {
	return "chat_conversation_members"
}

func (member *ChatConversationMember) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if member.JoinedAt == 0 {
		member.JoinedAt = now
	}
	if member.CreatedAt == 0 {
		member.CreatedAt = now
	}
	if member.UpdatedAt == 0 {
		member.UpdatedAt = now
	}
	if member.MemberRole == "" {
		member.MemberRole = ChatConversationMemberRoleMember
	}
	return nil
}

func (member *ChatConversationMember) BeforeUpdate(tx *gorm.DB) error {
	member.UpdatedAt = common.GetTimestamp()
	return nil
}

func upsertConversationMember(tx *gorm.DB, conversationID int, userID int, role string) error {
	if conversationID <= 0 || userID <= 0 {
		return errors.New("conversation and user ids are required")
	}
	if role == "" {
		role = ChatConversationMemberRoleMember
	}

	member := ChatConversationMember{}
	err := tx.Where("conversation_id = ? AND user_id = ?", conversationID, userID).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		member = ChatConversationMember{
			ConversationId: conversationID,
			UserId:         userID,
			MemberRole:     role,
			Active:         true,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "conversation_id"}, {Name: "user_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"member_role",
				"active",
				"left_at",
				"updated_at",
			}),
		}).Create(&member).Error
	}
	if err != nil {
		return err
	}
	member.MemberRole = role
	member.Active = true
	member.LeftAt = 0
	return tx.Save(&member).Error
}
