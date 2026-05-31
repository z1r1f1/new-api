package model

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ChatConversationType string

const (
	ChatConversationTypeDirect ChatConversationType = "direct"
	ChatConversationTypeGroup  ChatConversationType = "group"
)

const (
	ChatDefaultGroupDirectKey = "__default_group__"
	ChatDefaultGroupTitle     = "Default group"
)

const (
	ChatConversationMemberRoleOwner  = "owner"
	ChatConversationMemberRoleMember = "member"
	ChatMessageTypeText              = "text"
)

type ChatConversation struct {
	Id            int                  `json:"id"`
	Type          ChatConversationType `json:"type" gorm:"type:varchar(16);not null;index"`
	Title         string               `json:"title" gorm:"type:varchar(128);default:''"`
	OwnerId       int                  `json:"owner_id" gorm:"index"`
	DirectKey     string               `json:"direct_key" gorm:"type:varchar(32);uniqueIndex"`
	LastMessageId int                  `json:"last_message_id" gorm:"index"`
	LastMessageAt int64                `json:"last_message_at" gorm:"bigint;index"`
	CreatedAt     int64                `json:"created_at" gorm:"bigint;not null"`
	UpdatedAt     int64                `json:"updated_at" gorm:"bigint;not null"`
}

func (ChatConversation) TableName() string {
	return "chat_conversations"
}

func (conversation *ChatConversation) BeforeCreate(tx *gorm.DB) error {
	now := common.GetTimestamp()
	if conversation.CreatedAt == 0 {
		conversation.CreatedAt = now
	}
	if conversation.UpdatedAt == 0 {
		conversation.UpdatedAt = now
	}
	return nil
}

func (conversation *ChatConversation) BeforeUpdate(tx *gorm.DB) error {
	conversation.UpdatedAt = common.GetTimestamp()
	return nil
}

