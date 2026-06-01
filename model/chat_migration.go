package model

import (
	"errors"
	"sync"

	"gorm.io/gorm"
)

var (
	chatTablesMu sync.Mutex
	chatTablesDB *gorm.DB
)

// EnsureChatTables makes the embedded user-chat feature resilient when a
// running deployment reaches chat routes before the new chat tables have been
// created by startup migration.
func EnsureChatTables() error {
	chatTablesMu.Lock()
	defer chatTablesMu.Unlock()

	if DB == nil {
		return errors.New("database is not initialized")
	}
	if chatTablesDB == DB {
		return nil
	}
	if err := DB.AutoMigrate(
		&ChatConversation{},
		&ChatConversationMember{},
		&ChatMessage{},
		&ChatMessageReaction{},
		&ChatReadState{},
	); err != nil {
		return err
	}
	chatTablesDB = DB
	return nil
}
