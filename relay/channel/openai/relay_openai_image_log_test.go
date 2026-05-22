package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/gin-gonic/gin"
)

func TestOpenaiHandlerWithUsageStoresImageResponseForDrawingLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{
			"created": 123,
			"data": [{"url": "https://example.com/generated.png"}],
			"usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2}
		}`)),
	}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}

	usage, apiErr := OpenaiHandlerWithUsage(ctx, info, resp)
	if apiErr != nil {
		t.Fatalf("OpenaiHandlerWithUsage returned error: %v", apiErr)
	}
	if usage == nil || usage.TotalTokens != 2 {
		t.Fatalf("unexpected usage: %#v", usage)
	}
	stored, ok := common.GetContextKeyType[*dto.ImageResponse](ctx, constant.ContextKeyImageGenerationResponse)
	if !ok || stored == nil || len(stored.Data) != 1 {
		t.Fatalf("expected stored image response, got ok=%v stored=%#v", ok, stored)
	}
	if stored.Data[0].Url != "https://example.com/generated.png" {
		t.Fatalf("unexpected stored image URL: %q", stored.Data[0].Url)
	}
}

func TestOpenaiHandlerWithUsageIgnoresNonImageResponsesForDrawingLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"usage":{"total_tokens":1}}`)),
	}
	info := &relaycommon.RelayInfo{RelayMode: relayconstant.RelayModeImagesGenerations, ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeOpenAI}}

	_, apiErr := OpenaiHandlerWithUsage(ctx, info, resp)
	if apiErr != nil {
		t.Fatalf("OpenaiHandlerWithUsage returned error: %v", apiErr)
	}
	if stored, ok := common.GetContextKeyType[*dto.ImageResponse](ctx, constant.ContextKeyImageGenerationResponse); ok || stored != nil {
		t.Fatalf("expected no stored image response, got ok=%v stored=%#v", ok, stored)
	}
}