func normalizeConversationMemberIDs(memberIDs ...int) []int {
	seen := make(map[int]struct{}, len(memberIDs))
	result := make([]int, 0, len(memberIDs))
	for _, id := range memberIDs {
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

func normalizeDirectConversationKey(userAID, userBID int) (string, error) {
	if userAID <= 0 || userBID <= 0 {
		return "", errors.New("conversation members must be positive user ids")
	}
	if userAID == userBID {
		return "", errors.New("direct conversation requires two different users")
	}
	ids := []int{userAID, userBID}
	sort.Ints(ids)
	return fmt.Sprintf("%d:%d", ids[0], ids[1]), nil
}

func GetConversationByID(id int) (*ChatConversation, error) {
	if id <= 0 {
		return nil, gorm.ErrRecordNotFound
	}
	var conversation ChatConversation
	if err := DB.First(&conversation, id).Error; err != nil {
		return nil, err
	}
	return &conversation, nil
}

func ListUserConversations(userID int) ([]*ChatConversation, error) {
	if userID <= 0 {
		return []*ChatConversation{}, nil
	}
	var conversations []*ChatConversation
	err := DB.
		Distinct("chat_conversations.*").
		Joins("JOIN chat_conversation_members ON chat_conversation_members.conversation_id = chat_conversations.id").
		Where("chat_conversation_members.user_id = ? AND chat_conversation_members.active = ?", userID, true).
		Order("chat_conversations.last_message_at DESC, chat_conversations.id DESC").
		Find(&conversations).Error
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

func GetOrCreateDirectConversation(userAID, userBID int) (*ChatConversation, error) {
	directKey, err := normalizeDirectConversationKey(userAID, userBID)
	if err != nil {
		return nil, err
	}

	conversation := &ChatConversation{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("direct_key = ?", directKey).First(conversation).Error; err == nil {
			return ensureConversationMembers(tx, conversation.Id, map[int]string{
				userAID: ChatConversationMemberRoleMember,
				userBID: ChatConversationMemberRoleMember,
			})
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		ids := []int{userAID, userBID}
		sort.Ints(ids)
		conversation.Type = ChatConversationTypeDirect
		conversation.Title = ""
		conversation.OwnerId = ids[0]
		conversation.DirectKey = directKey
		conversation.LastMessageId = 0
		conversation.LastMessageAt = 0
		conversation.CreatedAt = common.GetTimestamp()
		conversation.UpdatedAt = conversation.CreatedAt
		if err := tx.Create(conversation).Error; err != nil {
			return err
		}
		return ensureConversationMembers(tx, conversation.Id, map[int]string{
			userAID: ChatConversationMemberRoleMember,
			userBID: ChatConversationMemberRoleMember,
		})
	})
	if err != nil {
		return nil, err
	}
	return conversation, nil
}

func GetOrCreateDefaultGroupConversation() (*ChatConversation, error) {
	users, err := ListEnabledChatUsers()
	if err != nil {
		return nil, err
	}
	if len(users) == 0 {
		return nil, nil
	}

	ownerID := users[0].Id
	ownerSelectedFromAdmin := users[0].Role >= common.RoleAdminUser
	memberIDs := make([]int, 0, len(users))
	for _, user := range users {
		memberIDs = append(memberIDs, user.Id)
		if user.Role >= common.RoleAdminUser && !ownerSelectedFromAdmin {
			ownerID = user.Id
			ownerSelectedFromAdmin = true
		}
	}

	conversation := &ChatConversation{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("direct_key = ?", ChatDefaultGroupDirectKey).First(conversation).Error; err == nil {
			if conversation.OwnerId <= 0 {
				conversation.OwnerId = ownerID
			}
			if conversation.Title == "" {
				conversation.Title = ChatDefaultGroupTitle
			}
			conversation.Type = ChatConversationTypeGroup
			if err := tx.Save(conversation).Error; err != nil {
				return err
			}
		} else if errors.Is(err, gorm.ErrRecordNotFound) {
			conversation.Type = ChatConversationTypeGroup
			conversation.Title = ChatDefaultGroupTitle
			conversation.OwnerId = ownerID
			conversation.DirectKey = ChatDefaultGroupDirectKey
			conversation.LastMessageId = 0
			conversation.LastMessageAt = 0
			conversation.CreatedAt = common.GetTimestamp()
			conversation.UpdatedAt = conversation.CreatedAt
			if err := tx.Create(conversation).Error; err != nil {
				return err
			}
		} else {
			return err
		}

		roleByUserID := make(map[int]string, len(memberIDs))
		for _, userID := range memberIDs {
			roleByUserID[userID] = ChatConversationMemberRoleMember
		}
		roleByUserID[ownerID] = ChatConversationMemberRoleOwner
		return ensureConversationMembers(tx, conversation.Id, roleByUserID)
	})
	if err != nil {
		return nil, err
	}
	return conversation, nil
}

func CreateGroupConversation(ownerID int, title string, memberIDs []int) (*ChatConversation, error) {
	if ownerID <= 0 {
		return nil, errors.New("group conversation requires a valid owner")
	}

	members := normalizeConversationMemberIDs(append(memberIDs, ownerID)...)
	if len(members) == 0 {
		members = []int{ownerID}
	}

	conversation := &ChatConversation{}
	err := DB.Transaction(func(tx *gorm.DB) error {
		conversation.Type = ChatConversationTypeGroup
		conversation.Title = strings.TrimSpace(title)
		conversation.OwnerId = ownerID
		conversation.DirectKey = ""
		conversation.LastMessageId = 0
		conversation.LastMessageAt = 0
		conversation.CreatedAt = common.GetTimestamp()
		conversation.UpdatedAt = conversation.CreatedAt
		if err := tx.Create(conversation).Error; err != nil {
			return err
		}

		roleByUserID := make(map[int]string, len(members))
		for _, userID := range members {
			roleByUserID[userID] = ChatConversationMemberRoleMember
		}
		roleByUserID[ownerID] = ChatConversationMemberRoleOwner
		return ensureConversationMembers(tx, conversation.Id, roleByUserID)
	})
	if err != nil {
		return nil, err
	}
	return conversation, nil
}

func updateConversationLastMessage(tx *gorm.DB, conversationID int, messageID int, messageAt int64) error {
	if conversationID <= 0 || messageID <= 0 {
		return nil
	}
	return tx.Model(&ChatConversation{}).
		Where("id = ?", conversationID).
		Updates(map[string]any{
			"last_message_id": messageID,
			"last_message_at": messageAt,
			"updated_at":      common.GetTimestamp(),
		}).Error
}

func ensureConversationMembers(tx *gorm.DB, conversationID int, roleByUserID map[int]string) error {
	if conversationID <= 0 || len(roleByUserID) == 0 {
		return nil
	}
	for userID, role := range roleByUserID {
		if err := upsertConversationMember(tx, conversationID, userID, role); err != nil {
			return err
		}
	}
	return nil
}

func setConversationMemberActive(tx *gorm.DB, conversationID int, userID int, active bool) error {
	member := ChatConversationMember{}
	err := tx.Where("conversation_id = ? AND user_id = ?", conversationID, userID).First(&member).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !active {
			return nil
		}
		return upsertConversationMember(tx, conversationID, userID, ChatConversationMemberRoleMember)
	}
	if err != nil {
		return err
	}
	member.Active = active
	if active {
		member.LeftAt = 0
	} else {
		member.LeftAt = common.GetTimestamp()
	}
	member.UpdatedAt = common.GetTimestamp()
	return tx.Save(&member).Error
}

func AddConversationMember(conversationID int, userID int, role string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return upsertConversationMember(tx, conversationID, userID, role)
	})
}

