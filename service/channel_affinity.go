package service

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/pkg/cachex"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
	"github.com/samber/hot"
	"github.com/tidwall/gjson"
)

const (
	ginKeyChannelAffinityCacheKey     = "channel_affinity_cache_key"
	ginKeyChannelAffinityTTLSeconds   = "channel_affinity_ttl_seconds"
	ginKeyChannelAffinityMeta         = "channel_affinity_meta"
	ginKeyChannelAffinityLogInfo      = "channel_affinity_log_info"
	ginKeyChannelAffinitySkipRetry    = "channel_affinity_skip_retry_on_failure"
	ginKeyUpstreamResponseServiceTier = "upstream_response_service_tier"

	channelAffinityCacheNamespace           = "new-api:channel_affinity:v1"
	channelAffinityUsageCacheStatsNamespace = "new-api:channel_affinity_usage_cache_stats:v1"
	maxChannelAffinityRequestPrefixChars    = 65536
	maxChannelAffinityInputItemFingerprints = 32
)

var (
	channelAffinityCacheOnce sync.Once
	channelAffinityCache     *cachex.HybridCache[int]

	channelAffinityUsageCacheStatsOnce  sync.Once
	channelAffinityUsageCacheStatsCache *cachex.HybridCache[ChannelAffinityUsageCacheCounters]

	channelAffinityRegexCache sync.Map // map[string]*regexp.Regexp
)

type channelAffinityMeta struct {
	CacheKey          string
	TTLSeconds        int
	RuleName          string
	SkipRetry         bool
	ParamTemplate     map[string]interface{}
	KeySourceType     string
	KeySourceKey      string
	KeySourcePath     string
	KeyHint           string
	KeyFingerprint    string
	UsingGroup        string
	ModelName         string
	RequestPath       string
	RequestPrefix     string
	RequestPrefixHash string
	RequestPrefixLen  int
	RequestBodyLen    int
	RequestDebug      map[string]interface{}
}

type ChannelAffinityStatsContext struct {
	RuleName       string
	UsingGroup     string
	KeyFingerprint string
	TTLSeconds     int64
}

const (
	cacheTokenRateModeCachedOverPrompt           = "cached_over_prompt"
	cacheTokenRateModeCachedOverPromptPlusCached = "cached_over_prompt_plus_cached"
	cacheTokenRateModeMixed                      = "mixed"
)

type ChannelAffinityCacheStats struct {
	Enabled       bool           `json:"enabled"`
	Total         int            `json:"total"`
	Unknown       int            `json:"unknown"`
	ByRuleName    map[string]int `json:"by_rule_name"`
	CacheCapacity int            `json:"cache_capacity"`
	CacheAlgo     string         `json:"cache_algo"`
}

func getChannelAffinityCache() *cachex.HybridCache[int] {
	channelAffinityCacheOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := setting.MaxEntries
		if capacity <= 0 {
			capacity = 100_000
		}
		defaultTTLSeconds := setting.DefaultTTLSeconds
		if defaultTTLSeconds <= 0 {
			defaultTTLSeconds = 3600
		}

		channelAffinityCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
			Namespace: cachex.Namespace(channelAffinityCacheNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.IntCodec{},
			Memory: func() *hot.HotCache[string, int] {
				return hot.NewHotCache[string, int](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityCache
}

func GetChannelAffinityCacheStats() ChannelAffinityCacheStats {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return ChannelAffinityCacheStats{
			Enabled:    false,
			Total:      0,
			Unknown:    0,
			ByRuleName: map[string]int{},
		}
	}

	cache := getChannelAffinityCache()
	mainCap, _ := cache.Capacity()
	mainAlgo, _ := cache.Algorithm()

	rules := setting.Rules
	ruleByName := make(map[string]operation_setting.ChannelAffinityRule, len(rules))
	for _, r := range rules {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		if !r.IncludeRuleName {
			continue
		}
		ruleByName[name] = r
	}

	byRuleName := make(map[string]int, len(ruleByName))
	for name := range ruleByName {
		byRuleName[name] = 0
	}

	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	total := len(keys)
	unknown := 0
	for _, k := range keys {
		prefix := channelAffinityCacheNamespace + ":"
		if !strings.HasPrefix(k, prefix) {
			unknown++
			continue
		}
		rest := strings.TrimPrefix(k, prefix)
		parts := strings.Split(rest, ":")
		if len(parts) < 2 {
			unknown++
			continue
		}
		ruleName := parts[0]
		rule, ok := ruleByName[ruleName]
		if !ok {
			unknown++
			continue
		}
		if rule.IncludeModelName {
			if len(parts) < 3 {
				unknown++
				continue
			}
		}
		if rule.IncludeUsingGroup {
			minParts := 3
			if rule.IncludeModelName {
				minParts = 4
			}
			if len(parts) < minParts {
				unknown++
				continue
			}
		}
		byRuleName[ruleName]++
	}

	return ChannelAffinityCacheStats{
		Enabled:       setting.Enabled,
		Total:         total,
		Unknown:       unknown,
		ByRuleName:    byRuleName,
		CacheCapacity: mainCap,
		CacheAlgo:     mainAlgo,
	}
}

func ClearChannelAffinityCacheAll() int {
	cache := getChannelAffinityCache()
	keys, err := cache.Keys()
	if err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache list keys failed: err=%v", err))
		keys = nil
	}
	if len(keys) > 0 {
		if _, err := cache.DeleteMany(keys); err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache delete many failed: err=%v", err))
		}
	}
	return len(keys)
}

