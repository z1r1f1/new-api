package model

import (
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/require"
)

func TestLogTokenStatRowIncludesIDForFindInBatchesCursor(t *testing.T) {
	if _, ok := reflect.TypeOf(logTokenStatRow{}).FieldByName("Id"); !ok {
		t.Fatal("logTokenStatRow must include Id so GORM FindInBatches can advance the primary-key cursor")
	}
}

func TestCacheHitRatePartsUsesLoggedPromptTokensForAnthropicUsage(t *testing.T) {
	other, err := common.Marshal(map[string]interface{}{
		"usage_semantic": "anthropic",
		"cache_tokens":   71680,
	})
	if err != nil {
		t.Fatalf("marshal other: %v", err)
	}

	cacheReadTokens, denominator := cacheHitRateParts(logTokenStatRow{
		PromptTokens: 73988,
		Other:        string(other),
	})

	if cacheReadTokens != 71680 {
		t.Fatalf("cacheReadTokens = %v, want 71680", cacheReadTokens)
	}
	if denominator != 73988 {
		t.Fatalf("denominator = %v, want logged prompt tokens", denominator)
	}
}

func TestCacheHitRatePartsPrefersExplicitInputTokensTotal(t *testing.T) {
	other, err := common.Marshal(map[string]interface{}{
		"usage_semantic":     "openai",
		"cache_tokens":       30,
		"input_tokens_total": 180,
	})
	if err != nil {
		t.Fatalf("marshal other: %v", err)
	}

	cacheReadTokens, denominator := cacheHitRateParts(logTokenStatRow{
		PromptTokens: 100,
		Other:        string(other),
	})

	if cacheReadTokens != 30 {
		t.Fatalf("cacheReadTokens = %v, want 30", cacheReadTokens)
	}
	if denominator != 180 {
		t.Fatalf("denominator = %v, want explicit input_tokens_total", denominator)
	}
}

func TestSumUsedQuotaRatesFollowSelectedTimeRange(t *testing.T) {
	truncateTables(t)

	require.NoError(t, LOG_DB.Create([]Log{
		{
			CreatedAt:        1000,
			Type:             LogTypeConsume,
			Quota:            10,
			PromptTokens:     30,
			CompletionTokens: 30,
			UseTime:          2,
			Other:            common.MapToJsonStr(map[string]interface{}{"frt": 1000}),
		},
		{
			CreatedAt:        1120,
			Type:             LogTypeConsume,
			Quota:            20,
			PromptTokens:     80,
			CompletionTokens: 40,
			UseTime:          4,
			Other:            common.MapToJsonStr(map[string]interface{}{"frt": 2000}),
		},
		{
			CreatedAt:        1180,
			Type:             LogTypeConsume,
			Quota:            30,
			PromptTokens:     300,
			CompletionTokens: 300,
			UseTime:          8,
		},
	}).Error)

	firstRange, err := SumUsedQuota(LogTypeConsume, 1000, 1120, "", "", "", 0, "", "", "", "")
	require.NoError(t, err)
	if firstRange.Rpm != 1 {
		t.Fatalf("firstRange.Rpm = %v, want 1", firstRange.Rpm)
	}
	if firstRange.Tpm != 90 {
		t.Fatalf("firstRange.Tpm = %v, want 90", firstRange.Tpm)
	}

	secondRange, err := SumUsedQuota(LogTypeConsume, 1180, 1180, "", "", "", 0, "", "", "", "")
	require.NoError(t, err)
	if secondRange.Rpm != 1 {
		t.Fatalf("secondRange.Rpm = %v, want 1", secondRange.Rpm)
	}
	if secondRange.Tpm != 600 {
		t.Fatalf("secondRange.Tpm = %v, want 600", secondRange.Tpm)
	}

	wideRange, err := SumUsedQuota(LogTypeConsume, 1000, 1240, "", "", "", 0, "", "", "", "")
	require.NoError(t, err)
	require.InDelta(t, 0.75, wideRange.Rpm, 0.000001)
	require.InDelta(t, 195, wideRange.Tpm, 0.000001)
	require.InDelta(t, 14.0/3.0, wideRange.AvgResponseTime, 0.000001)
	require.InDelta(t, 1.5, wideRange.AvgFirstResponseTime, 0.000001)
}
