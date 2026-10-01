package channel_test

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httptrace"
	"testing"
	"testing/iotest"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequestTimingCapturesIndependentStages(t *testing.T) {
	start := time.Now()
	trace := common.NewRequestTiming("0123456789abcdef0123456789abcdef", start)
	trace.RecordBodyRead(start.Add(5*time.Millisecond), start.Add(35*time.Millisecond), 1024)
	for stage, offset := range map[common.RequestTimingStage]time.Duration{
		common.TimingSelectStart: 40, common.TimingChannelSelected: 48,
		common.TimingUpstreamStart: 60, common.TimingUpstreamConnected: 65,
		common.TimingUpstreamBodySent: 85, common.TimingUpstreamFirstByte: 112,
		common.TimingUpstreamHeaders: 115, common.TimingFirstData: 145,
	} {
		trace.Mark(stage, start.Add(offset*time.Millisecond))
	}
	snapshot := trace.Snapshot(start.Add(200 * time.Millisecond))
	assert.Equal(t, "0123456789abcdef0123456789abcdef", snapshot.NginxID)
	assert.EqualValues(t, 1024, snapshot.BodyBytes)
	assert.Equal(t, map[string]int64{
		"body_read": 30, "channel_select": 8, "pre_channel": 48,
		"selected_to_upstream": 12, "upstream_to_body_sent": 25,
		"upstream_conn_acquire": 5, "upstream_body_send": 20,
		"upstream_wait_first_byte": 27, "upstream_header_decode": 3,
		"upstream_wait_headers": 30, "selected_to_first_data": 97, "total": 200,
	}, snapshot.DurationsMS)
	assert.NotContains(t, snapshot.EventsMS, "missing_stage")
}

func TestGetRequestBodyRecordsUploadOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	body := []byte(`{"model":"test","input":"large context"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	trace := common.NewRequestTiming("0123456789abcdef0123456789abcdef", time.Now())
	common.SetRequestTiming(c, trace)
	storage, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, storage.Close()) })
	first := trace.Snapshot(time.Now())
	assert.EqualValues(t, len(body), first.BodyBytes)
	assert.Contains(t, first.EventsMS, string(common.TimingBodyReadStart))
	assert.Contains(t, first.EventsMS, string(common.TimingBodyReadDone))
	assert.Contains(t, first.DurationsMS, "body_read")
	cached, err := common.GetBodyStorage(c)
	require.NoError(t, err)
	data, err := cached.Bytes()
	require.NoError(t, err)
	assert.Equal(t, body, data)
	second := trace.Snapshot(time.Now())
	assert.Equal(t, first.EventsMS[string(common.TimingBodyReadStart)], second.EventsMS[string(common.TimingBodyReadStart)])
	assert.Equal(t, first.EventsMS[string(common.TimingBodyReadDone)], second.EventsMS[string(common.TimingBodyReadDone)])
}

func TestRequestTimingOnlyTracesMarked8999Requests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.RequestId(), middleware.RequestTiming())
	var traced bool
	engine.GET("/test", func(c *gin.Context) {
		traced = common.GetRequestTiming(c) != nil
		c.Status(http.StatusNoContent)
	})
	for _, tc := range []struct {
		id   string
		want bool
	}{
		{"", false}, {"invalid\nlog", false}, {"0123456789abcdef0123456789abcdef", true},
	} {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(common.RequestTimingHeader, tc.id)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		assert.Equal(t, http.StatusNoContent, response.Code)
		assert.Equal(t, tc.want, traced, "header %q", tc.id)
	}
}

func TestDoRequestRecordsSuccessfulTransportStages(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	trace := common.NewRequestTiming("0123456789abcdef0123456789abcdef", time.Now())
	common.SetRequestTiming(c, trace)
	req, err := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader([]byte("request body")))
	require.NoError(t, err)
	resp, err := channel.DoRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, resp.Body.Close()) })
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	snapshot := trace.Snapshot(time.Now())
	for _, stage := range []common.RequestTimingStage{common.TimingUpstreamStart, common.TimingUpstreamConnected, common.TimingUpstreamBodySent, common.TimingUpstreamFirstByte, common.TimingUpstreamHeaders} {
		assert.Contains(t, snapshot.EventsMS, string(stage))
	}
	assert.Contains(t, snapshot.DurationsMS, "upstream_body_send")
	assert.Contains(t, snapshot.DurationsMS, "upstream_wait_headers")
}

func TestDoRequestDoesNotMarkFailedBodyWriteAsSent(t *testing.T) {
	service.InitHttpClient()
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	t.Cleanup(server.Close)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	trace := common.NewRequestTiming("0123456789abcdef0123456789abcdef", time.Now())
	common.SetRequestTiming(c, trace)
	req, err := http.NewRequest(http.MethodPost, server.URL, iotest.ErrReader(errors.New("fixture body read failed")))
	require.NoError(t, err)
	wrote := make(chan error, 1)
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		WroteRequest: func(info httptrace.WroteRequestInfo) { wrote <- info.Err },
	}))
	resp, err := channel.DoRequest(c, req, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}})
	require.Error(t, err)
	assert.Nil(t, resp)
	require.Len(t, wrote, 1)
	assert.Error(t, <-wrote)
	snapshot := trace.Snapshot(time.Now())
	assert.Contains(t, snapshot.EventsMS, string(common.TimingUpstreamConnected))
	assert.NotContains(t, snapshot.EventsMS, string(common.TimingUpstreamBodySent))
	assert.NotContains(t, snapshot.DurationsMS, "upstream_wait_headers")
}

func TestRequestTimingRetriesKeepAttemptStagesSeparate(t *testing.T) {
	start := time.Now()
	trace := common.NewRequestTiming("0123456789abcdef0123456789abcdef", start)
	recorder, ok := any(trace).(interface {
		BeginUpstreamAttempt(time.Time) int
		MarkUpstreamAttempt(int, common.RequestTimingStage, time.Time)
	})
	require.True(t, ok, "outbound timings must isolate each Client.Do invocation")
	first := recorder.BeginUpstreamAttempt(start.Add(60 * time.Millisecond))
	recorder.MarkUpstreamAttempt(first, common.TimingUpstreamConnected, start.Add(65*time.Millisecond))
	second := recorder.BeginUpstreamAttempt(start.Add(100 * time.Millisecond))
	recorder.MarkUpstreamAttempt(first, common.TimingUpstreamFirstByte, start.Add(101*time.Millisecond))
	recorder.MarkUpstreamAttempt(second, common.TimingUpstreamConnected, start.Add(105*time.Millisecond))
	recorder.MarkUpstreamAttempt(second, common.TimingUpstreamBodySent, start.Add(110*time.Millisecond))
	recorder.MarkUpstreamAttempt(second, common.TimingUpstreamFirstByte, start.Add(143*time.Millisecond))
	recorder.MarkUpstreamAttempt(first, common.TimingUpstreamHeaders, start.Add(144*time.Millisecond))
	snapshot := trace.Snapshot(start.Add(150 * time.Millisecond))
	assert.EqualValues(t, 100, snapshot.EventsMS[string(common.TimingUpstreamStart)])
	assert.EqualValues(t, 143, snapshot.EventsMS[string(common.TimingUpstreamFirstByte)])
	assert.NotContains(t, snapshot.EventsMS, string(common.TimingUpstreamHeaders))
	assert.EqualValues(t, 5, snapshot.DurationsMS["upstream_conn_acquire"])
	assert.EqualValues(t, 33, snapshot.DurationsMS["upstream_wait_first_byte"])
	data, err := common.Marshal(snapshot)
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, common.Unmarshal(data, &payload))
	assert.EqualValues(t, 2, payload["upstream_attempts"])
}
