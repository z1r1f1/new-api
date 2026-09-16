package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/billingexpr"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// attachQuotaSaturationToOther nests a quota saturation marker under
// other.admin_info.quota_saturation. Nesting under admin_info makes it
// admin-only for free, since model.formatUserLogs strips the whole admin_info
// object for non-admin viewers. Creates admin_info if absent. No-op when the
// clamp is nil (the common case: no saturation happened).
func attachQuotaSaturationToOther(other *model.LogOther, clamp *common.QuotaClamp) {
	if clamp == nil || other == nil {
		return
	}
	other.SetAdmin("quota_saturation", clamp.AuditMap())
}

// attachQuotaSaturation records the request's quota clamp (if any) onto the
// consume log's other.admin_info and emits a request-correlated backend audit
// line. Called right before RecordConsumeLog on the text/audio/wss paths.
func attachQuotaSaturation(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil {
		return
	}
	clamp := relayInfo.QuotaClamp
	if clamp == nil {
		return
	}
	attachQuotaSaturationToOther(other, clamp)
	logger.LogWarn(ctx, fmt.Sprintf("quota saturation on consume log: op=%s kind=%s original=%g clamped=%d user=%d model=%s",
		clamp.Op, clamp.Kind, clamp.Original, clamp.Clamped, relayInfo.UserId, relayInfo.GetBillingModelName()))
}

func appendRequestPath(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if other == nil {
		return
	}
	if ctx != nil && ctx.Request != nil && ctx.Request.URL != nil {
		if path := ctx.Request.URL.Path; path != "" {
			other.SetPublic("request_path", path)
			return
		}
	}
	if relayInfo != nil && relayInfo.RequestURLPath != "" {
		path := relayInfo.RequestURLPath
		if idx := strings.Index(path, "?"); idx != -1 {
			path = path[:idx]
		}
		other.SetPublic("request_path", path)
	}
}

// AppendRelayLogAdminInfo records relay routing and conversion diagnostics in
// the admin-only scope shared by successful and failed request logs.
func AppendRelayLogAdminInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if ctx == nil || other == nil {
		return
	}
	other.SetAdmin("use_channel", ctx.GetStringSlice("use_channel"))
	if relayInfo != nil {
		if billingModel := relayInfo.GetBillingModelName(); billingModel != "" && billingModel != relayInfo.OriginModelName {
			other.SetAdmin("billing_model", billingModel)
		}
		if diagnostics := relayInfo.ConversionDiagnostics(); len(diagnostics) > 0 {
			other.SetAdmin("conversion_diagnostics", diagnostics)
		}
		if relayInfo.ConversionDiagnosticsTruncated() {
			other.SetAdmin("conversion_diagnostics_truncated", true)
		}
	}
	if common.GetContextKeyBool(ctx, constant.ContextKeyChannelIsMultiKey) {
		other.SetAdmin("is_multi_key", true)
		other.SetAdmin("multi_key_index", common.GetContextKeyInt(ctx, constant.ContextKeyChannelMultiKeyIndex))
	}
	if common.GetContextKeyBool(ctx, constant.ContextKeyLocalCountTokens) {
		other.SetAdmin("local_count_tokens", true)
	}
	if headers := safeRequestHeadersFromContext(ctx); headers != nil {
		other.SetAdmin("request_headers", headers)
	}

	AppendChannelAffinityAdminInfo(ctx, other)
}

func GenerateTextOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64, modelPrice float64, userGroupRatio float64) *model.LogOther {
	other := model.NewLogOther()
	other.SetPublic("model_ratio", modelRatio)
	other.SetPublic("group_ratio", groupRatio)
	other.SetPublic("completion_ratio", completionRatio)
	other.SetPublic("cache_tokens", cacheTokens)
	other.SetPublic("cache_ratio", cacheRatio)
	other.SetPublic("model_price", modelPrice)
	other.SetPublic("user_group_ratio", userGroupRatio)
	other.SetPublic("frt", float64(relayInfo.FirstResponseTime.UnixMilli()-relayInfo.StartTime.UnixMilli()))
	if relayInfo.ReasoningEffort != "" {
		other.SetPublic("reasoning_effort", relayInfo.ReasoningEffort)
	}
	if relayInfo.IsModelMapped {
		other.SetPublic("is_model_mapped", true)
		other.SetPublic("upstream_model_name", relayInfo.UpstreamModelName)
	}

	isSystemPromptOverwritten := common.GetContextKeyBool(ctx, constant.ContextKeySystemPromptOverride)
	if isSystemPromptOverwritten {
		other.SetPublic("is_system_prompt_overwritten", true)
	}

	AppendRelayLogAdminInfo(ctx, relayInfo, other)
	adminInfo := logOtherAdminInfoSnapshot(other)
	AppendRequestProtocolInfo(ctx, other)
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

func logOtherAdminInfoSnapshot(other *model.LogOther) map[string]any {
	if other == nil {
		return nil
	}
	adminInfo, _ := other.Snapshot()["admin_info"].(map[string]any)
	return adminInfo
}

func AppendRequestProtocolInfo(ctx *gin.Context, other *model.LogOther) {
	if other == nil {
		return
	}
	other.SetPublic("request_protocol", requestProtocol(ctx))
}

func requestProtocol(ctx *gin.Context) string {
	if ctx == nil || ctx.Request == nil {
		return "http"
	}
	header := ctx.Request.Header
	if strings.EqualFold(strings.TrimSpace(header.Get("Upgrade")), "websocket") ||
		strings.TrimSpace(header.Get("Sec-WebSocket-Key")) != "" {
		return "websocket"
	}
	if strings.Contains(strings.ToLower(header.Get("Connection")), "upgrade") &&
		strings.EqualFold(strings.TrimSpace(header.Get("Upgrade")), "websocket") {
		return "websocket"
	}
	return "http"
}

func appendServiceTierInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, adminInfo map[string]any, other *model.LogOther) {
	if other == nil {
		return
	}
	if responseServiceTier := getContextStringValue(ctx, ginKeyUpstreamResponseServiceTier); responseServiceTier != "" {
		other.SetPublic("response_service_tier", responseServiceTier)
	} else if responseServiceTier := extractChannelAffinityResponseServiceTier(adminInfo); responseServiceTier != "" {
		other.SetPublic("response_service_tier", responseServiceTier)
	}
	if requestServiceTier := extractChannelAffinityRequestServiceTier(adminInfo); requestServiceTier != "" {
		other.SetPublic("request_service_tier", requestServiceTier)
	} else if requestServiceTier := extractRequestServiceTier(ctx, relayInfo); requestServiceTier != "" {
		other.SetPublic("request_service_tier", requestServiceTier)
	}
}

func appendRequestEffortInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, adminInfo map[string]any, other *model.LogOther) {
	if other == nil {
		return
	}
	if effort := extractChannelAffinityRequestEffort(adminInfo); effort != "" {
		other.SetPublic("request_effort", effort)
		return
	}
	if effort := extractRequestEffort(ctx, relayInfo); effort != "" {
		other.SetPublic("request_effort", effort)
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

func extractChannelAffinityResponseServiceTier(adminInfo map[string]any) string {
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

func extractChannelAffinityRequestServiceTier(adminInfo map[string]any) string {
	channelAffinity := getMapValue(adminInfo, "channel_affinity")
	if value := getStringValue(getMapValue(channelAffinity, "final_request_debug"), "service_tier"); value != "" {
		return value
	}
	return getStringValue(getMapValue(channelAffinity, "request_debug"), "service_tier")
}

func extractChannelAffinityRequestEffort(adminInfo map[string]any) string {
	channelAffinity := getMapValue(adminInfo, "channel_affinity")
	for _, debugKey := range []string{"final_request_debug", "request_debug"} {
		debug := getMapValue(channelAffinity, debugKey)
		if effort := extractEffortFromMap(debug); effort != "" {
			return effort
		}
	}
	return ""
}

func getMapValue(source map[string]any, key string) map[string]any {
	if source == nil {
		return nil
	}
	value, ok := source[key]
	if !ok {
		return nil
	}
	result, _ := value.(map[string]any)
	return result
}

func getStringValue(source map[string]any, key string) string {
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
	body := requestBodyForLogParamExtraction(ctx, relayInfo)
	if len(body) == 0 {
		return ""
	}
	var data map[string]json.RawMessage
	if err := common.Unmarshal(body, &data); err != nil {
		return ""
	}
	headers := map[string]string(nil)
	if relayInfo != nil {
		headers = relayInfo.RequestHeaders
	}
	if serviceTier := extractOpenAICompatServiceTier(data, headers); serviceTier != "" {
		return serviceTier
	}
	return stringFieldFromRawMap(data, "service_tier")
}

func extractRequestEffort(ctx *gin.Context, relayInfo *relaycommon.RelayInfo) string {
	body := requestBodyForLogParamExtraction(ctx, relayInfo)
	if len(body) == 0 {
		return ""
	}
	var data map[string]any
	if err := common.Unmarshal(body, &data); err != nil {
		return ""
	}
	return extractEffortFromMap(data)
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

func extractEffortFromMap(data map[string]any) string {
	if data == nil {
		return ""
	}
	if reasoning := getMapValue(data, "reasoning"); reasoning != nil {
		if effort := getStringValue(reasoning, "effort"); effort != "" {
			return effort
		}
	}
	if thinking := getMapValue(data, "thinking"); thinking != nil {
		if effort := getStringValue(thinking, "effort"); effort != "" {
			return effort
		}
	}
	if outputConfig := getMapValue(data, "output_config"); outputConfig != nil {
		for _, key := range []string{"effort", "think_effort", "reasoning_effort", "model_reasoning_effort"} {
			if effort := getStringValue(outputConfig, key); effort != "" {
				return effort
			}
		}
	}
	for _, key := range []string{"effort", "think_effort", "model_reasoning_effort", "reasoning_effort"} {
		if effort := getStringValue(data, key); effort != "" {
			return effort
		}
	}
	if metadata := getMapValue(data, "metadata"); metadata != nil {
		for _, key := range []string{"effort", "think_effort", "model_reasoning_effort", "reasoning_effort", "effortLevel"} {
			if effort := getStringValue(metadata, key); effort != "" {
				return effort
			}
		}
	}
	return ""
}

func appendFastServiceTierInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if other == nil {
		return
	}
	other.SetPublic("fast_service_tier", hasFastServiceTier(ctx, relayInfo))
	requestServiceTier, _ := other.Snapshot()["request_service_tier"].(string)
	requestServiceTier = strings.TrimSpace(requestServiceTier)
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
	other.SetPublic("request_fast", fast)
	if tier := extractRequestFastServiceTier(ctx, relayInfo, requestServiceTier, fast); tier != "" {
		other.SetPublic("request_fast_service_tier", tier)
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
	var data map[string]any
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
			return parseLogBoolishValue(value)
		}
	}

	body := requestBodyForLogParamExtraction(ctx, relayInfo)
	if len(body) == 0 {
		return false, false
	}
	var data map[string]any
	if err := common.Unmarshal(body, &data); err != nil {
		return false, false
	}
	if fast, ok := getBoolishValue(data, "fast"); ok {
		return fast, true
	}
	if fast, ok := getBoolishValue(data, "fastMode"); ok {
		return fast, true
	}
	metadata := getMapValue(data, "metadata")
	if fast, ok := getBoolishValue(metadata, "fast"); ok {
		return fast, true
	}
	return getBoolishValue(metadata, "fastMode")
}

func extractRequestFastServiceTier(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, requestServiceTier string, requestFast bool) string {
	if !requestFast {
		return ""
	}
	if ctx != nil && ctx.Request != nil {
		if value := strings.TrimSpace(ctx.Request.Header.Get(headerClaudeCodeProxyFastServiceTier)); value != "" {
			return value
		}
	}
	if requestServiceTier != "" {
		return requestServiceTier
	}
	return extractRequestServiceTier(ctx, relayInfo)
}

func getBoolishValue(source map[string]any, key string) (bool, bool) {
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
		return parseLogBoolishValue(typed)
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

func parseLogBoolishValue(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled", "enable", "fast":
		return true, true
	case "0", "false", "no", "off", "disabled", "disable":
		return false, true
	default:
		return false, false
	}
}

func appendParamOverrideInfo(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil || other == nil || len(relayInfo.ParamOverrideAudit) == 0 {
		return
	}
	other.SetPublic("po", relayInfo.ParamOverrideAudit)
}

func appendStreamStatus(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil || other == nil || !relayInfo.IsStream || relayInfo.StreamStatus == nil {
		return
	}
	ss := relayInfo.StreamStatus
	status := "ok"
	if !ss.IsNormalEnd() || ss.HasErrors() {
		status = "error"
	}
	streamInfo := map[string]any{
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
	other.SetPublic("stream_status", streamInfo)
}

func appendBillingInfo(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil || other == nil {
		return
	}
	// billing_source: "wallet" or "subscription"
	if relayInfo.BillingSource != "" {
		other.SetPublic("billing_source", relayInfo.BillingSource)
	}
	if relayInfo.UserSetting.BillingPreference != "" {
		other.SetPublic("billing_preference", relayInfo.UserSetting.BillingPreference)
	}
	if relayInfo.BillingSource == "subscription" {
		if relayInfo.SubscriptionId != 0 {
			other.SetPublic("subscription_id", relayInfo.SubscriptionId)
		}
		if relayInfo.SubscriptionPreConsumed > 0 {
			other.SetPublic("subscription_pre_consumed", relayInfo.SubscriptionPreConsumed)
		}
		// post_delta: settlement delta applied after actual usage is known (can be negative for refund)
		if relayInfo.SubscriptionPostDelta != 0 {
			other.SetPublic("subscription_post_delta", relayInfo.SubscriptionPostDelta)
		}
		if relayInfo.SubscriptionPlanId != 0 {
			other.SetPublic("subscription_plan_id", relayInfo.SubscriptionPlanId)
		}
		if relayInfo.SubscriptionPlanTitle != "" {
			other.SetPublic("subscription_plan_title", relayInfo.SubscriptionPlanTitle)
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
			remain := max(relayInfo.SubscriptionAmountTotal-usedFinal, 0)
			other.SetPublic("subscription_total", relayInfo.SubscriptionAmountTotal)
			other.SetPublic("subscription_used", usedFinal)
			other.SetPublic("subscription_remain", remain)
		}
		if consumed > 0 {
			other.SetPublic("subscription_consumed", consumed)
		}
		// Wallet quota is not deducted when billed from subscription.
		other.SetPublic("wallet_quota_deducted", 0)
	}
}

func appendRequestConversionChain(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
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
	other.SetPublic("request_conversion", chain)
}

func appendFinalRequestFormat(relayInfo *relaycommon.RelayInfo, other *model.LogOther) {
	if relayInfo == nil || other == nil {
		return
	}
	if relayInfo.GetFinalRequestRelayFormat() == types.RelayFormatClaude {
		// claude indicates the final upstream request format is Claude Messages.
		// Frontend log rendering uses this to keep the original Claude input display.
		other.SetPublic("claude", true)
	}
}

func GenerateWssOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.RealtimeUsage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) *model.LogOther {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info.SetPublic("ws", true)
	info.SetPublic("audio_input", usage.InputTokenDetails.AudioTokens)
	info.SetPublic("audio_output", usage.OutputTokenDetails.AudioTokens)
	info.SetPublic("text_input", usage.InputTokenDetails.TextTokens)
	info.SetPublic("text_output", usage.OutputTokenDetails.TextTokens)
	info.SetPublic("audio_ratio", audioRatio)
	info.SetPublic("audio_completion_ratio", audioCompletionRatio)
	return info
}

func GenerateAudioOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, usage *dto.Usage, modelRatio, groupRatio, completionRatio, audioRatio, audioCompletionRatio, modelPrice, userGroupRatio float64) *model.LogOther {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, 0, 0.0, modelPrice, userGroupRatio)
	info.SetPublic("audio", true)
	info.SetPublic("audio_input", usage.PromptTokensDetails.AudioTokens)
	info.SetPublic("audio_output", usage.CompletionTokenDetails.AudioTokens)
	info.SetPublic("text_input", usage.PromptTokensDetails.TextTokens)
	info.SetPublic("text_output", usage.CompletionTokenDetails.TextTokens)
	info.SetPublic("audio_ratio", audioRatio)
	info.SetPublic("audio_completion_ratio", audioCompletionRatio)
	return info
}

