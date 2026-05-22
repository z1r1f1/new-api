package relay

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupImageHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.UsingSQLite = true
	common.UsingMySQL = false
	common.UsingPostgreSQL = false
	common.RedisEnabled = false

	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	model.DB = db
	model.LOG_DB = db
	if err := db.AutoMigrate(&model.Midjourney{}); err != nil {
		t.Fatalf("migrate midjourney: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRecordImageGenerationDrawingLogPersistsOpenAIImageResponse(t *testing.T) {
	db := setupImageHandlerTestDB(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyImageGenerationResponse, &dto.ImageResponse{
		Created: 123,
		Data: []dto.ImageData{{
			Url:           "https://example.com/generated.png",
			RevisedPrompt: "revised cat",
		}},
	})
	startedAt := time.Unix(100, 0)
	info := &relaycommon.RelayInfo{
		UserId:                7,
		OriginModelName:       "gpt-image-2",
		RequestURLPath:        "/v1/images/generations",
		StartTime:             startedAt,
		FinalPreConsumedQuota: 42,
		ChannelMeta:           &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI, ChannelId: 12, UpstreamModelName: "gpt-image-2-upstream"},
	}
	req := &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw cat", Size: "1024x1024", Quality: "high", ResponseFormat: "url"}

	recordImageGenerationDrawingLog(ctx, info, req)

	var row model.Midjourney
	if err := db.First(&row).Error; err != nil {
		t.Fatalf("expected drawing log row: %v", err)
	}
	if row.UserId != 7 || row.ChannelId != 12 || row.Quota != 42 {
		t.Fatalf("unexpected row ownership/quota: %#v", row)
	}
	if row.Prompt != "revised cat" || row.State != "gpt-image-2" || row.ImageUrl != "https://example.com/generated.png" {
		t.Fatalf("unexpected row content: %#v", row)
	}
	if row.Description != constant.GetChannelTypeName(constant.ChannelTypeOpenAI) {
		t.Fatalf("unexpected description: %q", row.Description)
	}
	if !strings.Contains(row.Properties, `"endpoint":"/v1/images/generations"`) || !strings.Contains(row.Properties, `"source":"openai-image"`) {
		t.Fatalf("properties missing image log metadata: %s", row.Properties)
	}
	if row.SubmitTime != startedAt.UnixMilli() || row.StartTime != startedAt.UnixMilli() {
		t.Fatalf("unexpected timestamps: %#v", row)
	}
}

func TestRecordImageGenerationDrawingLogSkipsChatGPTWebDuplicate(t *testing.T) {
	db := setupImageHandlerTestDB(t)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	common.SetContextKey(ctx, constant.ContextKeyImageGenerationResponse, &dto.ImageResponse{
		Data: []dto.ImageData{{Url: "https://example.com/generated.png"}},
	})
	info := &relaycommon.RelayInfo{
		UserId:      7,
		ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeChatGPTImage, ChannelId: 12},
	}

	recordImageGenerationDrawingLog(ctx, info, &dto.ImageRequest{Model: "gpt-image-2", Prompt: "draw cat"})

	var count int64
	if err := db.Model(&model.Midjourney{}).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected no duplicate ChatGPT Web drawing log, got %d", count)
	}
}

func TestImageDataDrawingLogURLBuildsDataURL(t *testing.T) {
	if got := imageDataDrawingLogURL(dto.ImageData{B64Json: "abc"}); got != "data:image/png;base64,abc" {
		t.Fatalf("unexpected data URL: %q", got)
	}
	if got := imageDataDrawingLogURL(dto.ImageData{B64Json: "data:image/jpeg;base64,abc"}); got != "data:image/jpeg;base64,abc" {
		t.Fatalf("unexpected existing data URL: %q", got)
	}
}
