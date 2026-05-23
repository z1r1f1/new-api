package service

import (
	"encoding/base64"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

const (
	headerClaudeCodeProxyFast            = "X-Claude-Code-Proxy-Fast"
	headerClaudeCodeProxyFastServiceTier = "X-Claude-Code-Proxy-Fast-Service-Tier"
)

func appendRequestPath(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if other == nil {
		return
	}
	if ctx != nil && ctx.Request != nil && ctx.Request.URL != nil {
		if path := ctx.Request.URL.Path; path != "" {
			other["request_path"] = path
			return
		}
	}
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		path := relayInfo.RequestURLPath
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		other["request_path"] = path
	}
}

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_ratio"] = modelRatio
	other["group_ratio"] = groupRatio
	other["completion_ratio"] = completionRatio
	other["cache_tokens"] = cacheTokens
	other["cache_ratio"] = cacheRatio
	other["model_price"] = modelPrice
	other["user_group_ratio"] = userGroupRatio
	other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
	if relayInfo.ReasoningEffort != "" {
		other["reasoning_effort"] = relayInfo.ReasoningEffort
	}
	if relayInfo.IsModelMapped {
		other["is_model_mapped"] = true
		other["upstream_model_name"] = relayInfo.UpstreamModelName
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other["is_system_prompt_overwritten"] = true
	}

	adminInfo := make(map[string]interface{})
	adminInfo["use_channel"] = ctx.GetStringSlice("use_channel")
	isMultiKey := common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey)
	if isMultiKey {
		adminInfo["is_multi_key"] = true
		adminInfo["multi_key_index"] = common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex)
	}

	isLocalCountTokens := common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens)
	if isLocalCountTokens {
		adminInfo["local_count_tokens"] = isLocalCountTokens
	}

	AppendChannelAffinityAdminInfo(ctx, adminInfo)

	other["admin_info"] = adminInfo
	appendServiceTierInfo(ctx, relayInfo, adminInfo, other)
	appendRequestEffortInfo(ctx, relayInfo, adminInfo, other)
	appendRequestPath(ctx, relayInfo, other)
	appendRequestConversionChain(relayInfo, other)
	appendFinalRequestFormat(relayInfo, other)
	appendBillingInfo(relayInfo, other)
	appendParamOverrideInfo(relayInfo, other)
	appendFastServiceTierInfo(ctx, relayInfo, other)
	appendStreamStatus(relayInfo, other)
	return other
}

func appendServiceTierInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, adminInfo map[string]interface{}, other map[string]interface{}) {
	if other == nil {
		return
	}
	if responseServiceTier := getContextStringValue(ctx, ginKeyUpstreamResponseServiceTier); responseServiceTier != "" {
		other["response_service_tier"] = responseServiceTier
	} else if responseServiceTier := extractChannelAffinityResponseServiceTier(adminInfo); responseServiceTier != "" {
		other["response_service_tier"] = responseServiceTier
	}
	if requestServiceTier := extractChannelAffinityRequestServiceTier(adminInfo); requestServiceTier != "" {
		other["request_service_tier"] = requestServiceTier
	} else if requestServiceTier := extractRequestServiceTier(ctx, relayInfo); requestServiceTier != "" {
		other["request_service_tier"] = requestServiceTier
	}
}

func appendRequestEffortInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, adminInfo map[string]interface{}, other map[string]interface{}) {
	if other == nil {
		return
	}
	if effort := extractChannelAffinityRequestEffort(adminInfo); effort != "" {
		other["request_effort"] = effort
		return
	}
	if effort := extractRequestEffort(ctx, relayInfo); effort != "" {
		other["request_effort"] = effort
	}
}

func getContextStringValue(ctx *gin.Context, key string) string {
	if ctx == nil {
		return ""
	}
	value, ok := ctx.Get(key)
	if !ok {
		return ""
	}
	str, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(str)
}

func extractChannelAffinityResponseServiceTier(adminInfo map[string]interface{}) string {
	channelAffinity := getMapValue(adminInfo, "channel_affinity")
	responseDebug := getMapValue(channelAffinity, "response_debug")
	if responseDebug == nil {
		return ""
	}
	if value := getStringValue(responseDebug, "service_tier"); value != "" {
		return value
	}
	response := getMapValue(responseDebug, "response")
	return getStringValue(response, "service_tier")
}

