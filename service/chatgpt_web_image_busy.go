package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

const (
	chatGPTWebImageBusyKeyPrefix     = "chatgpt_web:image_busy"
	chatGPTWebImageBusyTTLSecondsEnv = "CHATGPT_WEB_IMAGE_BUSY_TTL_SECONDS"
	chatGPTWebImageBusyDefaultTTL    = 25 * time.Minute

	ginKeyChatGPTWebImageBusySelected      = "chatgpt_web_image_busy_selected"
	ginKeyChatGPTWebImageBusySelectedBusy  = "chatgpt_web_image_busy_selected_busy"
	ginKeyChatGPTWebImageBusyTTLSeconds    = "chatgpt_web_image_busy_ttl_seconds"
	ginKeyChatGPTWebImageBusyAcquireResult = "chatgpt_web_image_busy_acquired"
	ginKeyChatGPTWebImageBusyHeldChannels  = "chatgpt_web_image_busy_held_channels"
)

type chatGPTWebImageBusyEntry struct {
	owner     string
	expiresAt time.Time
}

var (
	chatGPTWebImageBusyMu     sync.Mutex
	chatGPTWebImageBusyMemory = make(map[int]chatGPTWebImageBusyEntry)
)

func ShouldUseChatGPTWebImageBusyAvoidance(relayMode int, modelName string) bool {
	switch relayMode {
	case relayconstant.RelayModeImagesGenerations, relayconstant.RelayModeImagesEdits:
	default:
		return false
	}
	modelName = strings.ToLower(strings.TrimSpace(modelName))
	return modelName == "gpt-image-2" || modelName == "chatgpt-image-2" || strings.Contains(modelName, "gpt-image-2")
}

func ChatGPTWebImageBusyTTL() time.Duration {
	seconds := common.GetEnvOrDefault(chatGPTWebImageBusyTTLSecondsEnv, int(chatGPTWebImageBusyDefaultTTL/time.Second))
	if seconds <= 0 {
		return chatGPTWebImageBusyDefaultTTL
	}
	return time.Duration(seconds) * time.Second
}

func IsChatGPTWebImageBusy(channelID int) bool {
	if channelID <= 0 {
		return false
	}
	if common.RedisEnabled && common.RDB != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		exists, err := common.RDB.Exists(ctx, chatGPTWebImageBusyKey(channelID)).Result()
		if err == nil {
			return exists > 0
		}
		logger.LogWarn(nil, fmt.Sprintf("failed to check ChatGPT Web image busy marker for channel #%d: %v", channelID, err))
	}
	return isChatGPTWebImageBusyMemory(channelID)
}

func PreferIdleChatGPTWebImageChannel(channel *model.Channel) bool {
	if channel == nil {
		return false
	}
	if channel.Type != constant.ChannelTypeChatGPTImage {
		return true
	}
	return !IsChatGPTWebImageBusy(channel.Id)
}

func AcquireChatGPTWebImageBusy(c *gin.Context, channel *model.Channel, relayMode int, modelName string) func() {
	if channel == nil || channel.Type != constant.ChannelTypeChatGPTImage || !ShouldUseChatGPTWebImageBusyAvoidance(relayMode, modelName) {
		return nil
	}
	if isChatGPTWebImageBusyHeldByRequest(c, channel.Id) {
		return nil
	}
	release, acquired := acquireChatGPTWebImageBusyWithTTL(channel.Id, ChatGPTWebImageBusyTTL())
	if c != nil {
		c.Set(ginKeyChatGPTWebImageBusySelected, channel.Id)
		c.Set(ginKeyChatGPTWebImageBusySelectedBusy, !acquired)
		c.Set(ginKeyChatGPTWebImageBusyTTLSeconds, int(ChatGPTWebImageBusyTTL()/time.Second))
		c.Set(ginKeyChatGPTWebImageBusyAcquireResult, acquired)
	}
	if !acquired {
		logger.LogDebug(c, "ChatGPT Web image channel #%d is already busy; proceeding because selection fallback allowed it", channel.Id)
		return nil
	}
	markChatGPTWebImageBusyHeldByRequest(c, channel.Id)
	logger.LogDebug(c, "marked ChatGPT Web image channel #%d busy", channel.Id)
	return func() {
		release()
		unmarkChatGPTWebImageBusyHeldByRequest(c, channel.Id)
	}
}

func AppendChatGPTWebImageBusyAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	selected := c.GetInt(ginKeyChatGPTWebImageBusySelected)
	if selected <= 0 {
		return
	}
	adminInfo["chatgpt_web_image_busy"] = map[string]interface{}{
		"selected_channel_id": selected,
		"selected_was_busy":   c.GetBool(ginKeyChatGPTWebImageBusySelectedBusy),
		"acquired":            c.GetBool(ginKeyChatGPTWebImageBusyAcquireResult),
		"ttl_seconds":         c.GetInt(ginKeyChatGPTWebImageBusyTTLSeconds),
	}
}

