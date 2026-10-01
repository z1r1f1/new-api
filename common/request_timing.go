package common

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const RequestTimingHeader = "X-New-Api-Perf-Id"

const requestTimingContextKey = "new_api_8999_request_timing"

type RequestTimingStage string

const (
	TimingBodyReadStart     RequestTimingStage = "body_read_start"
	TimingBodyReadDone      RequestTimingStage = "body_read_done"
	TimingSelectStart       RequestTimingStage = "select_start"
	TimingChannelSelected   RequestTimingStage = "channel_selected"
	TimingUpstreamStart     RequestTimingStage = "upstream_start"
	TimingUpstreamConnected RequestTimingStage = "upstream_connected"
	TimingUpstreamBodySent  RequestTimingStage = "upstream_body_sent"
	TimingUpstreamFirstByte RequestTimingStage = "upstream_first_byte"
	TimingUpstreamHeaders   RequestTimingStage = "upstream_headers"
	TimingFirstData         RequestTimingStage = "first_data"
)

// RequestTiming holds timestamps for one 8999 request. HTTP client trace callbacks
// can run in other goroutines, so all updates and snapshots share one mutex.
type RequestTiming struct {
	mu               sync.Mutex
	nginxID          string
	started          time.Time
	bodyBytes        int64
	events           map[RequestTimingStage]time.Time
	upstreamAttempts int
}

type RequestTimingSnapshot struct {
	NginxID          string           `json:"nginx_id"`
	BodyBytes        int64            `json:"body_bytes"`
	EventsMS         map[string]int64 `json:"events_ms"`
	DurationsMS      map[string]int64 `json:"durations_ms"`
	UpstreamAttempts int              `json:"upstream_attempts,omitempty"`
}

func NewRequestTiming(nginxID string, started time.Time) *RequestTiming {
	return &RequestTiming{nginxID: nginxID, started: started, events: make(map[RequestTimingStage]time.Time)}
}

func SetRequestTiming(c *gin.Context, timing *RequestTiming) {
	c.Set(requestTimingContextKey, timing)
}

func GetRequestTiming(c *gin.Context) *RequestTiming {
	if c == nil {
		return nil
	}
	timing, _ := c.Get(requestTimingContextKey)
	result, _ := timing.(*RequestTiming)
	return result
}

func (t *RequestTiming) Mark(stage RequestTimingStage, at time.Time) {
	if t == nil || at.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.events[stage]; !exists {
		t.events[stage] = at
	}
}

func (t *RequestTiming) RecordBodyRead(start, end time.Time, size int64) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.events[TimingBodyReadStart]; !exists {
		t.events[TimingBodyReadStart] = start
		t.events[TimingBodyReadDone] = end
		t.bodyBytes = size
	}
}

// BeginUpstreamAttempt starts one Client.Do invocation. Transparent retries
// inside net/http share this ID; application-level retries get a new ID.
func (t *RequestTiming) BeginUpstreamAttempt(at time.Time) int {
	if t == nil || at.IsZero() {
		return 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.upstreamAttempts++
	for _, stage := range []RequestTimingStage{TimingUpstreamStart, TimingUpstreamConnected, TimingUpstreamBodySent, TimingUpstreamFirstByte, TimingUpstreamHeaders} {
		delete(t.events, stage)
	}
	t.events[TimingUpstreamStart] = at
	return t.upstreamAttempts
}

// MarkUpstreamAttempt ignores callbacks from previous outbound invocations.
func (t *RequestTiming) MarkUpstreamAttempt(attempt int, stage RequestTimingStage, at time.Time) {
	if t == nil || at.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if attempt != t.upstreamAttempts || attempt <= 0 {
		return
	}
	if _, exists := t.events[stage]; !exists {
		t.events[stage] = at
	}
}

func (t *RequestTiming) Snapshot(end time.Time) RequestTimingSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	snapshot := RequestTimingSnapshot{
		NginxID: t.nginxID, BodyBytes: t.bodyBytes,
		EventsMS: make(map[string]int64), DurationsMS: make(map[string]int64),
		UpstreamAttempts: t.upstreamAttempts,
	}
	for stage, at := range t.events {
		if !at.Before(t.started) {
			snapshot.EventsMS[string(stage)] = at.Sub(t.started).Milliseconds()
		}
	}
	snapshot.EventsMS["request_end"] = end.Sub(t.started).Milliseconds()
	snapshot.DurationsMS["total"] = end.Sub(t.started).Milliseconds()
	addDuration := func(name string, from, to RequestTimingStage) {
		start, hasStart := t.events[from]
		finish, hasFinish := t.events[to]
		if hasStart && hasFinish && !finish.Before(start) {
			snapshot.DurationsMS[name] = finish.Sub(start).Milliseconds()
		}
	}
	addDuration("body_read", TimingBodyReadStart, TimingBodyReadDone)
	addDuration("channel_select", TimingSelectStart, TimingChannelSelected)
	addDuration("upstream_to_body_sent", TimingUpstreamStart, TimingUpstreamBodySent)
	addDuration("upstream_conn_acquire", TimingUpstreamStart, TimingUpstreamConnected)
	addDuration("upstream_body_send", TimingUpstreamConnected, TimingUpstreamBodySent)
	addDuration("upstream_wait_headers", TimingUpstreamBodySent, TimingUpstreamHeaders)
	addDuration("upstream_wait_first_byte", TimingUpstreamBodySent, TimingUpstreamFirstByte)
	addDuration("upstream_header_decode", TimingUpstreamFirstByte, TimingUpstreamHeaders)
	if selected, ok := t.events[TimingChannelSelected]; ok {
		snapshot.DurationsMS["pre_channel"] = selected.Sub(t.started).Milliseconds()
		if outbound, ok := t.events[TimingUpstreamStart]; ok && !outbound.Before(selected) {
			snapshot.DurationsMS["selected_to_upstream"] = outbound.Sub(selected).Milliseconds()
		}
		if firstData, ok := t.events[TimingFirstData]; ok && !firstData.Before(selected) {
			snapshot.DurationsMS["selected_to_first_data"] = firstData.Sub(selected).Milliseconds()
		}
	}
	return snapshot
}