func extractChannelAffinityRequestServiceTier(adminInfo map[string]interface{}) string {
	channelAffinity := getMapValue(adminInfo, "channel_affinity")
	if value := getStringValue(getMapValue(channelAffinity, "final_request_debug"), "service_tier"); value != "" {
		return value
	}
	return getStringValue(getMapValue(channelAffinity, "request_debug"), "service_tier")
}

func extractChannelAffinityRequestEffort(adminInfo map[string]interface{}) string {
	channelAffinity := getMapValue(adminInfo, "channel_affinity")
	for _, debugKey := range []string{"final_request_debug", "request_debug"} {
		debug := getMapValue(channelAffinity, debugKey)
		if effort := extractEffortFromMap(debug); effort != "" {
			return effort
		}
	}
	return ""
}

func getMapValue(source map[string]interface{}, key string) map[string]interface{} {
	if source == nil {
		return nil
	}
	value, ok := source[key]
	if !ok {
		return nil
	}
	result, _ := value.(map[string]interface{})
	return result
}

func getStringValue(source map[string]interface{}, key string) string {
	if source == nil {
		return ""
	}
	value, ok := source[key].(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func extractRequestServiceTier(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) string {
	return extractStringParamFromRequestBody(ctx, relayInfo, "service_tier")
}

func extractRequestEffort(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) string {
	body := requestBodyForLogParamExtraction(ctx, relayInfo)
	if len(body) == 0 {
		return ""
	}
	var data map[string]interface{}
	if err := common.Unmarshal(body, &data); err != nil {
		return ""
	}
	return extractEffortFromMap(data)
}

func extractStringParamFromRequestBody(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, key string) string {
	body := requestBodyForLogParamExtraction(ctx, relayInfo)
	if len(body) == 0 {
		return ""
	}
	var data map[string]interface{}
	if err := common.Unmarshal(body, &data); err != nil {
		return ""
	}
	return getStringValue(data, key)
}

func requestBodyForLogParamExtraction(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) []byte {
	if relayInfo != nil && relayInfo.BillingRequestInput != nil && len(relayInfo.BillingRequestInput.Body) > 0 {
		return relayInfo.BillingRequestInput.Body
	}
	if ctx == nil || ctx.Request == nil {
		return nil
	}
	contentType := strings.ToLower(strings.TrimSpace(ctx.Request.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "application/json") {
		return nil
	}
	storage, err := common.GetBodyStorage(ctx)
	if err != nil {
		return nil
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil
	}
	return body
}

func extractEffortFromMap(data map[string]interface{}) string {
	if data == nil {
		return ""
	}
	if reasoning := getMapValue(data, "reasoning"); reasoning != nil {
		if effort := getStringValue(reasoning, "effort"); effort != "" {
			return effort
		}
	}
	for _, key := range []string{"effort", "think_effort", "model_reasoning_effort", "reasoning_effort"} {
		if effort := getStringValue(data, key); effort != "" {
			return effort
		}
	}
	return ""
}

func appendFastServiceTierInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if other == nil {
		return
	}
	other["fast_service_tier"] = hasFastServiceTier(ctx, relayInfo)
	requestServiceTier := ""
	if value, ok := other["request_service_tier"].(string); ok {
		requestServiceTier = strings.TrimSpace(value)
	}
	if requestServiceTier == "" {
		requestServiceTier = extractRequestServiceTier(ctx, relayInfo)
	}
	fast, ok := extractRequestFastParam(ctx, relayInfo)
	if !ok {
		fast = false
	}
	if !fast && isFastRequestServiceTier(requestServiceTier) {
		fast = true
	}
	other["request_fast"] = fast
	if tier := extractRequestFastServiceTier(ctx, relayInfo, other, fast); tier != "" {
		other["request_fast_service_tier"] = tier
	}
}

func isFastRequestServiceTier(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "fast", "priority":
		return true
	default:
		return false
	}
}

func hasFastServiceTier(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) bool {
	if relayInfo != nil && relayInfo.BillingRequestInput != nil && len(relayInfo.BillingRequestInput.Body) > 0 {
		return hasFastServiceTierInBody(relayInfo.BillingRequestInput.Body)
	}
	if ctx == nil || ctx.Request == nil {
		return false
	}
	contentType := strings.ToLower(strings.TrimSpace(ctx.Request.Header.Get("Content-Type")))
	if !strings.HasPrefix(contentType, "application/json") {
		return false
	}
	storage, err := common.GetBodyStorage(ctx)
	if err != nil {
		return false
	}
	body, err := storage.Bytes()
	if err != nil {
		return false
	}
	return hasFastServiceTierInBody(body)
}

func hasFastServiceTierInBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var data map[string]interface{}
	if err := common.Unmarshal(body, &data); err != nil {
		return false
	}
	value, ok := data["service_tier"].(string)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "fast")
}

