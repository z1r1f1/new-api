package model

import "github.com/QuantumNous/new-api/common"

type ChatUser struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
}

func ListChatUsers(currentUserID int) ([]*ChatUser, error) {
	currentUser, err := GetUserById(currentUserID, false)
	if err != nil {
		return nil, err
	}

	users := make([]*ChatUser, 0)
	query := DB.Model(&User{}).
		Select("id, username, display_name, role").
		Where("status = ?", common.UserStatusEnabled).
		Where("id <> ?", currentUserID)
	if currentUser.Role < common.RoleAdminUser {
		query = query.Where("role >= ?", common.RoleAdminUser)
	}
	if err := query.Order("role DESC, username ASC, id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func ListChatUsersByIDs(userIDs []int) ([]*ChatUser, error) {
	ids := normalizeConversationMemberIDs(userIDs...)
	if len(ids) == 0 {
		return []*ChatUser{}, nil
	}
	users := make([]*ChatUser, 0, len(ids))
	if err := DB.Model(&User{}).
		Select("id, username, display_name, role").
		Where("id IN ?", ids).
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func ListEnabledChatUsers() ([]*ChatUser, error) {
	users := make([]*ChatUser, 0)
	if err := DB.Model(&User{}).
		Select("id, username, display_name, role").
		Where("status = ?", common.UserStatusEnabled).
		Order("role DESC, id ASC").
		Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}