func GenerateClaudeOtherInfo(ctx *gin.Context, relayInfo *relaycommon.RelayInfo, modelRatio, groupRatio, completionRatio float64,
	cacheTokens int, cacheRatio float64,
	cacheCreationTokens int, cacheCreationRatio float64,
	cacheCreationTokens5m int, cacheCreationRatio5m float64,
	cacheCreationTokens1h int, cacheCreationRatio1h float64,
	modelPrice float64, userGroupRatio float64) *model.LogOther {
	info := GenerateTextOtherInfo(ctx, relayInfo, modelRatio, groupRatio, completionRatio, cacheTokens, cacheRatio, modelPrice, userGroupRatio)
	info.SetPublic("claude", true)
	info.SetPublic("cache_creation_tokens", cacheCreationTokens)
	info.SetPublic("cache_creation_ratio", cacheCreationRatio)
	if cacheCreationTokens5m != 0 {
		info.SetPublic("cache_creation_tokens_5m", cacheCreationTokens5m)
		info.SetPublic("cache_creation_ratio_5m", cacheCreationRatio5m)
	}
	if cacheCreationTokens1h != 0 {
		info.SetPublic("cache_creation_tokens_1h", cacheCreationTokens1h)
		info.SetPublic("cache_creation_ratio_1h", cacheCreationRatio1h)
	}
	return info
}