func extractRequestFastParam(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) (bool, bool) {
	if ctx != nil && ctx.Request != nil {
		if value := strings.TrimSpace(ctx.Request.Header.Get(headerClaudeCodeProxyFast)); value != "" {
			return parseBoolishValue(value)
		}
	}

	body := requestBodyForLogParamExtraction(ctx, relayInfo)
	if len(body) == 0 {
		return false, false
	}
	var data map[string]interface{}
	if err := common.Unmarshal(body, &data); err != nil {
		return false, false
	}
	return getBoolishValue(data, "fast")
}

func extractRequestFastServiceTier(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other map[string]interface{}, requestFast bool) string {
	if !requestFast {
		return ""
	}
	if ctx != nil && ctx.Request != nil {
		if value := strings.TrimSpace(ctx.Request.Header.Get(headerClaudeCodeProxyFastServiceTier)); value != "" {
			return value
		}
	}
	if other != nil {
		if value, ok := other["request_service_tier"].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return extractRequestServiceTier(ctx, relayInfo)
}

func getBoolishValue(source map[string]interface{}, key string) (bool, bool) {
	if source == nil {
		return false, false
	}
	value, ok := source[key]
	if !ok {
		return false, false
	}
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

func parseBoolishValue(value string) (bool, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "1", "true", "yes", "on", "enabled", "enable", "fast":
		return true, true
	case "0", "false", "no", "off", "disabled", "disable":
		return false, true
	default:
		return false, false
	}
}

func appendParamOverrideInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || len(relayInfo.ParamOverrideAudit) == 0 {
		return
	}
	other["po"] = relayInfo.ParamOverrideAudit
}

func appendStreamStatus(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil || !relayInfo.IsStream || relayInfo.StreamStatus == nil {
		return
	}
	ss := relayInfo.StreamStatus
	status := "ok"
	if !ss.IsNormalEnd() || ss.HasErrors() {
		status = "error"
	}
	streamInfo := map[string]interface{}{
		"status":     status,
		"end_reason": string(ss.EndReason),
	}
	if ss.EndError != nil {
		streamInfo["end_error"] = ss.EndError.Error()
	}
	if ss.ErrorCount > 0 {
		streamInfo["error_count"] = ss.ErrorCount
		messages := make([]string, 0, len(ss.Errors))
		for _, e := range ss.Errors {
			messages = append(messages, e.Message)
		}
		streamInfo["errors"] = messages
	}
	other["stream_status"] = streamInfo
}

func appendBillingInfo(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	// billing_source: "wallet" or "subscription"
	if relayInfo.BillingSource != "" {
		other["billing_source"] = relayInfo.BillingSource
	}
	if relayInfo.UserSetting.BillingPreference != "" {
		other["billing_preference"] = relayInfo.UserSetting.BillingPreference
	}
	if relayInfo.BillingSource == "subscription" {
		if relayInfo.SubscriptionId != 0 {
			other["subscription_id"] = relayInfo.SubscriptionId
		}
		if relayInfo.SubscriptionPreConsumed > 0 {
			other["subscription_pre_consumed"] = relayInfo.SubscriptionPreConsumed
		}
		// post_delta: settlement delta applied after actual usage is known (can be negative for refund)
		if relayInfo.SubscriptionPostDelta != 0 {
			other["subscription_post_delta"] = relayInfo.SubscriptionPostDelta
		}
		if relayInfo.SubscriptionPlanId != 0 {
			other["subscription_plan_id"] = relayInfo.SubscriptionPlanId
		}
		if relayInfo.SubscriptionPlanTitle != "" {
			other["subscription_plan_title"] = relayInfo.SubscriptionPlanTitle
		}
		// Compute "this request" subscription consumed + remaining
		consumed := relayInfo.SubscriptionPreConsumed + relayInfo.SubscriptionPostDelta
		usedFinal := relayInfo.SubscriptionAmountUsedAfterPreConsume + relayInfo.SubscriptionPostDelta
		if consumed < 0 {
			consumed = 0
		}
		if usedFinal < 0 {
			usedFinal = 0
		}
		if relayInfo.SubscriptionAmountTotal > 0 {
			remain := relayInfo.SubscriptionAmountTotal - usedFinal
			if remain < 0 {
				remain = 0
			}
			other["subscription_total"] = relayInfo.SubscriptionAmountTotal
			other["subscription_used"] = usedFinal
			other["subscription_remain"] = remain
		}
		if consumed > 0 {
			other["subscription_consumed"] = consumed
		}
		// Wallet quota is not deducted when billed from subscription.
		other["wallet_quota_deducted"] = 0
	}
}

