package common

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"unicode/utf8"

	rootcommon "github.com/QuantumNous/new-api/common"
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

func ReaderWithNormalizedPromptCacheKey(storage rootcommon.BodyStorage) (io.Reader, error) {
	body, err := storage.Bytes()
	if err != nil {
		return nil, err
	}
	normalized, err := NormalizePromptCacheKey(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(normalized), nil
}

func normalizePromptCacheHeaderValue(headerName string, value string) string {
	if headerName != promptCacheSessionIDHeader {
		return value
	}
	return normalizePromptCacheKeyValue(value)
}