func GenerateMjOtherInfo(relayInfo *relaycommon.RelayInfo, priceData hosttypes.PriceData) *model.LogOther {
	other := model.NewLogOther()
	other.SetPublic("model_price", priceData.ModelPrice)
	other.SetPublic("group_ratio", priceData.GroupRatioInfo.GroupRatio)
	if priceData.GroupRatioInfo.HasSpecialRatio {
		other.SetPublic("user_group_ratio", priceData.GroupRatioInfo.GroupSpecialRatio)
	}
	appendRequestPath(nil, relayInfo, other)
	return other
}

// InjectTieredBillingInfo overlays tiered billing fields onto an existing
// module-specific other map. Call this after GenerateTextOtherInfo /
// GenerateClaudeOtherInfo / etc. when the request used tiered_expr billing.
func InjectTieredBillingInfo(other *model.LogOther, relayInfo *relaycommon.RelayInfo, result *billingexpr.TieredResult) {
	if relayInfo == nil || other == nil {
		return
	}
	snap := relayInfo.TieredBillingSnapshot
	if snap == nil {
		return
	}
	other.SetPublic("billing_mode", "tiered_expr")
	other.SetPublic("expr_b64", base64.StdEncoding.EncodeToString([]byte(snap.ExprString)))
	if result != nil {
		if tokens := result.BillingTokens; tokens != nil && result.BillingUnit == billingexpr.BillingUnitToken {
			other.SetPublic("image_cache_tokens", tokens.ImgCR)
			other.SetPublic("billing_tokens", map[string]float64{
				"p": tokens.P, "c": tokens.C, "len": tokens.Len,
				"cr": tokens.CR, "cc": tokens.CC, "cc1h": tokens.CC1h,
				"img": tokens.Img, "img_cr": tokens.ImgCR, "img_o": tokens.ImgO,
				"ai": tokens.AI, "ao": tokens.AO,
			})
		}
		if result.ImageCount != nil {
			other.SetPublic("image_count", *result.ImageCount)
		}
		other.SetPublic("matched_tier", result.MatchedTier)
		if result.BillingUnit != "" {
			other.SetPublic("billing_unit", result.BillingUnit)
		}
		if result.FixedPrice != nil {
			other.SetPublic("fixed_price", *result.FixedPrice)
		}
		if len(result.RequestRules) > 0 {
			other.SetPublic("request_rules", result.RequestRules)
		}
	} else if snap.EstimatedBillingUnit != "" {
		if snap.EstimatedImageCount != nil {
			other.SetPublic("image_count", *snap.EstimatedImageCount)
		}
		other.SetPublic("matched_tier", snap.EstimatedTier)
		other.SetPublic("billing_unit", snap.EstimatedBillingUnit)
		if snap.EstimatedFixedPrice != nil {
			other.SetPublic("fixed_price", *snap.EstimatedFixedPrice)
		}
	}
}