func appendRequestConversionChain(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if len(relayInfo.RequestConversionChain) == 0 {
		return
	}
	chain := make([]string, 0, len(relayInfo.RequestConversionChain))
	for _, f := range relayInfo.RequestConversionChain {
		switch f {
		case types.RelayFormatOpenAI:
			chain = append(chain, "OpenAI Compatible")
		case types.RelayFormatClaude:
			chain = append(chain, "Claude Messages")
		case types.RelayFormatGemini:
			chain = append(chain, "Google Gemini")
		case types.RelayFormatOpenAIResponses:
			chain = append(chain, "OpenAI Responses")
		default:
			chain = append(chain, string(f))
		}
	}
	if len(chain) == 0 {
		return
	}
	other["request_conversion"] = chain
}

func appendFinalRequestFormat(relayInfo *relaycommon.RelayInfo, other map[string]interface{}) {
	if relayInfo == nil || other == nil {
		return
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		// claude indicates the final upstream request format is Claude Messages.
		// Frontend log rendering uses this to keep the original Claude input display.
		other["claude"] = true
	}
}

func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["ws"] = true
	info["audio_input"] = usage.InputTokenDetails.AudioTokens
	info["audio_output"] = usage.OutputTokenDetails.AudioTokens
	info["text_input"] = usage.InputTokenDetails.TextTokens
	info["text_output"] = usage.OutputTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info["audio"] = true
	info["audio_input"] = usage.PromptTokensDetails.AudioTokens
	info["audio_output"] = usage.CompletionTokenDetails.AudioTokens
	info["text_input"] = usage.PromptTokensDetails.TextTokens
	info["text_output"] = usage.CompletionTokenDetails.TextTokens
	info["audio_ratio"] = audioRatio
	info["audio_completion_ratio"] = audioCompletionRatio
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64) map[string]interface{} {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio)
	info["claude"] = true
	info["cache_creation_tokens"] = cacheCreationTokens
	info["cache_creation_ratio"] = cacheCreationRatio
	if cacheCreationTokens5m != 0 {
		info["cache_creation_tokens_5m"] = cacheCreationTokens5m
		info["cache_creation_ratio_5m"] = cacheCreationRatio5m
	}
	if cacheCreationTokens1h != 0 {
		info["cache_creation_tokens_1h"] = cacheCreationTokens1h
		info["cache_creation_ratio_1h"] = cacheCreationRatio1h
	}
	return info
}

func GenerateMjOtherInfo(relayInfo *relaycommon.RelayInfo, priceData types.PriceData) map[string]interface{} {
	other := make(map[string]interface{})
	other["model_price"] = priceData.ModelPrice
	other["group_ratio"] = priceData.GroupRatioInfo.GroupRatio
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other["user_group_ratio"] = priceData.GroupRatioInfo.GroupSpecialRatio
	}
	appendRequestPath(nil, relayInfo, other)
	return other
}

// InjectTieredBillingInfo overlays tiered billing fields onto an existing
// module-specific other map. Call this after GenerateTextOtherInfo /
// GenerateClaudeOtherInfo / etc. when the request used tiered_expr billing.
func InjectTieredBillingInfo(other map[string]interface{}, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) {
	if relayInfo == nil || other == nil {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil {
		return
	}
	other["billing_mode"] = "tiered_expr"
	other["expr_b64"] = base64.StdEncoding.EncodeToString([]byte(snap.ExprString))
	if result != nil {
		other["matched_tier"] = result.MatchedTier
	}
}
