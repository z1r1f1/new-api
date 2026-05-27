package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/gin-gonic/gin"
)

func TestRecordConsumeLogMarksRequestContextAfterPersisting(t *testing.T) {
	gin.SetMode(gin.TestMode)
	truncateTables(t)

	originalLogConsumeEnabled := common.LogConsumeEnabled
	t.Cleanup(func() {
		common.LogConsumeEnabled = originalLogConsumeEnabled
	})
	common.LogConsumeEnabled = true

	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordConsumeLog(ctx, 1, RecordConsumeLogParams{
		ChannelId:        2,
		PromptTokens:     3,
		CompletionTokens: 4,
		ModelName:        "gpt-test",
		TokenName:        "token",
		Quota:            5,
		TokenId:          6,
		Group:            "default",
	})

	if !common.GetContextKeyBool(ctx, constant.ContextKeyConsumeLogRecorded) {
		t.Fatal("consume log context marker was not set after successful log persistence")
	}
}
