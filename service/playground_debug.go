package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

const (
	PlaygroundDebugIDHeader          = "X-Playground-Debug-Id"
	maxPlaygroundDebugIDLength       = 128
	maxPlaygroundUpstreamDebugBytes  = 128 * 1024
	playgroundUpstreamDebugRetention = 10 * time.Minute
)

type PlaygroundUpstreamRequestDebug struct {
	UpstreamRequest any    `json:"upstream_request"`
	BodyBytes       int    `json:"body_bytes"`
	BodyTruncated   bool   `json:"body_truncated"`
	CapturedAt      string `json:"captured_at"`
}

type playgroundUpstreamDebugEntry struct {
	data      PlaygroundUpstreamRequestDebug
	expiresAt time.Time
}

var playgroundUpstreamDebugStore sync.Map // map[string]playgroundUpstreamDebugEntry

func NormalizePlaygroundDebugID(debugID string) string {
	debugID = strings.TrimSpace(debugID)
	if debugID == "" || len(debugID) > maxPlaygroundDebugIDLength {
		return ""
	}
	for _, r := range debugID {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			continue
		}
		return ""
	}
	return debugID
}

func ShouldCapturePlaygroundUpstreamRequestDebug(c *gin.Context) bool {
	if c == nil || c.GetInt("id") <= 0 {
		return false
	}
	return NormalizePlaygroundDebugID(common.GetContextKeyString(c, constant.ContextKeyPlaygroundDebugId)) != ""
}

func RecordPlaygroundUpstreamRequestDebug(c *gin.Context, body []byte) {
	if !ShouldCapturePlaygroundUpstreamRequestDebug(c) {
		return
	}
	debugID := NormalizePlaygroundDebugID(common.GetContextKeyString(c, constant.ContextKeyPlaygroundDebugId))
	if debugID == "" {
		return
	}
	bodyText, truncated := sanitizePlaygroundUpstreamDebugBody(body)
	var upstreamRequest any = bodyText
	if strings.TrimSpace(bodyText) != "" {
		var parsed any
		if err := common.Unmarshal([]byte(bodyText), &parsed); err == nil {
			upstreamRequest = parsed
		}
	}
	now := time.Now()
	data := PlaygroundUpstreamRequestDebug{
		UpstreamRequest: upstreamRequest,
		BodyBytes:       len(body),
		BodyTruncated:   truncated,
		CapturedAt:      now.Format(time.RFC3339Nano),
	}
	cleanupExpiredPlaygroundUpstreamDebug(now)
	playgroundUpstreamDebugStore.Store(playgroundUpstreamDebugKey(c.GetInt("id"), debugID), playgroundUpstreamDebugEntry{
		data:      data,
		expiresAt: now.Add(playgroundUpstreamDebugRetention),
	})
}

func GetPlaygroundUpstreamRequestDebug(userID int, debugID string) (PlaygroundUpstreamRequestDebug, bool) {
	debugID = NormalizePlaygroundDebugID(debugID)
	if userID <= 0 || debugID == "" {
		return PlaygroundUpstreamRequestDebug{}, false
	}
	key := playgroundUpstreamDebugKey(userID, debugID)
	value, ok := playgroundUpstreamDebugStore.Load(key)
	if !ok {
		return PlaygroundUpstreamRequestDebug{}, false
	}
	entry, ok := value.(playgroundUpstreamDebugEntry)
	if !ok || time.Now().After(entry.expiresAt) {
		playgroundUpstreamDebugStore.Delete(key)
		return PlaygroundUpstreamRequestDebug{}, false
	}
	return entry.data, true
}

func playgroundUpstreamDebugKey(userID int, debugID string) string {
	return fmt.Sprintf("%d:%s", userID, debugID)
}

func cleanupExpiredPlaygroundUpstreamDebug(now time.Time) {
	playgroundUpstreamDebugStore.Range(func(key, value any) bool {
		entry, ok := value.(playgroundUpstreamDebugEntry)
		if !ok || now.After(entry.expiresAt) {
			playgroundUpstreamDebugStore.Delete(key)
		}
		return true
	})
}

func sanitizePlaygroundUpstreamDebugBody(body []byte) (string, bool) {
	if len(body) == 0 {
		return "", false
	}
	var b strings.Builder
	b.Grow(min(len(body), maxPlaygroundUpstreamDebugBytes))
	truncated := false
	for i := 0; i < len(body); {
		if b.Len() >= maxPlaygroundUpstreamDebugBytes {
			truncated = true
			break
		}
		if hasPlaygroundDataURLPrefix(body, i) {
			next, replacement := redactPlaygroundDataURLForDebug(body, i)
			b.WriteString(replacement)
			i = next
			continue
		}
		b.WriteByte(body[i])
		i++
	}
	if truncated {
		b.WriteString(fmt.Sprintf("...[truncated upstream request body: %d bytes]", len(body)))
	}
	return b.String(), truncated
}

func hasPlaygroundDataURLPrefix(body []byte, index int) bool {
	if index+5 > len(body) {
		return false
	}
	return string(body[index:index+5]) == "data:"
}

func redactPlaygroundDataURLForDebug(body []byte, start int) (int, string) {
	end := start
	for end < len(body) && body[end] != '"' && body[end] != '\\' && body[end] != '\n' && body[end] != '\r' {
		end++
	}
	token := string(body[start:end])
	if !strings.Contains(token, ";base64,") {
		return start + 5, "data:"
	}
	mime := "image"
	if semi := strings.Index(token, ";"); semi > len("data:") {
		mime = token[len("data:"):semi]
	}
	return end, fmt.Sprintf("data:%s;base64,[redacted %d bytes]", mime, end-start)
}
