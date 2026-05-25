package chatgptimg

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	chatGPTWebClientCacheTTL = 10 * time.Minute
	chatGPTWebClientCacheMax = 128
)

type chatGPTWebClientCacheEntry struct {
	client    *Client
	expiresAt time.Time
	lastUsed  time.Time
}

var chatGPTWebClientCache = struct {
	sync.Mutex
	entries map[string]*chatGPTWebClientCacheEntry
}{
	entries: make(map[string]*chatGPTWebClientCacheEntry),
}

func getCachedClient(opt ClientOptions) (*Client, bool, error) {
	key := chatGPTWebClientCacheKey(opt)
	now := time.Now()

	chatGPTWebClientCache.Lock()
	if entry := chatGPTWebClientCache.entries[key]; entry != nil && now.Before(entry.expiresAt) && entry.client != nil {
		entry.lastUsed = now
		client := entry.client
		chatGPTWebClientCache.Unlock()
		return client, true, nil
	}
	chatGPTWebClientCache.Unlock()

	client, err := NewClient(opt)
	if err != nil {
		return nil, false, err
	}

	chatGPTWebClientCache.Lock()
	defer chatGPTWebClientCache.Unlock()
	pruneExpiredChatGPTWebClientsLocked(now)
	if len(chatGPTWebClientCache.entries) >= chatGPTWebClientCacheMax {
		evictOldestChatGPTWebClientLocked()
	}
	chatGPTWebClientCache.entries[key] = &chatGPTWebClientCacheEntry{
		client:    client,
		expiresAt: now.Add(chatGPTWebClientCacheTTL),
		lastUsed:  now,
	}
	return client, false, nil
}

func chatGPTWebClientCacheKey(opt ClientOptions) string {
	parts := []string{
		strings.TrimSpace(opt.BaseURL),
		strings.TrimSpace(opt.ProxyURL),
		hashCachePart(opt.AuthToken),
		strings.TrimSpace(opt.DeviceID),
		strings.TrimSpace(opt.SessionID),
		strings.TrimSpace(opt.UserAgent),
		strings.TrimSpace(opt.ClientVersion),
		strings.TrimSpace(opt.Language),
		strconv.FormatInt(int64(opt.Timeout), 10),
		strconv.FormatInt(int64(opt.SSETimeout), 10),
	}
	return strings.Join(parts, "\x00")
}

func hashCachePart(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func pruneExpiredChatGPTWebClientsLocked(now time.Time) {
	for key, entry := range chatGPTWebClientCache.entries {
		if entry == nil || !now.Before(entry.expiresAt) {
			delete(chatGPTWebClientCache.entries, key)
		}
	}
}

func evictOldestChatGPTWebClientLocked() {
	var oldestKey string
	var oldestTime time.Time
	for key, entry := range chatGPTWebClientCache.entries {
		if entry == nil {
			oldestKey = key
			break
		}
		if oldestKey == "" || entry.lastUsed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastUsed
		}
	}
	if oldestKey != "" {
		delete(chatGPTWebClientCache.entries, oldestKey)
	}
}

func resetChatGPTWebClientCacheForTest() {
	chatGPTWebClientCache.Lock()
	defer chatGPTWebClientCache.Unlock()
	chatGPTWebClientCache.entries = make(map[string]*chatGPTWebClientCacheEntry)
}
