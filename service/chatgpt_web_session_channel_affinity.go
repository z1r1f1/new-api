package service

import (
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
)

const (
	ginKeyChatGPTWebSessionChannelAffinityCacheKey   = "chatgpt_web_session_channel_affinity_cache_key"
	ginKeyChatGPTWebSessionChannelAffinityKeySuffix  = "chatgpt_web_session_channel_affinity_key_suffix"
	ginKeyChatGPTWebSessionChannelAffinityTTLSeconds = "chatgpt_web_session_channel_affinity_ttl_seconds"
	ginKeyChatGPTWebSessionChannelAffinityMeta       = "chatgpt_web_session_channel_affinity_meta"

	chatGPTWebSessionChannelAffinityCacheNamespace = "new-api:chatgpt_web_session_channel_affinity:v1"
	chatGPTWebSessionChannelAffinityTTL            = 6 * time.Hour
)

var (
	chatGPTWebSessionChannelAffinityCacheOnce sync.Once
	chatGPTWebSessionChannelAffinityCache     *cachex.HybridCache[int]
)

type chatGPTWebSessionChannelAffinityMeta struct {
	CacheKey       string
	CacheKeySuffix string
	TTLSeconds     int
	UsingGroup     string
	ModelName      string
	RequestPath    string
	KeyFingerprint string
	KeyHint        string
}

func getChatGPTWebSessionChannelAffinityCache() *cachex.HybridCache[int] {
	chatGPTWebSessionChannelAffinityCacheOnce.Do(func() {
		capacity := 100_000
		if setting := operation_setting.GetChannelAffinitySetting(); setting != nil && setting.MaxEntries > 0 {
			capacity = setting.MaxEntries
		}

		chatGPTWebSessionChannelAffinityCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace:  cachex.Namespace(chatGPTWebSessionChannelAffinityCacheNamespace),
			Redis:      common.RDB,
			RedisCodec: cachex.IntCodec{},
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, capacity).
					WithTTL(chatGPTWebSessionChannelAffinityTTL).
					WithJanitor().
					Build()
			},
		})
	})
	return chatGPTWebSessionChannelAffinityCache
}

func GetPreferredChatGPTWebSessionChannelByAffinity(c *gin.Context, modelName string, usingGroup string) (int, bool) {
	meta, ok := prepareChatGPTWebSessionChannelAffinity(c, modelName, usingGroup)
	if !ok {
		return 0, false
	}

	channelID, found, err := getChatGPTWebSessionChannelAffinityCache().Get(meta.CacheKeySuffix)
	if err != nil {
		common.SysError(fmt.Sprintf("chatgpt web session channel affinity cache get failed: key=%s, err=%v", meta.CacheKey, err))
		return 0, false
	}
	if !found || channelID <= 0 {
		return 0, false
	}
	return channelID, true
}

func RecordChatGPTWebSessionChannelAffinity(c *gin.Context, channel *model.Channel) {
	if c == nil || channel == nil || channel.Id <= 0 || channel.Type != constant.ChannelTypeChatGPTImage {
		return
	}
	if channel.Status != common.ChannelStatusEnabled {
		return
	}

	meta, ok := getChatGPTWebSessionChannelAffinityMeta(c)
	if !ok {
		meta, ok = prepareChatGPTWebSessionChannelAffinity(
			c,
			strings.TrimSpace(c.GetString("original_model")),
			common.GetContextKeyString(c, constant.ContextKeyUsingGroup),
		)
		if !ok {
			return
		}
	}

	ttl := time.Duration(meta.TTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = chatGPTWebSessionChannelAffinityTTL
	}
	if err := getChatGPTWebSessionChannelAffinityCache().SetWithTTL(meta.CacheKeySuffix, channel.Id, ttl); err != nil {
		common.SysError(fmt.Sprintf("chatgpt web session channel affinity cache set failed: key=%s, err=%v", meta.CacheKey, err))
	}
}

func ClearCurrentChatGPTWebSessionChannelAffinity(c *gin.Context) {
	meta, ok := getChatGPTWebSessionChannelAffinityMeta(c)
	if !ok || strings.TrimSpace(meta.CacheKeySuffix) == "" {
		return
	}
	if _, err := getChatGPTWebSessionChannelAffinityCache().DeleteMany([]string{meta.CacheKeySuffix}); err != nil {
		common.SysError(fmt.Sprintf("chatgpt web session channel affinity cache delete failed: key=%s, err=%v", meta.CacheKey, err))
	}
}

