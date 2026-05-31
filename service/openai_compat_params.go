package service

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

const defaultFastServiceTier = "priority"

var openAICompatTopLevelSessionKeys = []string{
	"session_id",
	"sessionId",
	"conversation_id",
	"conversationId",
	"thread_id",
	"threadId",
	"request_session_id",
}

var openAICompatMetadataSessionKeys = []string{
	"session_id",
	"sessionId",
	"conversation_id",
	"conversationId",
	"thread_id",
	"threadId",
	"claude_session_id",
	"claudeSessionId",
	"codex_session_id",
	"codexSessionId",
}

var openAICompatMetadataUserIDSessionKeys = []string{
	"session_id",
	"sessionId",
	"conversation_id",
	"conversationId",
}

var openAICompatHeaderSessionKeys = []string{
	"Session_id",
	"session_id",
	"Session-Id",
	"session-id",
	"X-Session-Id",
	"x-session-id",
	"X-Conversation-Id",
	"x-conversation-id",
	"X-Thread-Id",
	"x-thread-id",
	"X-Claude-Session-Id",
	"x-claude-session-id",
	"X-Claude-Code-Session-Id",
	"x-claude-code-session-id",
	"X-Codex-Session-Id",
	"x-codex-session-id",
}

// ApplyOpenAICompatRequestParamsFromRawBody preserves compatibility-only
// request parameters that are not represented by Claude's public DTO but are
// used by Codex/OpenAI Responses clients. It intentionally writes into the
// OpenAI-compatible intermediate request so existing RemoveDisabledFields and
// channel allow-list settings still decide what is actually sent upstream.
func ApplyOpenAICompatRequestParamsFromRawBody(req *dto.GeneralOpenAIRequest, body []byte, headers map[string]string) {
	if req == nil {
		return
	}

	var data map[string]json.RawMessage
	if len(body) > 0 {
		_ = common.Unmarshal(body, &data)
	}

	if serviceTier := normalizeServiceTierRaw(req.ServiceTier); serviceTier != "" {
		setGeneralRequestServiceTier(req, serviceTier)
	} else if serviceTier := extractOpenAICompatServiceTier(data, headers); serviceTier != "" {
		setGeneralRequestServiceTier(req, serviceTier)
	}

	if req.ReasoningEffort == "" {
		if effort := extractOpenAICompatEffort(data); effort != "" {
			req.ReasoningEffort = effort
		}
	}

	if strings.TrimSpace(req.PromptCacheKey) == "" {
		if key := extractOpenAICompatPromptCacheKey(data, headers); key != "" {
			req.PromptCacheKey = key
		}
	}
}

func ApplyOpenAIResponsesCompatRequestParamsFromRawBody(req *dto.OpenAIResponsesRequest, body []byte, headers map[string]string) {
	if req == nil {
		return
	}

	var data map[string]json.RawMessage
	if len(body) > 0 {
		_ = common.Unmarshal(body, &data)
	}

	if serviceTier := normalizeFastServiceTier(req.ServiceTier); serviceTier != "" {
		req.ServiceTier = serviceTier
	} else if serviceTier := extractOpenAICompatServiceTier(data, headers); serviceTier != "" {
		req.ServiceTier = serviceTier
	}

	if req.Reasoning == nil || strings.TrimSpace(req.Reasoning.Effort) == "" {
		if effort := extractOpenAICompatEffort(data); effort != "" {
			if req.Reasoning == nil {
				req.Reasoning = &dto.Reasoning{}
			}
			req.Reasoning.Effort = effort
		}
	}

	if len(req.PromptCacheKey) == 0 {
		if key := extractOpenAICompatPromptCacheKey(data, headers); key != "" {
			req.PromptCacheKey, _ = common.Marshal(key)
		}
	}
}

func ExtractOpenAICompatPromptCacheKeyFromRawBody(body []byte, headers map[string]string) string {
	var data map[string]json.RawMessage
	if len(body) > 0 {
		if err := common.Unmarshal(body, &data); err == nil {
			if key := extractOpenAICompatPromptCacheKey(data, headers); key != "" {
				return key
			}
		}
		if key := extractOpenAICompatPromptCacheKeyFromFormBody(body, headers); key != "" {
			return key
		}
	}
	return extractOpenAICompatPromptCacheKey(nil, headers)
}

func setGeneralRequestServiceTier(req *dto.GeneralOpenAIRequest, serviceTier string) {
	serviceTier = normalizeFastServiceTier(serviceTier)
	if serviceTier == "" || req == nil {
		return
	}
	if raw, err := common.Marshal(serviceTier); err == nil {
		req.ServiceTier = raw
	}
}

func normalizeServiceTierRaw(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return normalizeFastServiceTier(value)
}

func normalizeFastServiceTier(value string) string {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "fast") {
		return defaultFastServiceTier
	}
	return value
}

