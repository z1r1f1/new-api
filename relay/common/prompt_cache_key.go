package common

import (
	"crypto/sha256"
	"fmt"
	"unicode/utf8"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const maxPromptCacheKeyLength = 64
const promptCacheSessionIDHeader = "session_id"

func normalizePromptCacheKeyValue(value string) string {
	if utf8.RuneCountInString(value) <= maxPromptCacheKeyLength && len(value) <= maxPromptCacheKeyLength {
		return value
	}
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func NormalizePromptCacheKey(jsonData []byte) ([]byte, error) {
	result := gjson.GetBytes(jsonData, "prompt_cache_key")
	if !result.Exists() || result.Type != gjson.String {
		return jsonData, nil
	}

	normalized := normalizePromptCacheKeyValue(result.String())
	if normalized == result.String() {
		return jsonData, nil
	}

	return sjson.SetBytes(jsonData, "prompt_cache_key", normalized)
}

func normalizePromptCacheHeaderValue(headerName string, value string) string {
	if headerName != promptCacheSessionIDHeader {
		return value
	}
	return normalizePromptCacheKeyValue(value)
}
