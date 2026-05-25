package service

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const ginKeyChatGPTWebTiming = "chatgpt_web_timing"

// ChatGPTWebTiming records coarse, non-sensitive phase timings for the
// ChatGPT Web adapter. Values are persisted into consume-log Other only when a
// request actually used that adapter.
type ChatGPTWebTiming struct {
	mu     sync.Mutex
	values map[string]interface{}
}

func NewChatGPTWebTiming() *ChatGPTWebTiming {
	return &ChatGPTWebTiming{values: make(map[string]interface{})}
}

func SetChatGPTWebTiming(ctx *gin.Context, timing *ChatGPTWebTiming) {
	if ctx == nil || timing == nil {
		return
	}
	ctx.Set(ginKeyChatGPTWebTiming, timing)
}

func (t *ChatGPTWebTiming) Set(key string, value interface{}) {
	if t == nil || key == "" || value == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.values == nil {
		t.values = make(map[string]interface{})
	}
	t.values[key] = value
}

func (t *ChatGPTWebTiming) ObserveSince(key string, start time.Time) {
	if t == nil || key == "" || start.IsZero() {
		return
	}
	t.Set(key, time.Since(start).Milliseconds())
}

func (t *ChatGPTWebTiming) AddDuration(key string, duration time.Duration) {
	if t == nil || key == "" {
		return
	}
	t.Set(key, duration.Milliseconds())
}

func (t *ChatGPTWebTiming) Snapshot() map[string]interface{} {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(t.values) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(t.values))
	for key, value := range t.values {
		out[key] = value
	}
	return out
}

func appendChatGPTWebTimingInfo(ctx *gin.Context, other map[string]interface{}) {
	if ctx == nil || other == nil {
		return
	}
	value, ok := ctx.Get(ginKeyChatGPTWebTiming)
	if !ok {
		return
	}
	timing, ok := value.(*ChatGPTWebTiming)
	if !ok || timing == nil {
		return
	}
	if snapshot := timing.Snapshot(); len(snapshot) > 0 {
		other["chatgpt_web_timing"] = snapshot
	}
}
