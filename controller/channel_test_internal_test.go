package controller

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestGetAllChannelsFiltersByGroupWithoutKeyword(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	require.NoError(t, db.Create(&model.Channel{
		Name:   "vip-only",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-vip",
		Models: "gpt-4o",
		Group:  "vip",
		Status: common.ChannelStatusEnabled,
	}).Error)
	require.NoError(t, db.Create(&model.Channel{
		Name:   "default-only",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-default",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel?group=vip&p=1&page_size=20", nil)

	GetAllChannels(ctx)

	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	require.Equal(t, "vip-only", response.Data.Items[0].Name)
}

func TestGetAllChannelsFiltersByLastStatusCode(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	okChannel := &model.Channel{
		Name:   "status-429",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-429",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	okChannel.SetOtherInfo(map[string]interface{}{"last_status_code": 429})
	require.NoError(t, db.Create(okChannel).Error)

	otherChannel := &model.Channel{
		Name:   "status-200",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-200",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	otherChannel.SetOtherInfo(map[string]interface{}{"last_status_code": 200})
	require.NoError(t, db.Create(otherChannel).Error)
	require.NoError(t, db.Create(&model.Channel{
		Name:   "status-empty",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-empty",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel?status_code=429&p=1&page_size=20", nil)

	GetAllChannels(ctx)

	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	require.Equal(t, "status-429", response.Data.Items[0].Name)
}

func TestSearchChannelsFiltersByLastStatusCode(t *testing.T) {
	db := setupModelListControllerTestDB(t)
	matchChannel := &model.Channel{
		Name:   "search-status-500",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-search-500",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	matchChannel.SetOtherInfo(map[string]interface{}{"last_status_code": 500})
	require.NoError(t, db.Create(matchChannel).Error)

	missChannel := &model.Channel{
		Name:   "search-status-200",
		Type:   constant.ChannelTypeOpenAI,
		Key:    "sk-search-200",
		Models: "gpt-4o",
		Group:  "default",
		Status: common.ChannelStatusEnabled,
	}
	missChannel.SetOtherInfo(map[string]interface{}{"last_status_code": 200})
	require.NoError(t, db.Create(missChannel).Error)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("GET", "/api/channel/search?keyword=search&status_code=500&p=1&page_size=20", nil)

	SearchChannels(ctx)

	require.Equal(t, 200, recorder.Code)
	var response struct {
		Success bool `json:"success"`
		Data    struct {
			Items []model.Channel `json:"items"`
			Total int             `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Success)
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Items, 1)
	require.Equal(t, "search-status-500", response.Data.Items[0].Name)
}

func TestSettleTestQuotaUsesTieredBilling(t *testing.T) {
	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode:   "tiered_expr",
			ExprString:    `param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`,
			ExprHash:      billingexpr.ExprHashString(`param("stream") == true ? tier("stream", p * 3) : tier("base", p * 2)`),
			GroupRatio:    1,
			EstimatedTier: "stream",
			QuotaPerUnit:  common.QuotaPerUnit,
			ExprVersion:   1,
		},
		BillingRequestInput: &billingexpr.RequestInput{
			Body: []byte(`{"stream":true}`),
		},
	}

	quota, result := settleTestQuota(info, types.PriceData{
		ModelRatio:      1,
		CompletionRatio: 2,
	}, &dto.Usage{
		PromptTokens: 1000,
	})

	require.Equal(t, 1500, quota)
	require.NotNil(t, result)
	require.Equal(t, "stream", result.MatchedTier)
}

func TestBuildTestLogOtherInjectsTieredInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())

	info := &relaycommon.RelayInfo{
		TieredBillingSnapshot: &billingexpr.BillingSnapshot{
			BillingMode: "tiered_expr",
			ExprString:  `tier("base", p * 2)`,
		},
		ChannelMeta: &relaycommon.ChannelMeta{},
	}
	priceData := types.PriceData{
		GroupRatioInfo: types.GroupRatioInfo{GroupRatio: 1},
	}
	usage := &dto.Usage{
		PromptTokensDetails: dto.InputTokenDetails{
			CachedTokens: 12,
		},
	}

	other := buildTestLogOther(ctx, info, priceData, usage, &billingexpr.TieredResult{
		MatchedTier: "base",
	})

	require.Equal(t, "tiered_expr", other["billing_mode"])
	require.Equal(t, "base", other["matched_tier"])
	require.NotEmpty(t, other["expr_b64"])
}

func TestChatGPTImageChannelTestEndpointDependsOnModelKind(t *testing.T) {
	channel := &model.Channel{Type: constant.ChannelTypeChatGPTImage}

	require.Equal(t,
		string(constant.EndpointTypeImageGeneration),
		normalizeChannelTestEndpoint(channel, "gpt-image-2", ""),
	)
	require.Equal(t,
		string(constant.EndpointTypeImageGeneration),
		normalizeChannelTestEndpoint(channel, "gpt-5.4-pro", ""),
	)
}

func TestShouldUseChatGPTWebImageQuotaTest(t *testing.T) {
	require.True(t, shouldUseChatGPTWebImageQuotaTest(&model.Channel{Type: constant.ChannelTypeChatGPTImage}))
	require.False(t, shouldUseChatGPTWebImageQuotaTest(&model.Channel{Type: constant.ChannelTypeOpenAI}))
	require.False(t, shouldUseChatGPTWebImageQuotaTest(nil))
}

func TestFormatChatGPTWebImageQuotaTestContent(t *testing.T) {
	require.Equal(t,
		"gpt-image-2 图片剩余额度 87/100",
		formatChatGPTWebImageQuotaTestContent(&chatGPTImageBalanceData{ImageQuotaRemaining: 87, ImageQuotaTotal: 100}),
	)
	require.Equal(t,
		"gpt-image-2 图片剩余额度 87",
		formatChatGPTWebImageQuotaTestContent(&chatGPTImageBalanceData{ImageQuotaRemaining: 87}),
	)
	require.Equal(t,
		"gpt-image-2 图片剩余额度未知",
		formatChatGPTWebImageQuotaTestContent(nil),
	)
}

func TestChatGPTWebImageQuotaTestUsesProbeResult(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest("POST", chatGPTWebImageQuotaProbePath, nil)
	ctx.Set("group", "vip")

	oldLogConsumeEnabled := common.LogConsumeEnabled
	oldProbe := updateChatGPTWebImageBalanceForChannelTest
	t.Cleanup(func() {
		common.LogConsumeEnabled = oldLogConsumeEnabled
		updateChatGPTWebImageBalanceForChannelTest = oldProbe
	})
	common.LogConsumeEnabled = false

	var called bool
	updateChatGPTWebImageBalanceForChannelTest = func(_ context.Context, channel *model.Channel, timeout time.Duration) (float64, *chatGPTImageBalanceData, error) {
		called = true
		require.Equal(t, 1111, channel.Id)
		require.Zero(t, timeout)
		return 87, &chatGPTImageBalanceData{ImageQuotaRemaining: 87, ImageQuotaTotal: 100}, nil
	}

	result := testChatGPTWebImageQuota(ctx, &model.Channel{Id: 1111, Type: constant.ChannelTypeChatGPTImage}, time.Now())

	require.True(t, called)
	require.NoError(t, result.localErr)
	require.Nil(t, result.newAPIError)
	require.True(t, result.hasResponseTime)
}

func TestBuildTestRequestForChatGPTImageTextModelUsesChatRequest(t *testing.T) {
	req := buildTestRequest("gpt-5.4-pro", "", &model.Channel{Type: constant.ChannelTypeChatGPTImage}, true)

	chatReq, ok := req.(*dto.GeneralOpenAIRequest)
	require.True(t, ok, "expected text model to use chat request, got %T", req)
	require.Equal(t, "gpt-5.4-pro", chatReq.Model)
	require.NotNil(t, chatReq.Stream)
	require.True(t, *chatReq.Stream)
	require.Len(t, chatReq.Messages, 1)
}