func RemoveConversationMember(conversationID int, userID int) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return setConversationMemberActive(tx, conversationID, userID, false)
	})
}

func ListConversationMembers(conversationID int) ([]*ChatConversationMember, error) {
	if conversationID <= 0 {
		return []*ChatConversationMember{}, nil
	}
	var members []*ChatConversationMember
	if err := DB.Where("conversation_id = ? AND active = ?", conversationID, true).
		Order("user_id ASC").
		Find(&members).Error; err != nil {
		return nil, err
	}
	return members, nil
}

func IsConversationMember(conversationID int, userID int) (bool, error) {
	if conversationID <= 0 || userID <= 0 {
		return false, nil
	}
	var count int64
	if err := DB.Model(&ChatConversationMember{}).
		Where("conversation_id = ? AND user_id = ? AND active = ?", conversationID, userID, true).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func touchConversationAfterMessage(conversationID int, messageID int, messageAt int64) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		return updateConversationLastMessage(tx, conversationID, messageID, messageAt)
	})
}

func applyDirectConversationPatch(conversationID int, userID int) error {
	if conversationID <= 0 || userID <= 0 {
		return nil
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return setConversationMemberActive(tx, conversationID, userID, true)
	})
}

func refreshConversationMembers(conversationID int, userIDs []int) error {
	if conversationID <= 0 {
		return nil
	}
	members := normalizeConversationMemberIDs(userIDs...)
	roleByUserID := make(map[int]string, len(members))
	for _, userID := range members {
		roleByUserID[userID] = ChatConversationMemberRoleMember
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		return ensureConversationMembers(tx, conversationID, roleByUserID)
	})
}

func conversationMemberIDs(conversationID int) ([]int, error) {
	members, err := ListConversationMembers(conversationID)
	if err != nil {
		return nil, err
	}
	result := make([]int, 0, len(members))
	for _, member := range members {
		result = append(result, member.UserId)
	}
	return result, nil
}

func ConversationMemberIDsForChat(conversationID int) ([]int, error) {
	return conversationMemberIDs(conversationID)
}

func deleteChatConversationMembers(conversationID int) error {
	if conversationID <= 0 {
		return nil
	}
	return DB.Where("conversation_id = ?", conversationID).Delete(&ChatConversationMember{}).Error
}

func deleteChatConversationMessages(conversationID int) error {
	if conversationID <= 0 {
		return nil
	}
	return DB.Where("conversation_id = ?", conversationID).Delete(&ChatMessage{}).Error
}

func deleteChatReadStates(conversationID int) error {
	if conversationID <= 0 {
		return nil
	}
	return DB.Where("conversation_id = ?", conversationID).Delete(&ChatReadState{}).Error
}

func ensureChatConversationExists(conversationID int) error {
	if conversationID <= 0 {
		return gorm.ErrRecordNotFound
	}
	var conversation ChatConversation
	return DB.First(&conversation, conversationID).Error
}

func upsertChatConversation(conversation *ChatConversation) error {
	if conversation == nil {
		return errors.New("conversation is nil")
	}
	if conversation.Type == "" {
		conversation.Type = ChatConversationTypeGroup
	}
	conversation.Title = strings.TrimSpace(conversation.Title)
	if conversation.Type == ChatConversationTypeDirect && conversation.DirectKey == "" {
		return errors.New("direct conversation requires direct key")
	}
	if conversation.OwnerId <= 0 {
		return errors.New("conversation owner is required")
	}
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(conversation).Error
}
