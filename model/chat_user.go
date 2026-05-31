package model

import "github.com/QuantumNous/new-api/common"

type ChatUser struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
}

func ListChatUsers(currentUserID int) ([]*ChatUser, error) {
	users := make([]*ChatUser, 0)
	query := DB.Model(&User{}).
		Select("id, username, display_name").
		Where("status = ?", common.UserStatusEnabled)
	if currentUserID > 0 {
		query = query.Where("id <> ?", currentUserID)
	}
	if err := query.Order("username ASC, id ASC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}