func acquireChatGPTWebImageBusyWithTTL(channelID int, ttl time.Duration) (func(), bool) {
	if channelID <= 0 {
		return nil, false
	}
	if ttl <= 0 {
		ttl = chatGPTWebImageBusyDefaultTTL
	}
	owner := fmt.Sprintf("%d:%s", time.Now().UnixNano(), common.GetRandomString(12))
	if common.RedisEnabled && common.RDB != nil {
		return acquireChatGPTWebImageBusyRedis(channelID, owner, ttl)
	}
	return acquireChatGPTWebImageBusyMemory(channelID, owner, ttl)
}

func acquireChatGPTWebImageBusyRedis(channelID int, owner string, ttl time.Duration) (func(), bool) {
	key := chatGPTWebImageBusyKey(channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ok, err := common.RDB.SetNX(ctx, key, owner, ttl).Result()
	if err != nil {
		logger.LogWarn(nil, fmt.Sprintf("failed to acquire ChatGPT Web image busy marker for channel #%d: %v", channelID, err))
		return acquireChatGPTWebImageBusyMemory(channelID, owner, ttl)
	}
	if !ok {
		return nil, false
	}
	return func() {
		releaseChatGPTWebImageBusyRedis(channelID, owner)
	}, true
}

func releaseChatGPTWebImageBusyRedis(channelID int, owner string) {
	key := chatGPTWebImageBusyKey(channelID)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	const compareAndDelete = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
	if err := common.RDB.Eval(ctx, compareAndDelete, []string{key}, owner).Err(); err != nil && err != redis.Nil {
		logger.LogWarn(nil, fmt.Sprintf("failed to release ChatGPT Web image busy marker for channel #%d: %v", channelID, err))
	}
}

func acquireChatGPTWebImageBusyMemory(channelID int, owner string, ttl time.Duration) (func(), bool) {
	chatGPTWebImageBusyMu.Lock()
	defer chatGPTWebImageBusyMu.Unlock()

	now := time.Now()
	if entry, ok := chatGPTWebImageBusyMemory[channelID]; ok && entry.expiresAt.After(now) {
		return nil, false
	}
	chatGPTWebImageBusyMemory[channelID] = chatGPTWebImageBusyEntry{
		owner:     owner,
		expiresAt: now.Add(ttl),
	}
	return func() {
		releaseChatGPTWebImageBusyMemory(channelID, owner)
	}, true
}

func releaseChatGPTWebImageBusyMemory(channelID int, owner string) {
	chatGPTWebImageBusyMu.Lock()
	defer chatGPTWebImageBusyMu.Unlock()
	entry, ok := chatGPTWebImageBusyMemory[channelID]
	if ok && entry.owner == owner {
		delete(chatGPTWebImageBusyMemory, channelID)
	}
}

func isChatGPTWebImageBusyMemory(channelID int) bool {
	chatGPTWebImageBusyMu.Lock()
	defer chatGPTWebImageBusyMu.Unlock()
	entry, ok := chatGPTWebImageBusyMemory[channelID]
	if !ok {
		return false
	}
	if !entry.expiresAt.After(time.Now()) {
		delete(chatGPTWebImageBusyMemory, channelID)
		return false
	}
	return true
}

func chatGPTWebImageBusyKey(channelID int) string {
	return fmt.Sprintf("%s:%d", chatGPTWebImageBusyKeyPrefix, channelID)
}

func isChatGPTWebImageBusyHeldByRequest(c *gin.Context, channelID int) bool {
	if c == nil || channelID <= 0 {
		return false
	}
	value, ok := c.Get(ginKeyChatGPTWebImageBusyHeldChannels)
	if !ok {
		return false
	}
	held, ok := value.(map[int]struct{})
	if !ok {
		return false
	}
	_, exists := held[channelID]
	return exists
}

func markChatGPTWebImageBusyHeldByRequest(c *gin.Context, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	held := map[int]struct{}{}
	if value, ok := c.Get(ginKeyChatGPTWebImageBusyHeldChannels); ok {
		if existing, ok := value.(map[int]struct{}); ok {
			held = existing
		}
	}
	held[channelID] = struct{}{}
	c.Set(ginKeyChatGPTWebImageBusyHeldChannels, held)
}

func unmarkChatGPTWebImageBusyHeldByRequest(c *gin.Context, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	value, ok := c.Get(ginKeyChatGPTWebImageBusyHeldChannels)
	if !ok {
		return
	}
	held, ok := value.(map[int]struct{})
	if !ok {
		return
	}
	delete(held, channelID)
	if len(held) == 0 {
		c.Set(ginKeyChatGPTWebImageBusyHeldChannels, nil)
		return
	}
	c.Set(ginKeyChatGPTWebImageBusyHeldChannels, held)
}

func resetChatGPTWebImageBusyForTest() {
	chatGPTWebImageBusyMu.Lock()
	defer chatGPTWebImageBusyMu.Unlock()
	chatGPTWebImageBusyMemory = make(map[int]chatGPTWebImageBusyEntry)
}