func extractOpenAICompatServiceTier(data map[string]json.RawMessage, headers map[string]string) string {
	for _, key := range []string{"service_tier", "openai_service_tier"} {
		if value := stringFieldFromRawMap(data, key); value != "" {
			return normalizeFastServiceTier(value)
		}
	}
	metadata := objectFieldFromRawMap(data, "metadata")
	for _, key := range []string{"service_tier", "openai_service_tier"} {
		if value := stringFieldFromRawMap(metadata, key); value != "" {
			return normalizeFastServiceTier(value)
		}
	}

	if fastServiceTier := headerValue(headers, headerClaudeCodeProxyFastServiceTier); fastServiceTier != "" {
		return normalizeFastServiceTier(fastServiceTier)
	}
	if fast, ok := extractOpenAICompatFastFlag(data, headers); ok && fast {
		if tier := headerValue(headers, headerClaudeCodeProxyFastServiceTier); tier != "" {
			return normalizeFastServiceTier(tier)
		}
		return defaultFastServiceTier
	} else if ok {
		return ""
	}
	if isClaudeCodeCLIRequest(headers) {
		return defaultFastServiceTier
	}
	return ""
}

func isClaudeCodeCLIRequest(headers map[string]string) bool {
	userAgent := strings.ToLower(headerValue(headers, "User-Agent"))
	if strings.Contains(userAgent, "claude-cli/") ||
		strings.Contains(userAgent, "claude-code/") ||
		strings.Contains(userAgent, "claude_code/") ||
		strings.Contains(userAgent, "claude code") ||
		strings.Contains(userAgent, "claude-code") ||
		strings.Contains(userAgent, "claude_code") {
		return true
	}
	if headerValue(headers, "X-Claude-Code-Session-Id") != "" ||
		headerValue(headers, "X-Claude-Session-Id") != "" {
		return true
	}
	return false
}

func extractOpenAICompatFastFlag(data map[string]json.RawMessage, headers map[string]string) (bool, bool) {
	if value := headerValue(headers, headerClaudeCodeProxyFast); value != "" {
		return parseBoolishValue(value)
	}
	for _, key := range []string{"fast", "fastMode"} {
		if fast, ok := boolishFieldFromRawMap(data, key); ok {
			return fast, true
		}
	}
	metadata := objectFieldFromRawMap(data, "metadata")
	for _, key := range []string{"fast", "fastMode"} {
		if fast, ok := boolishFieldFromRawMap(metadata, key); ok {
			return fast, true
		}
	}
	return false, false
}

func extractOpenAICompatEffort(data map[string]json.RawMessage) string {
	if reasoning := objectFieldFromRawMap(data, "reasoning"); reasoning != nil {
		if effort := stringFieldFromRawMap(reasoning, "effort"); effort != "" {
			return effort
		}
	}
	if thinking := objectFieldFromRawMap(data, "thinking"); thinking != nil {
		if effort := stringFieldFromRawMap(thinking, "effort"); effort != "" {
			return effort
		}
	}
	if outputConfig := objectFieldFromRawMap(data, "output_config"); outputConfig != nil {
		for _, key := range []string{"effort", "think_effort", "reasoning_effort", "model_reasoning_effort"} {
			if effort := stringFieldFromRawMap(outputConfig, key); effort != "" {
				return effort
			}
		}
	}
	for _, source := range []map[string]json.RawMessage{data, objectFieldFromRawMap(data, "metadata")} {
		for _, key := range []string{"think_effort", "reasoning_effort", "model_reasoning_effort", "effort", "effortLevel"} {
			if effort := stringFieldFromRawMap(source, key); effort != "" {
				return effort
			}
		}
	}
	return ""
}

func extractOpenAICompatPromptCacheKey(data map[string]json.RawMessage, headers map[string]string) string {
	// Keep the primary extraction order aligned with claude-code-proxy's
	// _extract_prompt_cache_key_info: explicit top-level cache key, explicit
	// metadata cache key, then metadata.user_id (derived nested session first,
	// otherwise the whole user_id string). The broader session/header aliases
	// below are kept only as new-api compatibility fallbacks when the
	// claude-code-proxy-compatible sources are absent.
	for _, key := range []string{"prompt_cache_key", "openai_prompt_cache_key"} {
		if value := stringFieldFromRawMap(data, key); value != "" {
			return value
		}
	}
	metadata := objectFieldFromRawMap(data, "metadata")
	for _, key := range []string{"prompt_cache_key", "openai_prompt_cache_key"} {
		if value := stringFieldFromRawMap(metadata, key); value != "" {
			return value
		}
	}
	if userID := stringFieldFromRawMap(metadata, "user_id"); userID != "" {
		if sessionKey := deriveSessionKeyFromMetadataUserID(userID); sessionKey != "" {
			return sessionKey
		}
		return userID
	}
	for _, key := range openAICompatTopLevelSessionKeys {
		if value := stringFieldFromRawMap(data, key); value != "" {
			return value
		}
	}
	for _, key := range openAICompatMetadataSessionKeys {
		if value := stringFieldFromRawMap(metadata, key); value != "" {
			return value
		}
	}
	for _, key := range openAICompatHeaderSessionKeys {
		if value := headerValue(headers, key); value != "" {
			return value
		}
	}
	return ""
}

func deriveSessionKeyFromMetadataUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" || !strings.HasPrefix(userID, "{") {
		return ""
	}
	var nested map[string]json.RawMessage
	if err := common.Unmarshal([]byte(userID), &nested); err != nil {
		return ""
	}
	for _, key := range openAICompatMetadataUserIDSessionKeys {
		if value := stringFieldFromRawMap(nested, key); value != "" {
			return value
		}
	}
	return ""
}

func extractOpenAICompatPromptCacheKeyFromFormBody(body []byte, headers map[string]string) string {
	values := openAICompatFormValuesFromRawBody(body, headerValue(headers, "Content-Type"))
	if len(values) == 0 {
		return ""
	}
	return extractOpenAICompatPromptCacheKey(openAICompatRawMapFromFormValues(values), headers)
}

func openAICompatFormValuesFromRawBody(body []byte, contentType string) url.Values {
	if len(body) == 0 {
		return nil
	}
	mediaType, params, err := mime.ParseMediaType(strings.TrimSpace(contentType))
	if err != nil {
		return nil
	}
	switch strings.ToLower(mediaType) {
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(string(body))
		if err != nil {
			return nil
		}
		return values
	case "multipart/form-data":
		boundary := strings.TrimSpace(params["boundary"])
		if boundary == "" {
			return nil
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		values := url.Values{}
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil
			}
			name := strings.TrimSpace(part.FormName())
			if name == "" || strings.TrimSpace(part.FileName()) != "" {
				_ = part.Close()
				continue
			}
			data, readErr := io.ReadAll(io.LimitReader(part, 64*1024+1))
			_ = part.Close()
			if readErr != nil || len(data) > 64*1024 {
				continue
			}
			values.Add(name, string(data))
		}
		if len(values) == 0 {
			return nil
		}
		return values
	default:
		return nil
	}
}

func openAICompatRawMapFromFormValues(values url.Values) map[string]json.RawMessage {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(values))
	for key, vals := range values {
		key = strings.TrimSpace(key)
		if key == "" || len(vals) == 0 {
			continue
		}
		raw, ok := openAICompatRawMessageFromFormValues(vals)
		if ok {
			out[key] = raw
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func openAICompatRawMessageFromFormValues(values []string) (json.RawMessage, bool) {
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		cleaned = append(cleaned, strings.TrimSpace(value))
	}
	if len(cleaned) == 0 {
		return nil, false
	}
	if len(cleaned) == 1 {
		value := cleaned[0]
		if value == "" {
			return nil, false
		}
		if raw := openAICompatRawJSONValueFromString(value); len(raw) > 0 {
			return raw, true
		}
		raw, err := common.Marshal(value)
		return raw, err == nil
	}
	raw, err := common.Marshal(cleaned)
	return raw, err == nil
}

func openAICompatRawJSONValueFromString(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if !strings.HasPrefix(value, "{") && !strings.HasPrefix(value, "[") {
		return nil
	}
	var decoded any
	if err := common.Unmarshal([]byte(value), &decoded); err != nil {
		return nil
	}
	return json.RawMessage([]byte(value))
}

func objectFieldFromRawMap(data map[string]json.RawMessage, key string) map[string]json.RawMessage {
	if data == nil {
		return nil
	}
	raw, ok := data[key]
	if !ok || len(raw) == 0 {
		return nil
	}
	var result map[string]json.RawMessage
	if err := common.Unmarshal(raw, &result); err != nil {
		var encoded string
		if stringErr := common.Unmarshal(raw, &encoded); stringErr != nil {
			return nil
		}
		encoded = strings.TrimSpace(encoded)
		if encoded == "" || !strings.HasPrefix(encoded, "{") {
			return nil
		}
		if err := common.Unmarshal([]byte(encoded), &result); err != nil {
			return nil
		}
	}
	return result
}

func stringFieldFromRawMap(data map[string]json.RawMessage, key string) string {
	if data == nil {
		return ""
	}
	raw, ok := data[key]
	if !ok || len(raw) == 0 {
		return ""
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func boolishFieldFromRawMap(data map[string]json.RawMessage, key string) (bool, bool) {
	if data == nil {
		return false, false
	}
	raw, ok := data[key]
	if !ok || len(raw) == 0 {
		return false, false
	}
	var value interface{}
	if err := common.Unmarshal(raw, &value); err != nil {
		return false, false
	}
	return parseBoolishAny(value)
}

func parseBoolishAny(value interface{}) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		return parseBoolishValue(typed)
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case uint:
		return typed != 0, true
	case uint64:
		return typed != 0, true
	default:
		return false, false
	}
}

func headerValue(headers map[string]string, key string) string {
	if len(headers) == 0 || strings.TrimSpace(key) == "" {
		return ""
	}
	if value := strings.TrimSpace(headers[key]); value != "" {
		return value
	}
	lowerKey := strings.ToLower(key)
	for candidateKey, candidateValue := range headers {
		if strings.EqualFold(candidateKey, lowerKey) {
			return strings.TrimSpace(candidateValue)
		}
	}
	return ""
}
