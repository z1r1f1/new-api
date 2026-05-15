package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func buildChannelAffinityTemplateContextForTest(meta channelAffinityMeta) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, meta)
	return ctx
}

func TestApplyChannelAffinityOverrideTemplate_NoTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-no-template",
	})
	base := map[string]interface{}{
		"temperature": 0.7,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.False(t, applied)
	require.Equal(t, base, merged)
}

func TestApplyChannelAffinityOverrideTemplate_MergeTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-template",
		ParamTemplate: map[string]interface{}{
			"temperature": 0.2,
			"top_p":       0.95,
		},
		UsingGroup:     "default",
		ModelName:      "gpt-4.1",
		RequestPath:    "/v1/responses",
		KeySourceType:  "gjson",
		KeySourcePath:  "prompt_cache_key",
		KeyHint:        "abcd...wxyz",
		KeyFingerprint: "abcd1234",
	})
	base := map[string]interface{}{
		"temperature": 0.7,
		"max_tokens":  2000,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"])
	require.Equal(t, 0.95, merged["top_p"])
	require.Equal(t, 2000, merged["max_tokens"])
	require.Equal(t, 0.7, base["temperature"])

	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]interface{})
	require.True(t, ok)
	overrideInfoAny, ok := info["override_template"]
	require.True(t, ok)
	overrideInfo, ok := overrideInfoAny.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, overrideInfo["applied"])
	require.Equal(t, "rule-with-template", overrideInfo["rule_name"])
	require.EqualValues(t, 2, overrideInfo["param_override_keys"])
}

func TestApplyChannelAffinityOverrideTemplate_PreservesRequestPrefixInfo(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-template",
		ParamTemplate: map[string]interface{}{
			"temperature": 0.2,
		},
		RequestPrefix:     `{"prompt":"hello world"}`,
		RequestPrefixHash: "prefix-sha1",
		RequestPrefixLen:  24,
		RequestBodyLen:    64,
	})
	ctx.Set(ginKeyChannelAffinityLogInfo, map[string]interface{}{
		"channel_id": 901,
	})

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, map[string]interface{}{})
	require.True(t, applied)
	require.Equal(t, 0.2, merged["temperature"])

	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 901, info["channel_id"])
	require.Equal(t, "prefix-sha1", info["request_prefix_sha1"])
	require.Equal(t, 24, info["request_prefix_len"])
	require.Equal(t, 64, info["request_body_len"])
	require.Contains(t, info, "override_template")
}

func TestApplyChannelAffinityOverrideTemplate_MergeOperations(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-ops-template",
		ParamTemplate: map[string]interface{}{
			"operations": []map[string]interface{}{
				{
					"mode":  "pass_headers",
					"value": []string{"Originator"},
				},
			},
		},
	})
	base := map[string]interface{}{
		"temperature": 0.7,
		"operations": []map[string]interface{}{
			{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"])

	opsAny, ok := merged["operations"]
	require.True(t, ok)
	ops, ok := opsAny.([]interface{})
	require.True(t, ok)
	require.Len(t, ops, 2)

	firstOp, ok := ops[0].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "pass_headers", firstOp["mode"])

	secondOp, ok := ops[1].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "trim_prefix", secondOp["mode"])
}

func TestShouldSkipRetryAfterChannelAffinityFailure(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() *gin.Context
		want bool
	}{
		{
			name: "nil context",
			ctx: func() *gin.Context {
				return nil
			},
			want: false,
		},
		{
			name: "explicit skip retry flag in context",
			ctx: func() *gin.Context {
				ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-explicit-flag",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
				ctx.Set(ginKeyChannelAffinitySkipRetry, true)
				return ctx
			},
			want: true,
		},
		{
			name: "fallback to matched rule meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-skip-retry",
					SkipRetry:  true,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: true,
		},
		{
			name: "no flag and no skip retry meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-no-skip-retry",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ShouldSkipRetryAfterChannelAffinityFailure(tt.ctx()))
		})
	}
}

