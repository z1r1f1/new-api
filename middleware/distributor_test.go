package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newGetModelRequestTestContext(method, target string) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	return ctx
}

func TestGetModelRequestReadsResponsesWebSocketCompatibilityModelFromQuery(t *testing.T) {
	ctx := newGetModelRequestTestContext(http.MethodGet, "/v1/responses?model=gpt-4o-realtime-preview")

	req, shouldSelectChannel, err := getModelRequest(ctx)
	if err != nil {
		t.Fatalf("expected responses websocket compatibility request to parse, got error: %v", err)
	}
	if !shouldSelectChannel {
		t.Fatal("expected responses websocket compatibility request to select a channel")
	}
	if req.Model != "gpt-4o-realtime-preview" {
		t.Fatalf("expected model from query, got %q", req.Model)
	}
}

func newChannelAffinityRecordTestContext(status int, info *relaycommon.RelayInfo) *gin.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	if status != 0 {
		ctx.Status(status)
	}
	if info != nil {
		common.SetContextKey(ctx, constant.ContextKeyRelayInfo, info)
	}
	return ctx
}

func TestShouldRecordChannelAffinityAllowsMissingRelayInfo(t *testing.T) {
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, nil)

	if !shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record without relay info")
	}
}

func TestShouldRecordChannelAffinityRejectsHTTPErrorStatus(t *testing.T) {
	ctx := newChannelAffinityRecordTestContext(http.StatusInternalServerError, nil)

	if shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record to be skipped for HTTP error status")
	}
}

func TestShouldRecordChannelAffinityAllowsNormalStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: status,
	})

	if !shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record for normal stream")
	}
}

func TestShouldRecordChannelAffinityRejectsAbnormalStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonScannerErr, fmt.Errorf("stream ID 1: INTERNAL_ERROR"))
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: status,
	})

	if shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record to be skipped for scanner_error stream")
	}
}

func TestShouldRecordChannelAffinityRejectsSoftErrorStream(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	status.RecordError("chunk parse error")
	ctx := newChannelAffinityRecordTestContext(http.StatusOK, &relaycommon.RelayInfo{
		IsStream:     true,
		StreamStatus: status,
	})

	if shouldRecordChannelAffinity(ctx) {
		t.Fatal("expected channel affinity record to be skipped for stream with soft errors")
	}
}

func TestDistributeUsesChatGPTWebSessionChannelAffinityAfterConfiguredAffinityMiss(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupDistributorTestDB(t)
	oldMemoryCacheEnabled := common.MemoryCacheEnabled
	oldRedisEnabled := common.RedisEnabled
	common.MemoryCacheEnabled = true
	common.RedisEnabled = false
	t.Cleanup(func() {
		_ = model.DB.Exec("DELETE FROM abilities").Error
		_ = model.DB.Exec("DELETE FROM channels").Error
		common.MemoryCacheEnabled = oldMemoryCacheEnabled
		common.RedisEnabled = oldRedisEnabled
	})

	_ = model.DB.Exec("DELETE FROM abilities").Error
	_ = model.DB.Exec("DELETE FROM channels").Error
	insertDistributorChannel(t, 3101, constant.ChannelTypeChatGPTImage, 1)
	insertDistributorChannel(t, 3102, constant.ChannelTypeOpenAI, 10)
	model.InitChannelCache()

	body := `{"model":"gpt-5.5-thinking","prompt_cache_key":"chatgpt-web-session-affinity-router"}`
	seed := newGetModelRequestTestContext(http.MethodPost, "/v1/chat/completions")
	seed.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	seed.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(seed, constant.ContextKeyUsingGroup, "vip")
	seed.Set("original_model", "gpt-5.5-thinking")
	service.RecordChatGPTWebSessionChannelAffinity(seed, &model.Channel{
		Id:     3101,
		Type:   constant.ChannelTypeChatGPTImage,
		Status: common.ChannelStatusEnabled,
	})
	t.Cleanup(func() { service.ClearCurrentChatGPTWebSessionChannelAffinity(seed) })

	router := gin.New()
	router.Use(func(c *gin.Context) {
		common.SetContextKey(c, constant.ContextKeyUsingGroup, "vip")
		common.SetContextKey(c, constant.ContextKeyUserGroup, "vip")
		c.Next()
	})
	router.Use(Distribute())
	router.POST("/v1/chat/completions", func(c *gin.Context) {
		c.String(http.StatusOK, fmt.Sprintf("%d", c.GetInt("channel_id")))
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.TrimSpace(rec.Body.String()); got != "3101" {
		t.Fatalf("expected ChatGPT Web session affinity channel 3101, got %s", got)
	}
}

func insertDistributorChannel(t *testing.T, id int, channelType int, priority int64) {
	t.Helper()
	weight := uint(100)
	require.NoError(t, model.DB.Create(&model.Channel{
		Id:       id,
		Type:     channelType,
		Key:      fmt.Sprintf("test-key-%d", id),
		Status:   common.ChannelStatusEnabled,
		Name:     fmt.Sprintf("test-channel-%d", id),
		Priority: &priority,
		Weight:   &weight,
		Models:   "gpt-5.5-thinking",
		Group:    "vip",
	}).Error)
	require.NoError(t, model.DB.Create(&model.Ability{
		Group:     "vip",
		Model:     "gpt-5.5-thinking",
		ChannelId: id,
		Enabled:   true,
		Priority:  &priority,
		Weight:    weight,
	}).Error)
}

func setupDistributorTestDB(t *testing.T) {
	t.Helper()
	previousDB := model.DB
	previousLogDB := model.LOG_DB
	previousUsingSQLite := common.UsingSQLite
	previousUsingMySQL := common.UsingMySQL
	previousUsingPostgreSQL := common.UsingPostgreSQL

	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false

	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Ability{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
		model.DB = previousDB
		model.LOG_DB = previousLogDB
		common.UsingSQLite = previousUsingSQLite
		common.UsingMySQL = previousUsingMySQL
		common.UsingPostgreSQL = previousUsingPostgreSQL
		if common.MemoryCacheEnabled && model.DB != nil {
			model.InitChannelCache()
		}
	})
}