func ClearChannelAffinityCacheByRuleName(ruleName string) (int, error) {
	ruleName = strings.TrimSpace(ruleName)
	if ruleName == "" {
		return 0, fmt.Errorf("rule_name 不能为空")
	}

	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil {
		return 0, fmt.Errorf("channel_affinity_setting 未初始化")
	}

	var matchedRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		r := &setting.Rules[i]
		if strings.TrimSpace(r.Name) != ruleName {
			continue
		}
		matchedRule = r
		break
	}
	if matchedRule == nil {
		return 0, fmt.Errorf("未知规则名称")
	}
	if !matchedRule.IncludeRuleName {
		return 0, fmt.Errorf("该规则未启用 include_rule_name，无法按规则清空缓存")
	}

	cache := getChannelAffinityCache()
	deleted, err := cache.DeleteByPrefix(ruleName)
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func matchAnyRegexCached(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	for _, pattern := range patterns {
		if pattern == "" {
			continue
		}
		re, ok := channelAffinityRegexCache.Load(pattern)
		if !ok {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				continue
			}
			re = compiled
			channelAffinityRegexCache.Store(pattern, re)
		}
		if re.(*regexp.Regexp).MatchString(s) {
			return true
		}
	}
	return false
}

func matchAnyIncludeFold(patterns []string, s string) bool {
	if len(patterns) == 0 || s == "" {
		return false
	}
	sLower := strings.ToLower(s)
	for _, p := range patterns {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(sLower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

func extractChannelAffinityValue(c *gin.Context, src operation_setting.ChannelAffinityKeySource) string {
	switch src.Type {
	case "context_int":
		if src.Key == "" {
			return ""
		}
		v := c.GetInt(src.Key)
		if v <= 0 {
			return ""
		}
		return strconv.Itoa(v)
	case "context_string":
		if src.Key == "" {
			return ""
		}
		return strings.TrimSpace(c.GetString(src.Key))
	case "gjson":
		if src.Path == "" {
			return ""
		}
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return ""
		}
		body, err := storage.Bytes()
		if err != nil || len(body) == 0 {
			return ""
		}
		res := gjson.GetBytes(body, src.Path)
		if !res.Exists() {
			return ""
		}
		switch res.Type {
		case gjson.String, gjson.Number, gjson.True, gjson.False:
			return strings.TrimSpace(res.String())
		default:
			return strings.TrimSpace(res.Raw)
		}
	default:
		return ""
	}
}

func buildChannelAffinityCacheKeySuffix(rule operation_setting.ChannelAffinityRule, modelName string, usingGroup string, affinityValue string) string {
	parts := make([]string, 0, 4)
	if rule.IncludeRuleName && rule.Name != "" {
		parts = append(parts, rule.Name)
	}
	if rule.IncludeModelName && modelName != "" {
		parts = append(parts, modelName)
	}
	if rule.IncludeUsingGroup && usingGroup != "" {
		parts = append(parts, usingGroup)
	}
	parts = append(parts, affinityValue)
	return strings.Join(parts, ":")
}

func setChannelAffinityContext(c *gin.Context, meta channelAffinityMeta) {
	c.Set(ginKeyChannelAffinityCacheKey, meta.CacheKey)
	c.Set(ginKeyChannelAffinityTTLSeconds, meta.TTLSeconds)
	c.Set(ginKeyChannelAffinityMeta, meta)
}

func getChannelAffinityContext(c *gin.Context) (string, int, bool) {
	keyAny, ok := c.Get(ginKeyChannelAffinityCacheKey)
	if !ok {
		return "", 0, false
	}
	key, ok := keyAny.(string)
	if !ok || key == "" {
		return "", 0, false
	}
	ttlAny, ok := c.Get(ginKeyChannelAffinityTTLSeconds)
	if !ok {
		return key, 0, true
	}
	ttlSeconds, _ := ttlAny.(int)
	return key, ttlSeconds, true
}

func getChannelAffinityMeta(c *gin.Context) (channelAffinityMeta, bool) {
	anyMeta, ok := c.Get(ginKeyChannelAffinityMeta)
	if !ok {
		return channelAffinityMeta{}, false
	}
	meta, ok := anyMeta.(channelAffinityMeta)
	if !ok {
		return channelAffinityMeta{}, false
	}
	return meta, true
}

func GetChannelAffinityStatsContext(c *gin.Context) (ChannelAffinityStatsContext, bool) {
	if c == nil {
		return ChannelAffinityStatsContext{}, false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return ChannelAffinityStatsContext{}, false
	}
	ruleName := strings.TrimSpace(meta.RuleName)
	keyFp := strings.TrimSpace(meta.KeyFingerprint)
	usingGroup := strings.TrimSpace(meta.UsingGroup)
	if ruleName == "" || keyFp == "" {
		return ChannelAffinityStatsContext{}, false
	}
	ttlSeconds := int64(meta.TTLSeconds)
	if ttlSeconds <= 0 {
		return ChannelAffinityStatsContext{}, false
	}
	return ChannelAffinityStatsContext{
		RuleName:       ruleName,
		UsingGroup:     usingGroup,
		KeyFingerprint: keyFp,
		TTLSeconds:     ttlSeconds,
	}, true
}

func affinityFingerprint(s string) string {
	if s == "" {
		return ""
	}
	hex := common.Sha1([]byte(s))
	if len(hex) >= 8 {
		return hex[:8]
	}
	return hex
}

func buildChannelAffinityKeyHint(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	if len(s) <= 12 {
		return s
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func buildChannelAffinityRequestPrefixDebug(c *gin.Context, maxChars int) (string, string, int, int, map[string]interface{}) {
	if c == nil {
		return "", "", 0, 0, nil
	}
	if maxChars <= 0 {
		maxChars = 512
	}
	if maxChars > maxChannelAffinityRequestPrefixChars {
		maxChars = maxChannelAffinityRequestPrefixChars
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return "", "", 0, 0, nil
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return "", "", 0, 0, nil
	}
	prefix, prefixHash, prefixLen, bodyLen, debug := buildChannelAffinityBodyDebug(body, maxChars)
	if debug != nil {
		debug["request_headers"] = buildChannelAffinityHeaderDebug(c)
	}
	return prefix, prefixHash, prefixLen, bodyLen, debug
}

func buildChannelAffinityBodyDebug(body []byte, maxChars int) (string, string, int, int, map[string]interface{}) {
	if len(body) == 0 {
		return "", "", 0, 0, nil
	}
	if maxChars <= 0 {
		maxChars = 512
	}
	if maxChars > maxChannelAffinityRequestPrefixChars {
		maxChars = maxChannelAffinityRequestPrefixChars
	}
	text := strings.TrimSpace(string(body))
	if text == "" {
		return "", "", 0, 0, nil
	}
	text = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(text)
	text = strings.Join(strings.Fields(text), " ")
	runes := []rune(text)
	bodyLen := len(runes)
	if len(runes) > maxChars {
		text = string(runes[:maxChars])
	}
	prefixLen := len([]rune(text))
	prefixHash := affinityFingerprint(text)
	debug := buildChannelAffinityJSONDebug(body)
	debug["body_sha1"] = common.Sha1(body)
	debug["body_bytes"] = len(body)
	debug["body_chars_normalized"] = bodyLen
	debug["first_prefix_sha1"] = prefixHash
	debug["first_prefix_chars"] = prefixLen
	return text, prefixHash, prefixLen, bodyLen, debug
}

func appendChannelAffinityRequestPrefixInfo(info map[string]interface{}, meta channelAffinityMeta) {
	if info == nil {
		return
	}
	if meta.RequestPrefixHash != "" {
		info["request_prefix_sha1"] = meta.RequestPrefixHash
		info["request_prefix_len"] = meta.RequestPrefixLen
		info["request_body_len"] = meta.RequestBodyLen
	}
	if len(meta.RequestDebug) > 0 {
		info["request_debug"] = meta.RequestDebug
	}
}

func AppendChannelAffinityFinalRequestDebug(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo)
	if !ok {
		return
	}
	info, ok := anyInfo.(map[string]interface{})
	if !ok {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.LogRequestPrefix {
		return
	}
	maxChars := 512
	if setting.RequestPrefixChars > 0 {
		maxChars = setting.RequestPrefixChars
	}
	_, _, _, _, debug := buildChannelAffinityBodyDebug(body, maxChars)
	if len(debug) == 0 {
		return
	}
	info["final_request_debug"] = debug
	c.Set(ginKeyChannelAffinityLogInfo, info)
	logChannelAffinityDebug("channel affinity final request debug", info, debug)
}

func buildChannelAffinityJSONDebug(body []byte) map[string]interface{} {
	debug := make(map[string]interface{})
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(body, &obj); err != nil {
		debug["json_valid"] = false
		return debug
	}
	debug["json_valid"] = true
	debug["top_level_keys"] = sortedRawMessageKeys(obj)

	setStringFieldDebug(debug, obj, "model", "model")
	setStringFieldDebug(debug, obj, "service_tier", "service_tier")
	setHashedRawFieldDebug(debug, obj, "prompt_cache_key", "prompt_cache_key")
	setHashedRawFieldDebug(debug, obj, "previous_response_id", "previous_response_id")
	setHashedRawFieldDebug(debug, obj, "conversation", "conversation")
	setHashedRawFieldDebug(debug, obj, "instructions", "instructions")

	if raw, ok := obj["input"]; ok {
		debug["input"] = summarizeChannelAffinityInput(raw)
	}
	if raw, ok := obj["metadata"]; ok {
		debug["metadata"] = summarizeChannelAffinityObject(raw)
	}
	if raw, ok := obj["reasoning"]; ok {
		debug["reasoning"] = summarizeChannelAffinityObject(raw)
	}
	if raw, ok := obj["store"]; ok {
		debug["store"] = summarizeScalarRawMessage(raw)
	}
	if raw, ok := obj["tools"]; ok {
		debug["tools"] = summarizeChannelAffinityArray(raw)
	}
	return debug
}

func AppendChannelAffinityResponseDebug(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	if serviceTier := extractUpstreamResponseServiceTier(body); serviceTier != "" {
		c.Set(ginKeyUpstreamResponseServiceTier, serviceTier)
	}
	anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo)
	if !ok {
		return
	}
	info, ok := anyInfo.(map[string]interface{})
	if !ok {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.LogRequestPrefix {
		return
	}
	debug := buildChannelAffinityResponseDebug(body)
	if len(debug) == 0 {
		return
	}
	info["response_debug"] = debug
	c.Set(ginKeyChannelAffinityLogInfo, info)
	logChannelAffinityDebug("channel affinity response debug", info, debug)
}

func extractUpstreamResponseServiceTier(body []byte) string {
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(body, &obj); err != nil {
		return ""
	}
	if raw, ok := obj["service_tier"]; ok {
		var value string
		if err := common.Unmarshal(raw, &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	if raw, ok := obj["response"]; ok {
		var response map[string]json.RawMessage
		if err := common.Unmarshal(raw, &response); err != nil {
			return ""
		}
		rawServiceTier, ok := response["service_tier"]
		if !ok {
			return ""
		}
		var value string
		if err := common.Unmarshal(rawServiceTier, &value); err == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func buildChannelAffinityResponseDebug(body []byte) map[string]interface{} {
	debug := make(map[string]interface{})
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(body, &obj); err != nil {
		debug["json_valid"] = false
		return debug
	}
	debug["json_valid"] = true
	debug["top_level_keys"] = sortedRawMessageKeys(obj)
	setStringFieldDebug(debug, obj, "type", "type")
	setStringFieldDebug(debug, obj, "service_tier", "service_tier")

	if raw, ok := obj["response"]; ok {
		var responseObj map[string]json.RawMessage
		response := map[string]interface{}{
			"json_type": common.GetJsonType(raw),
			"sha1":      common.Sha1(raw),
			"bytes":     len(raw),
		}
		if err := common.Unmarshal(raw, &responseObj); err == nil {
			response["top_level_keys"] = sortedRawMessageKeys(responseObj)
			setStringFieldDebug(response, responseObj, "service_tier", "service_tier")
			setStringFieldDebug(response, responseObj, "model", "model")
			setHashedRawFieldDebug(response, responseObj, "id", "id")
		}
		debug["response"] = response
	}
	return debug
}

func buildChannelAffinityHeaderDebug(c *gin.Context) map[string]interface{} {
	if c == nil || c.Request == nil {
		return nil
	}
	type headerDigest struct {
		key  string
		hash string
	}
	digests := make([]headerDigest, 0, len(c.Request.Header))
	keys := make([]string, 0, len(c.Request.Header))
	redactedKeys := make([]string, 0)
	for key, values := range c.Request.Header {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		if isSensitiveChannelAffinityHeader(normalized) {
			redactedKeys = append(redactedKeys, normalized)
			continue
		}
		keys = append(keys, normalized)
		digests = append(digests, headerDigest{
			key:  normalized,
			hash: common.Sha1([]byte(strings.Join(values, "\x00"))),
		})
	}
	sort.Strings(keys)
	sort.Strings(redactedKeys)
	sort.Slice(digests, func(i, j int) bool {
		return digests[i].key < digests[j].key
	})
	digestParts := make([]string, 0, len(digests))
	sha1ByKey := make(map[string]string, len(digests))
	for _, digest := range digests {
		digestParts = append(digestParts, digest.key+"="+digest.hash)
		sha1ByKey[digest.key] = digest.hash
	}
	debug := map[string]interface{}{
		"keys":        keys,
		"sha1":        common.Sha1([]byte(strings.Join(digestParts, "\n"))),
		"sha1_by_key": sha1ByKey,
	}
	if len(redactedKeys) > 0 {
		debug["redacted_keys"] = redactedKeys
	}
	return debug
}

func isSensitiveChannelAffinityHeader(header string) bool {
	switch strings.ToLower(strings.TrimSpace(header)) {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key", "api-key":
		return true
	default:
		return false
	}
}

func setStringFieldDebug(debug map[string]interface{}, obj map[string]json.RawMessage, field string, output string) {
	raw, ok := obj[field]
	if !ok {
		return
	}
	var value string
	if err := common.Unmarshal(raw, &value); err == nil {
		debug[output] = value
		return
	}
	debug[output] = summarizeScalarRawMessage(raw)
}

func setHashedRawFieldDebug(debug map[string]interface{}, obj map[string]json.RawMessage, field string, output string) {
	raw, ok := obj[field]
	if !ok {
		return
	}
	debug[output] = summarizeHashedRawMessage(raw)
}

func summarizeHashedRawMessage(raw json.RawMessage) map[string]interface{} {
	summary := map[string]interface{}{
		"json_type": common.GetJsonType(raw),
		"sha1":      common.Sha1(raw),
		"bytes":     len(raw),
	}
	if common.GetJsonType(raw) == "string" {
		var value string
		if err := common.Unmarshal(raw, &value); err == nil {
			summary["chars"] = len([]rune(value))
		}
	}
	return summary
}

func summarizeScalarRawMessage(raw json.RawMessage) interface{} {
	jsonType := common.GetJsonType(raw)
	switch jsonType {
	case "boolean":
		var value bool
		if err := common.Unmarshal(raw, &value); err == nil {
			return value
		}
	case "number":
		return map[string]interface{}{
			"json_type": jsonType,
			"sha1":      common.Sha1(raw),
		}
	case "string":
		return summarizeHashedRawMessage(raw)
	}
	return map[string]interface{}{
		"json_type": jsonType,
		"sha1":      common.Sha1(raw),
		"bytes":     len(raw),
	}
}

func summarizeChannelAffinityInput(raw json.RawMessage) map[string]interface{} {
	summary := map[string]interface{}{
		"json_type": common.GetJsonType(raw),
		"sha1":      common.Sha1(raw),
		"bytes":     len(raw),
	}
	var items []json.RawMessage
	if err := common.Unmarshal(raw, &items); err != nil {
		return summary
	}
	summary["items_count"] = len(items)
	if len(items) > 0 {
		summary["first_item"] = summarizeChannelAffinityInputItem(items[0])
		summary["last_item"] = summarizeChannelAffinityInputItem(items[len(items)-1])
		summary["item_fingerprints"] = summarizeChannelAffinityInputItemFingerprints(items)
	}
	return summary
}

func summarizeChannelAffinityInputItemFingerprints(items []json.RawMessage) map[string]interface{} {
	summary := map[string]interface{}{
		"limit":       maxChannelAffinityInputItemFingerprints,
		"total_count": len(items),
	}
	if len(items) == 0 {
		summary["items"] = []map[string]interface{}{}
		summary["covered_count"] = 0
		return summary
	}

	indexes := make([]int, 0, len(items))
	if len(items) <= maxChannelAffinityInputItemFingerprints {
		for i := range items {
			indexes = append(indexes, i)
		}
	} else {
		headCount := maxChannelAffinityInputItemFingerprints / 2
		tailCount := maxChannelAffinityInputItemFingerprints - headCount
		for i := 0; i < headCount; i++ {
			indexes = append(indexes, i)
		}
		for i := len(items) - tailCount; i < len(items); i++ {
			indexes = append(indexes, i)
		}
		summary["omitted_middle_count"] = len(items) - len(indexes)
	}

	fingerprints := make([]map[string]interface{}, 0, len(indexes))
	for _, idx := range indexes {
		item := summarizeChannelAffinityInputItem(items[idx])
		item["index"] = idx
		fingerprints = append(fingerprints, item)
	}
	summary["items"] = fingerprints
	summary["covered_count"] = len(fingerprints)
	return summary
}

func summarizeChannelAffinityInputItem(raw json.RawMessage) map[string]interface{} {
	summary := map[string]interface{}{
		"json_type": common.GetJsonType(raw),
		"sha1":      common.Sha1(raw),
		"bytes":     len(raw),
	}
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &obj); err != nil {
		return summary
	}
	summary["keys"] = sortedRawMessageKeys(obj)
	setStringFieldDebug(summary, obj, "type", "type")
	setStringFieldDebug(summary, obj, "role", "role")
	if rawContent, ok := obj["content"]; ok {
		summary["content"] = summarizeChannelAffinityArray(rawContent)
	}
	return summary
}

func summarizeChannelAffinityArray(raw json.RawMessage) map[string]interface{} {
	summary := map[string]interface{}{
		"json_type": common.GetJsonType(raw),
		"sha1":      common.Sha1(raw),
		"bytes":     len(raw),
	}
	var items []json.RawMessage
	if err := common.Unmarshal(raw, &items); err == nil {
		summary["items_count"] = len(items)
	}
	return summary
}

func summarizeChannelAffinityObject(raw json.RawMessage) map[string]interface{} {
	summary := map[string]interface{}{
		"json_type": common.GetJsonType(raw),
		"sha1":      common.Sha1(raw),
		"bytes":     len(raw),
	}
	var obj map[string]json.RawMessage
	if err := common.Unmarshal(raw, &obj); err == nil {
		summary["keys"] = sortedRawMessageKeys(obj)
	}
	return summary
}

func sortedRawMessageKeys(obj map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func logChannelAffinityDebug(message string, info map[string]interface{}, debug map[string]interface{}) {
	if len(debug) == 0 {
		return
	}
	fields := map[string]interface{}{
		"debug": debug,
	}
	for _, key := range []string{"rule_name", "model", "using_group", "selected_group", "channel_id", "key_fp", "request_path"} {
		if value, ok := info[key]; ok {
			fields[key] = value
		}
	}
	data, err := common.Marshal(fields)
	if err != nil {
		return
	}
	common.SysLog(fmt.Sprintf("%s: %s", message, string(data)))
}

func cloneStringAnyMap(src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func mergeChannelOverride(base map[string]interface{}, tpl map[string]interface{}) map[string]interface{} {
	if len(base) == 0 && len(tpl) == 0 {
		return map[string]interface{}{}
	}
	if len(tpl) == 0 {
		return base
	}
	out := cloneStringAnyMap(base)
	for k, v := range tpl {
		if strings.EqualFold(strings.TrimSpace(k), "operations") {
			baseOps, hasBaseOps := extractParamOperations(out[k])
			tplOps, hasTplOps := extractParamOperations(v)
			if hasTplOps {
				if hasBaseOps {
					out[k] = append(tplOps, baseOps...)
				} else {
					out[k] = tplOps
				}
				continue
			}
		}
		if _, exists := out[k]; exists {
			continue
		}
		out[k] = v
	}
	return out
}

func extractParamOperations(value interface{}) ([]interface{}, bool) {
	switch ops := value.(type) {
	case []interface{}:
		if len(ops) == 0 {
			return []interface{}{}, true
		}
		cloned := make([]interface{}, 0, len(ops))
		cloned = append(cloned, ops...)
		return cloned, true
	case []map[string]interface{}:
		cloned := make([]interface{}, 0, len(ops))
		for _, op := range ops {
			cloned = append(cloned, op)
		}
		return cloned, true
	default:
		return nil, false
	}
}

func appendChannelAffinityTemplateAdminInfo(c *gin.Context, meta channelAffinityMeta) {
	if c == nil {
		return
	}
	if len(meta.ParamTemplate) == 0 {
		return
	}

	templateInfo := map[string]interface{}{
		"applied":             true,
		"rule_name":           meta.RuleName,
		"param_override_keys": len(meta.ParamTemplate),
	}
	if anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo); ok {
		if info, ok := anyInfo.(map[string]interface{}); ok {
			info["override_template"] = templateInfo
			appendChannelAffinityRequestPrefixInfo(info, meta)
			c.Set(ginKeyChannelAffinityLogInfo, info)
			return
		}
	}
	info := map[string]interface{}{
		"reason":            meta.RuleName,
		"rule_name":         meta.RuleName,
		"using_group":       meta.UsingGroup,
		"model":             meta.ModelName,
		"request_path":      meta.RequestPath,
		"key_source":        meta.KeySourceType,
		"key_key":           meta.KeySourceKey,
		"key_path":          meta.KeySourcePath,
		"key_hint":          meta.KeyHint,
		"key_fp":            meta.KeyFingerprint,
		"override_template": templateInfo,
	}
	appendChannelAffinityRequestPrefixInfo(info, meta)
	c.Set(ginKeyChannelAffinityLogInfo, info)
}

// ApplyChannelAffinityOverrideTemplate merges per-rule channel override templates onto the selected channel override config.
func ApplyChannelAffinityOverrideTemplate(c *gin.Context, paramOverride map[string]interface{}) (map[string]interface{}, bool) {
	if c == nil {
		return paramOverride, false
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return paramOverride, false
	}
	if len(meta.ParamTemplate) == 0 {
		return paramOverride, false
	}

	mergedParam := mergeChannelOverride(paramOverride, meta.ParamTemplate)
	appendChannelAffinityTemplateAdminInfo(c, meta)
	return mergedParam, true
}

func GetPreferredChannelByAffinity(c *gin.Context, modelName string, usingGroup string) (int, bool) {
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return 0, false
	}
	path := ""
	if c != nil && c.Request != nil && c.Request.URL != nil {
		path = c.Request.URL.Path
	}
	userAgent := ""
	if c != nil && c.Request != nil {
		userAgent = c.Request.UserAgent()
	}

	for _, rule := range setting.Rules {
		if !matchAnyRegexCached(rule.ModelRegex, modelName) {
			continue
		}
		if len(rule.PathRegex) > 0 && !matchAnyRegexCached(rule.PathRegex, path) {
			continue
		}
		if len(rule.UserAgentInclude) > 0 && !matchAnyIncludeFold(rule.UserAgentInclude, userAgent) {
			continue
		}
		var affinityValue string
		var usedSource operation_setting.ChannelAffinityKeySource
		for _, src := range rule.KeySources {
			affinityValue = extractChannelAffinityValue(c, src)
			if affinityValue != "" {
				usedSource = src
				break
			}
		}
		if affinityValue == "" {
			continue
		}
		if rule.ValueRegex != "" && !matchAnyRegexCached([]string{rule.ValueRegex}, affinityValue) {
			continue
		}

		ttlSeconds := rule.TTLSeconds
		if ttlSeconds <= 0 {
			ttlSeconds = setting.DefaultTTLSeconds
		}
		cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, modelName, usingGroup, affinityValue)
		cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
		requestPrefix, requestPrefixHash, requestPrefixLen, requestBodyLen := "", "", 0, 0
		var requestDebug map[string]interface{}
		if setting.LogRequestPrefix {
			requestPrefix, requestPrefixHash, requestPrefixLen, requestBodyLen, requestDebug = buildChannelAffinityRequestPrefixDebug(c, setting.RequestPrefixChars)
		}
		setChannelAffinityContext(c, channelAffinityMeta{
			CacheKey:          cacheKeyFull,
			TTLSeconds:        ttlSeconds,
			RuleName:          rule.Name,
			SkipRetry:         rule.SkipRetryOnFailure,
			ParamTemplate:     cloneStringAnyMap(rule.ParamOverrideTemplate),
			KeySourceType:     strings.TrimSpace(usedSource.Type),
			KeySourceKey:      strings.TrimSpace(usedSource.Key),
			KeySourcePath:     strings.TrimSpace(usedSource.Path),
			KeyHint:           buildChannelAffinityKeyHint(affinityValue),
			KeyFingerprint:    affinityFingerprint(affinityValue),
			UsingGroup:        usingGroup,
			ModelName:         modelName,
			RequestPath:       path,
			RequestPrefix:     requestPrefix,
			RequestPrefixHash: requestPrefixHash,
			RequestPrefixLen:  requestPrefixLen,
			RequestBodyLen:    requestBodyLen,
			RequestDebug:      requestDebug,
		})

		cache := getChannelAffinityCache()
		channelID, found, err := cache.Get(cacheKeySuffix)
		if err != nil {
			common.SysError(fmt.Sprintf("channel affinity cache get failed: key=%s, err=%v", cacheKeyFull, err))
			return 0, false
		}
		if found {
			return channelID, true
		}
		return 0, false
	}
	return 0, false
}

func ShouldSkipRetryAfterChannelAffinityFailure(c *gin.Context) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(ginKeyChannelAffinitySkipRetry)
	if ok {
		b, ok := v.(bool)
		if ok {
			return b
		}
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return false
	}
	return meta.SkipRetry
}

func MarkChannelAffinityUsed(c *gin.Context, selectedGroup string, channelID int) {
	if c == nil || channelID <= 0 {
		return
	}
	meta, ok := getChannelAffinityMeta(c)
	if !ok {
		return
	}
	c.Set(ginKeyChannelAffinitySkipRetry, meta.SkipRetry)
	info := map[string]interface{}{
		"reason":         meta.RuleName,
		"rule_name":      meta.RuleName,
		"using_group":    meta.UsingGroup,
		"selected_group": selectedGroup,
		"model":          meta.ModelName,
		"request_path":   meta.RequestPath,
		"channel_id":     channelID,
		"key_source":     meta.KeySourceType,
		"key_key":        meta.KeySourceKey,
		"key_path":       meta.KeySourcePath,
		"key_hint":       meta.KeyHint,
		"key_fp":         meta.KeyFingerprint,
	}
	appendChannelAffinityRequestPrefixInfo(info, meta)
	c.Set(ginKeyChannelAffinityLogInfo, info)
	if len(meta.RequestDebug) > 0 {
		logChannelAffinityDebug("channel affinity request debug", info, meta.RequestDebug)
	}
}

func AppendChannelAffinityAdminInfo(c *gin.Context, adminInfo map[string]interface{}) {
	if c == nil || adminInfo == nil {
		return
	}
	anyInfo, ok := c.Get(ginKeyChannelAffinityLogInfo)
	if !ok || anyInfo == nil {
		return
	}
	adminInfo["channel_affinity"] = anyInfo
}

func RecordChannelAffinity(c *gin.Context, channelID int) {
	if channelID <= 0 {
		return
	}
	setting := operation_setting.GetChannelAffinitySetting()
	if setting == nil || !setting.Enabled {
		return
	}
	if setting.SwitchOnSuccess && c != nil {
		if successChannelID := c.GetInt("channel_id"); successChannelID > 0 {
			channelID = successChannelID
		}
	}
	cacheKey, ttlSeconds, ok := getChannelAffinityContext(c)
	if !ok {
		return
	}
	if ttlSeconds <= 0 {
		ttlSeconds = setting.DefaultTTLSeconds
	}
	if ttlSeconds <= 0 {
		ttlSeconds = 3600
	}
	cache := getChannelAffinityCache()
	if err := cache.SetWithTTL(cacheKey, channelID, time.Duration(ttlSeconds)*time.Second); err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache set failed: key=%s, err=%v", cacheKey, err))
	}
}

func ClearCurrentChannelAffinityCache(c *gin.Context) {
	cacheKey, _, ok := getChannelAffinityContext(c)
	if !ok || cacheKey == "" {
		return
	}
	cache := getChannelAffinityCache()
	if _, err := cache.DeleteMany([]string{cacheKey}); err != nil {
		common.SysError(fmt.Sprintf("channel affinity cache delete failed: key=%s, err=%v", cacheKey, err))
	}
}

type ChannelAffinityUsageCacheStats struct {
	RuleName            string `json:"rule_name"`
	UsingGroup          string `json:"using_group"`
	KeyFingerprint      string `json:"key_fp"`
	CachedTokenRateMode string `json:"cached_token_rate_mode"`

	Hit           int64 `json:"hit"`
	Total         int64 `json:"total"`
	WindowSeconds int64 `json:"window_seconds"`

	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	TotalTokens          int64 `json:"total_tokens"`
	CachedTokens         int64 `json:"cached_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
	LastSeenAt           int64 `json:"last_seen_at"`
}

type ChannelAffinityUsageCacheCounters struct {
	CachedTokenRateMode string `json:"cached_token_rate_mode"`

	Hit           int64 `json:"hit"`
	Total         int64 `json:"total"`
	WindowSeconds int64 `json:"window_seconds"`

	PromptTokens         int64 `json:"prompt_tokens"`
	CompletionTokens     int64 `json:"completion_tokens"`
	TotalTokens          int64 `json:"total_tokens"`
	CachedTokens         int64 `json:"cached_tokens"`
	PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
	LastSeenAt           int64 `json:"last_seen_at"`
}

var channelAffinityUsageCacheStatsLocks [64]sync.Mutex

// ObserveChannelAffinityUsageCacheByRelayFormat records usage cache stats with a stable rate mode derived from relay format.
func ObserveChannelAffinityUsageCacheByRelayFormat(c *gin.Context, usage *dto.Usage, relayFormat types.RelayFormat) {
	ObserveChannelAffinityUsageCacheFromContext(c, usage, cachedTokenRateModeByRelayFormat(relayFormat))
}

func ObserveChannelAffinityUsageCacheFromContext(c *gin.Context, usage *dto.Usage, cachedTokenRateMode string) {
	statsCtx, ok := GetChannelAffinityStatsContext(c)
	if !ok {
		return
	}
	observeChannelAffinityUsageCache(statsCtx, usage, cachedTokenRateMode)
}

func GetChannelAffinityUsageCacheStats(ruleName, usingGroup, keyFp string) ChannelAffinityUsageCacheStats {
	ruleName = strings.TrimSpace(ruleName)
	usingGroup = strings.TrimSpace(usingGroup)
	keyFp = strings.TrimSpace(keyFp)

	entryKey := channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFp)
	if entryKey == "" {
		return ChannelAffinityUsageCacheStats{
			RuleName:       ruleName,
			UsingGroup:     usingGroup,
			KeyFingerprint: keyFp,
		}
	}

	cache := getChannelAffinityUsageCacheStatsCache()
	v, found, err := cache.Get(entryKey)
	if err != nil || !found {
		return ChannelAffinityUsageCacheStats{
			RuleName:       ruleName,
			UsingGroup:     usingGroup,
			KeyFingerprint: keyFp,
		}
	}
	return ChannelAffinityUsageCacheStats{
		CachedTokenRateMode:  v.CachedTokenRateMode,
		RuleName:             ruleName,
		UsingGroup:           usingGroup,
		KeyFingerprint:       keyFp,
		Hit:                  v.Hit,
		Total:                v.Total,
		WindowSeconds:        v.WindowSeconds,
		PromptTokens:         v.PromptTokens,
		CompletionTokens:     v.CompletionTokens,
		TotalTokens:          v.TotalTokens,
		CachedTokens:         v.CachedTokens,
		PromptCacheHitTokens: v.PromptCacheHitTokens,
		LastSeenAt:           v.LastSeenAt,
	}
}

func observeChannelAffinityUsageCache(statsCtx ChannelAffinityStatsContext, usage *dto.Usage, cachedTokenRateMode string) {
	entryKey := channelAffinityUsageCacheEntryKey(statsCtx.RuleName, statsCtx.UsingGroup, statsCtx.KeyFingerprint)
	if entryKey == "" {
		return
	}

	windowSeconds := statsCtx.TTLSeconds
	if windowSeconds <= 0 {
		return
	}

	cache := getChannelAffinityUsageCacheStatsCache()
	ttl := time.Duration(windowSeconds) * time.Second

	lock := channelAffinityUsageCacheStatsLock(entryKey)
	lock.Lock()
	defer lock.Unlock()

	prev, found, err := cache.Get(entryKey)
	if err != nil {
		return
	}
	next := prev
	if !found {
		next = ChannelAffinityUsageCacheCounters{}
	}
	currentMode := normalizeCachedTokenRateMode(cachedTokenRateMode)
	if currentMode != "" {
		if next.CachedTokenRateMode == "" {
			next.CachedTokenRateMode = currentMode
		} else if next.CachedTokenRateMode != currentMode && next.CachedTokenRateMode != cacheTokenRateModeMixed {
			next.CachedTokenRateMode = cacheTokenRateModeMixed
		}
	}
	next.Total++
	hit, cachedTokens, promptCacheHitTokens := usageCacheSignals(usage)
	if hit {
		next.Hit++
	}
	next.WindowSeconds = windowSeconds
	next.LastSeenAt = time.Now().Unix()
	next.CachedTokens += cachedTokens
	next.PromptCacheHitTokens += promptCacheHitTokens
	next.PromptTokens += int64(usagePromptTokens(usage))
	next.CompletionTokens += int64(usageCompletionTokens(usage))
	next.TotalTokens += int64(usageTotalTokens(usage))
	_ = cache.SetWithTTL(entryKey, next, ttl)
}

func normalizeCachedTokenRateMode(mode string) string {
	switch mode {
	case cacheTokenRateModeCachedOverPrompt:
		return cacheTokenRateModeCachedOverPrompt
	case cacheTokenRateModeCachedOverPromptPlusCached:
		return cacheTokenRateModeCachedOverPromptPlusCached
	case cacheTokenRateModeMixed:
		return cacheTokenRateModeMixed
	default:
		return ""
	}
}

func cachedTokenRateModeByRelayFormat(relayFormat types.RelayFormat) string {
	switch relayFormat {
	case types.RelayFormatOpenAI, types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		return cacheTokenRateModeCachedOverPrompt
	case types.RelayFormatClaude:
		return cacheTokenRateModeCachedOverPromptPlusCached
	default:
		return ""
	}
}

func channelAffinityUsageCacheEntryKey(ruleName, usingGroup, keyFp string) string {
	ruleName = strings.TrimSpace(ruleName)
	usingGroup = strings.TrimSpace(usingGroup)
	keyFp = strings.TrimSpace(keyFp)
	if ruleName == "" || keyFp == "" {
		return ""
	}
	return ruleName + "\n" + usingGroup + "\n" + keyFp
}

func usageCacheSignals(usage *dto.Usage) (hit bool, cachedTokens int64, promptCacheHitTokens int64) {
	if usage == nil {
		return false, 0, 0
	}

	cached := int64(0)
	if usage.PromptTokensDetails.CachedTokens > 0 {
		cached = int64(usage.PromptTokensDetails.CachedTokens)
	} else if usage.InputTokensDetails != nil && usage.InputTokensDetails.CachedTokens > 0 {
		cached = int64(usage.InputTokensDetails.CachedTokens)
	}
	pcht := int64(0)
	if usage.PromptCacheHitTokens > 0 {
		pcht = int64(usage.PromptCacheHitTokens)
	}
	return cached > 0 || pcht > 0, cached, pcht
}

func usagePromptTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.PromptTokens > 0 {
		return usage.PromptTokens
	}
	return usage.InputTokens
}

func usageCompletionTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.CompletionTokens > 0 {
		return usage.CompletionTokens
	}
	return usage.OutputTokens
}

func usageTotalTokens(usage *dto.Usage) int {
	if usage == nil {
		return 0
	}
	if usage.TotalTokens > 0 {
		return usage.TotalTokens
	}
	pt := usagePromptTokens(usage)
	ct := usageCompletionTokens(usage)
	if pt > 0 || ct > 0 {
		return pt + ct
	}
	return 0
}

func getChannelAffinityUsageCacheStatsCache() *cachex.HybridCache[ChannelAffinityUsageCacheCounters] {
	channelAffinityUsageCacheStatsOnce.Do(func() {
		setting := operation_setting.GetChannelAffinitySetting()
		capacity := 100_000
		defaultTTLSeconds := 3600
		if setting != nil {
			if setting.MaxEntries > 0 {
				capacity = setting.MaxEntries
			}
			if setting.DefaultTTLSeconds > 0 {
				defaultTTLSeconds = setting.DefaultTTLSeconds
			}
		}

		channelAffinityUsageCacheStatsCache = cachex.NewHybridCache[ChannelAffinityUsageCacheCounters](cachex.HybridCacheConfig[ChannelAffinityUsageCacheCounters]{
			Namespace: cachex.Namespace(channelAffinityUsageCacheStatsNamespace),
			Redis:     common.RDB,
			RedisEnabled: func() bool {
				return common.RedisEnabled && common.RDB != nil
			},
			RedisCodec: cachex.JSONCodec[ChannelAffinityUsageCacheCounters]{},
			Memory: func() *hot.HotCache[string, ChannelAffinityUsageCacheCounters] {
				return hot.NewHotCache[string, ChannelAffinityUsageCacheCounters](hot.LRU, capacity).
					WithTTL(time.Duration(defaultTTLSeconds) * time.Second).
					WithJanitor().
					Build()
			},
		})
	})
	return channelAffinityUsageCacheStatsCache
}

func channelAffinityUsageCacheStatsLock(key string) *sync.Mutex {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	idx := h.Sum32() % uint32(len(channelAffinityUsageCacheStatsLocks))
	return &channelAffinityUsageCacheStatsLocks[idx]
}
