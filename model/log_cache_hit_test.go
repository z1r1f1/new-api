package model

import (
	"reflect"
	"testing"

	"github.com/QuantumNous/new-api/common"
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