func prepareChatGPTWebSessionChannelAffinity(c *gin.Context, modelName string, usingGroup string) (chatGPTWebSessionChannelAffinityMeta, bool) {
	sessionKey := extractChatGPTWebSessionChannelAffinityKey(c)
	if sessionKey == "" {
		return chatGPTWebSessionChannelAffinityMeta{}, false
	}
	usingGroup = strings.TrimSpace(usingGroup)
	if usingGroup == "" {
		usingGroup = common.GetContextKeyString(c, constant.ContextKeyUsingGroup)
	}
	if usingGroup == "" {
		usingGroup = common.GetContextKeyString(c, constant.ContextKeyTokenGroup)
	}
	if usingGroup == "" {
		usingGroup = "default"
	}

	suffix := buildChatGPTWebSessionChannelAffinityCacheKeySuffix(usingGroup, sessionKey)
	if suffix == "" {
		return chatGPTWebSessionChannelAffinityMeta{}, false
	}
	meta := chatGPTWebSessionChannelAffinityMeta{
		CacheKey:       getChatGPTWebSessionChannelAffinityCache().FullKey(suffix),
		CacheKeySuffix: suffix,
		TTLSeconds:     int(chatGPTWebSessionChannelAffinityTTL / time.Second),
		UsingGroup:     usingGroup,
		ModelName:      strings.TrimSpace(modelName),
		RequestPath:    chatGPTWebSessionChannelAffinityRequestPath(c),
		KeyFingerprint: affinityFingerprint(sessionKey),
		KeyHint:        buildChannelAffinityKeyHint(sessionKey),
	}
	setChatGPTWebSessionChannelAffinityContext(c, meta)
	return meta, true
}

func extractChatGPTWebSessionChannelAffinityKey(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	body := []byte(nil)
	if c.Request.Body != nil {
		if storage, err := common.GetBodyStorage(c); err == nil {
			if b, err := storage.Bytes(); err == nil {
				body = b
			}
		}
	}
	return ExtractOpenAICompatPromptCacheKeyFromRawBody(body, chatGPTWebSessionChannelAffinityHeaders(c.Request.Header))
}

func chatGPTWebSessionChannelAffinityHeaders(header http.Header) map[string]string {
	if len(header) == 0 {
		return nil
	}
	out := make(map[string]string, len(header))
	for key := range header {
		value := strings.TrimSpace(header.Get(key))
		if value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func buildChatGPTWebSessionChannelAffinityCacheKeySuffix(usingGroup string, sessionKey string) string {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" {
		return ""
	}
	usingGroup = strings.TrimSpace(usingGroup)
	if usingGroup == "" {
		usingGroup = "default"
	}
	return strings.Join([]string{
		"group",
		fmt.Sprintf("%x", common.Sha256Raw([]byte(usingGroup))),
		"session",
		fmt.Sprintf("%x", common.Sha256Raw([]byte(sessionKey))),
	}, ":")
}

func setChatGPTWebSessionChannelAffinityContext(c *gin.Context, meta chatGPTWebSessionChannelAffinityMeta) {
	if c == nil || strings.TrimSpace(meta.CacheKeySuffix) == "" {
		return
	}
	c.Set(ginKeyChatGPTWebSessionChannelAffinityCacheKey, meta.CacheKey)
	c.Set(ginKeyChatGPTWebSessionChannelAffinityKeySuffix, meta.CacheKeySuffix)
	c.Set(ginKeyChatGPTWebSessionChannelAffinityTTLSeconds, meta.TTLSeconds)
	c.Set(ginKeyChatGPTWebSessionChannelAffinityMeta, meta)
}

func getChatGPTWebSessionChannelAffinityMeta(c *gin.Context) (chatGPTWebSessionChannelAffinityMeta, bool) {
	if c == nil {
		return chatGPTWebSessionChannelAffinityMeta{}, false
	}
	rawMeta, ok := c.Get(ginKeyChatGPTWebSessionChannelAffinityMeta)
	if !ok {
		return chatGPTWebSessionChannelAffinityMeta{}, false
	}
	meta, ok := rawMeta.(chatGPTWebSessionChannelAffinityMeta)
	if !ok || strings.TrimSpace(meta.CacheKeySuffix) == "" {
		return chatGPTWebSessionChannelAffinityMeta{}, false
	}
	return meta, true
}

func chatGPTWebSessionChannelAffinityRequestPath(c *gin.Context) string {
	if c == nil || c.Request == nil || c.Request.URL == nil {
		return ""
	}
	return c.Request.URL.Path
}