func TestChannelAffinityHitCodexTemplatePassHeadersEffective(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	var codexRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		if strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli trace") {
			codexRule = rule
			break
		}
	}
	require.NotNil(t, codexRule)

	affinityValue := fmt.Sprintf("pc-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(*codexRule, "gpt-5", "default", affinityValue)

	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"prompt_cache_key":"%s"}`, affinityValue)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 9527, channelID)

	baseOverride := map[string]interface{}{
		"temperature": 0.2,
	}
	mergedOverride, applied := ApplyChannelAffinityOverrideTemplate(ctx, baseOverride)
	require.True(t, applied)
	require.Equal(t, 0.2, mergedOverride["temperature"])

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
			"User-Agent": "codex-cli-test",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: mergedOverride,
			HeadersOverride: map[string]interface{}{
				"X-Static": "legacy-static",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-5"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)

	require.Equal(t, "legacy-static", info.RuntimeHeadersOverride["x-static"])
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	require.Equal(t, "codex-cli-test", info.RuntimeHeadersOverride["user-agent"])

	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	_, exists = info.RuntimeHeadersOverride["x-codex-turn-metadata"]
	require.False(t, exists)
}

func TestChannelAffinityRequestPrefixLoggingSetting(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	oldEnabled := setting.Enabled
	oldDefaultTTLSeconds := setting.DefaultTTLSeconds
	oldLogRequestPrefix := setting.LogRequestPrefix
	oldRequestPrefixChars := setting.RequestPrefixChars
	oldRules := append([]operation_setting.ChannelAffinityRule(nil), setting.Rules...)
	t.Cleanup(func() {
		setting.Enabled = oldEnabled
		setting.DefaultTTLSeconds = oldDefaultTTLSeconds
		setting.LogRequestPrefix = oldLogRequestPrefix
		setting.RequestPrefixChars = oldRequestPrefixChars
		setting.Rules = oldRules
	})

	setting.Enabled = true
	setting.DefaultTTLSeconds = 3600
	setting.RequestPrefixChars = 32
	setting.Rules = []operation_setting.ChannelAffinityRule{
		{
			Name:       "debug-prefix-test",
			ModelRegex: []string{"^gpt-5$"},
			KeySources: []operation_setting.ChannelAffinityKeySource{
				{Type: "gjson", Path: "prompt_cache_key"},
			},
		},
	}

	body := `{"prompt_cache_key":"cache-key-1","input":"line1 line2 line3","model":"gpt-5"}`
	expectedPrefix := string([]rune(body)[:32])

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	setting.LogRequestPrefix = false
	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.False(t, found)
	require.Equal(t, 0, channelID)
	MarkChannelAffinityUsed(ctx, "default", 901)
	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]interface{})
	require.True(t, ok)
	require.NotContains(t, info, "request_prefix")

	rec = httptest.NewRecorder()
	ctx, _ = gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	setting.LogRequestPrefix = true
	channelID, found = GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.False(t, found)
	require.Equal(t, 0, channelID)
	MarkChannelAffinityUsed(ctx, "default", 901)
	anyInfo, ok = ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok = anyInfo.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, affinityFingerprint(expectedPrefix), info["request_prefix_sha1"])
	require.Equal(t, len([]rune(expectedPrefix)), info["request_prefix_len"])
	require.Equal(t, len([]rune(body)), info["request_body_len"])
	debug, ok := info["request_debug"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, common.Sha1([]byte(body)), debug["body_sha1"])
	require.Equal(t, []string{"input", "model", "prompt_cache_key"}, debug["top_level_keys"])
	require.NotContains(t, info, "request_prefix")
}

func TestChannelAffinityInputDebugIncludesItemFingerprints(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`)

	debug := buildChannelAffinityJSONDebug(body)
	input, ok := debug["input"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 2, input["items_count"])

	itemFingerprints, ok := input["item_fingerprints"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, 2, itemFingerprints["total_count"])
	require.Equal(t, 2, itemFingerprints["covered_count"])

	items, ok := itemFingerprints["items"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, items, 2)
	require.Equal(t, 0, items[0]["index"])
	require.Equal(t, "message", items[0]["type"])
	require.Equal(t, "user", items[0]["role"])
	require.Equal(t, 1, items[1]["index"])
	require.Equal(t, "function_call_output", items[1]["type"])
	require.Contains(t, items[1], "sha1")
	require.NotContains(t, items[0], "text")
	require.NotContains(t, items[1], "output")
}

func TestChannelAffinityJSONDebugIncludesServiceTierValue(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","service_tier":"priority","input":"hello"}`)

	debug := buildChannelAffinityJSONDebug(body)

	require.Equal(t, true, debug["json_valid"])
	require.Equal(t, "priority", debug["service_tier"])
}

func TestAppendChannelAffinityResponseDebugIncludesNestedServiceTierValue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)
	oldLogRequestPrefix := setting.LogRequestPrefix
	t.Cleanup(func() {
		setting.LogRequestPrefix = oldLogRequestPrefix
	})
	setting.LogRequestPrefix = true

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Set(ginKeyChannelAffinityLogInfo, map[string]interface{}{
		"channel_id": 935,
		"model":      "gpt-5.5",
	})

	AppendChannelAffinityResponseDebug(ctx, []byte(`{"type":"response.completed","response":{"id":"resp_123","model":"gpt-5.5","service_tier":"priority"}}`))

	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]interface{})
	require.True(t, ok)
	debug, ok := info["response_debug"].(map[string]interface{})
	require.True(t, ok)
	response, ok := debug["response"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "priority", response["service_tier"])
}

