package chatgptimg

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
)

// OAuthKey 是 ChatGPT Web 渠道使用的凭证载体。
// access_token 优先；refresh_token / session_token 作为可选刷新材料。
type OAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`
	SessionToken string `json:"session_token,omitempty"`

	DeviceID  string `json:"device_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	ClientID  string `json:"client_id,omitempty"`

	Email   string `json:"email,omitempty"`
	Expires string `json:"expires,omitempty"`
	Type    string `json:"type,omitempty"`
}

func ParseOAuthKey(raw string) (*OAuthKey, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, errors.New("chatgpt web channel: empty oauth key")
	}
	var key OAuthKey
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("chatgpt web channel: invalid oauth key json")
	}
	return &key, nil
}

func ValidateOAuthKeyObject(keyMap map[string]any) error {
	if keyMap == nil {
		return errors.New("chatgpt web key JSON must be an object")
	}
	if !hasNonEmptyValue(keyMap, "access_token") &&
		!hasNonEmptyValue(keyMap, "refresh_token") &&
		!hasNonEmptyValue(keyMap, "session_token") {
		return fmt.Errorf("chatgpt web key JSON must include access_token or refresh/session token")
	}
	return nil
}

func NormalizeOAuthKey(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return "", fmt.Errorf("chatgpt web key must be a valid JSON object")
	}
	var keyMap map[string]any
	if err := common.Unmarshal([]byte(trimmed), &keyMap); err != nil {
		return "", fmt.Errorf("chatgpt web key must be a valid JSON object")
	}
	if err := ValidateOAuthKeyObject(keyMap); err != nil {
		return "", err
	}
	normalizedBytes, err := common.Marshal(keyMap)
	if err != nil {
		return "", fmt.Errorf("chatgpt web key JSON 编码失败: %w", err)
	}
	return string(normalizedBytes), nil
}

func NormalizeOAuthKeyValue(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return NormalizeOAuthKey(v)
	case map[string]any:
		if err := ValidateOAuthKeyObject(v); err != nil {
			return "", err
		}
		normalizedBytes, err := common.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("chatgpt web key JSON 编码失败: %w", err)
		}
		return string(normalizedBytes), nil
	default:
		rawBytes, err := common.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("chatgpt web key JSON 编码失败: %w", err)
		}
		return NormalizeOAuthKey(string(rawBytes))
	}
}

func GetOAuthChannelName(rawKey string, currentName string) string {
	if strings.TrimSpace(currentName) != "" {
		return currentName
	}
	var keyMap map[string]any
	if err := common.Unmarshal([]byte(strings.TrimSpace(rawKey)), &keyMap); err != nil {
		return defaultOAuthChannelName()
	}
	if email, ok := keyMap["email"]; ok {
		emailStr := strings.TrimSpace(fmt.Sprintf("%v", email))
		if emailStr != "" {
			return emailStr
		}
	}
	return defaultOAuthChannelName()
}

func GetOAuthBatchKeys(keys string) ([]string, error) {
	trimmed := strings.TrimSpace(keys)
	if trimmed == "" {
		return nil, fmt.Errorf("batch chatgpt web keys不能为空")
	}
	if !strings.HasPrefix(trimmed, "[") {
		return nil, fmt.Errorf("batch chatgpt web keys must use standard JsonArray format")
	}
	var keyArray []any
	if err := common.Unmarshal([]byte(trimmed), &keyArray); err != nil {
		return nil, fmt.Errorf("batch chatgpt web keys must use standard JsonArray format: %w", err)
	}
	cleanKeys := make([]string, 0, len(keyArray))
	for index, keyValue := range keyArray {
		normalizedKey, err := NormalizeOAuthKeyValue(keyValue)
		if err != nil {
			return nil, fmt.Errorf("第 %d 个 chatgpt web key 无效: %w", index+1, err)
		}
		if normalizedKey != "" {
			cleanKeys = append(cleanKeys, normalizedKey)
		}
	}
	if len(cleanKeys) == 0 {
		return nil, fmt.Errorf("batch chatgpt web keys不能为空")
	}
	return cleanKeys, nil
}

func hasNonEmptyValue(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok || v == nil {
		return false
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v)) != ""
}

func defaultOAuthChannelName() string {
	return "chatgpt-web-" + strings.ToLower(common.GetRandomString(6))
}