func TestAppendChannelAffinityResponseDebugRecordsServiceTierWhenPrefixLoggingDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)
	oldLogRequestPrefix := setting.LogRequestPrefix
	t.Cleanup(func() {
		setting.LogRequestPrefix = oldLogRequestPrefix
	})
	setting.LogRequestPrefix = false

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)

	AppendChannelAffinityResponseDebug(ctx, []byte(`{"type":"response.completed","response":{"service_tier":"default"}}`))

	serviceTier, ok := ctx.Get(ginKeyUpstreamResponseServiceTier)
	require.True(t, ok)
	require.Equal(t, "default", serviceTier)
}

func TestChannelAffinityInputDebugCapsItemFingerprints(t *testing.T) {
	items := make([]string, 0, maxChannelAffinityInputItemFingerprints+10)
	for i := 0; i < maxChannelAffinityInputItemFingerprints+10; i++ {
		items = append(items, fmt.Sprintf(`{"type":"message","role":"user","content":[{"type":"input_text","text":"%d"}]}`, i))
	}
	body := []byte(`{"input":[` + strings.Join(items, ",") + `]}`)

	debug := buildChannelAffinityJSONDebug(body)
	input, ok := debug["input"].(map[string]interface{})
	require.True(t, ok)
	itemFingerprints, ok := input["item_fingerprints"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, maxChannelAffinityInputItemFingerprints+10, itemFingerprints["total_count"])
	require.Equal(t, maxChannelAffinityInputItemFingerprints, itemFingerprints["covered_count"])
	require.Equal(t, 10, itemFingerprints["omitted_middle_count"])

	fingerprints, ok := itemFingerprints["items"].([]map[string]interface{})
	require.True(t, ok)
	require.Len(t, fingerprints, maxChannelAffinityInputItemFingerprints)
	require.Equal(t, 0, fingerprints[0]["index"])
	require.Equal(t, maxChannelAffinityInputItemFingerprints+9, fingerprints[len(fingerprints)-1]["index"])
}

func TestChannelAffinityHeaderDebugHashesIndividualNonSensitiveHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{}`))
	ctx.Request.Header.Set("Authorization", "Bearer secret")
	ctx.Request.Header.Set("X-Codex-Turn-Metadata", "turn-a")
	ctx.Request.Header.Set("X-Client-Request-Id", "request-a")

	debug := buildChannelAffinityHeaderDebug(ctx)
	require.Equal(t, []string{"x-client-request-id", "x-codex-turn-metadata"}, debug["keys"])
	require.Equal(t, []string{"authorization"}, debug["redacted_keys"])
	sha1ByKey, ok := debug["sha1_by_key"].(map[string]string)
	require.True(t, ok)
	require.Equal(t, common.Sha1([]byte("turn-a")), sha1ByKey["x-codex-turn-metadata"])
	require.Equal(t, common.Sha1([]byte("request-a")), sha1ByKey["x-client-request-id"])
	require.NotContains(t, sha1ByKey, "authorization")
}

func TestChannelAffinityRequestPrefixDebugClampsLargeLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := strings.Repeat("a", maxChannelAffinityRequestPrefixChars+10)
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	ctx.Request.Header.Set("Content-Type", "application/json")

	prefix, prefixHash, prefixLen, bodyLen, debug := buildChannelAffinityRequestPrefixDebug(ctx, maxChannelAffinityRequestPrefixChars+1000)

	require.Len(t, []rune(prefix), maxChannelAffinityRequestPrefixChars)
	require.Equal(t, affinityFingerprint(prefix), prefixHash)
	require.Equal(t, maxChannelAffinityRequestPrefixChars, prefixLen)
	require.Equal(t, maxChannelAffinityRequestPrefixChars+10, bodyLen)
	require.Equal(t, common.Sha1([]byte(body)), debug["body_sha1"])
}

func TestClearCurrentChannelAffinityCache(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	cache := getChannelAffinityCache()
	cacheKey := channelAffinityCacheNamespace + ":test-clear-current-channel-affinity-cache"
	_, _ = cache.DeleteMany([]string{cacheKey})
	t.Cleanup(func() { _, _ = cache.DeleteMany([]string{cacheKey}) })

	setChannelAffinityContext(ctx, channelAffinityMeta{
		CacheKey:   cacheKey,
		TTLSeconds: 60,
	})
	require.NoError(t, cache.SetWithTTL(cacheKey, 9527, time.Minute))

	ClearCurrentChannelAffinityCache(ctx)

	_, found, err := cache.Get(cacheKey)
	require.NoError(t, err)
	require.False(t, found)
}
