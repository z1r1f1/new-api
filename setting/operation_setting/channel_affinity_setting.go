package operation_setting

import (
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type ChannelAffinityKeySource struct {
	Type string `json:"type"` // context_int, context_string, request_header, gjson
	Key  string `json:"key,omitempty"`
	Path string `json:"path,omitempty"`
}

type ChannelAffinityRule struct {
	Name             string                     `json:"name"`
	ModelRegex       []string                   `json:"model_regex"`
	PathRegex        []string                   `json:"path_regex"`
	UserAgentInclude []string                   `json:"user_agent_include,omitempty"`
	KeySources       []ChannelAffinityKeySource `json:"key_sources"`

	ValueRegex string `json:"value_regex"`
	TTLSeconds int    `json:"ttl_seconds"`

	ParamOverrideTemplate map[string]interface{} `json:"param_override_template,omitempty"`

	SkipRetryOnFailure bool `json:"skip_retry_on_failure"`

	IncludeUsingGroup bool `json:"include_using_group"`
	IncludeModelName  bool `json:"include_model_name"`
	IncludeRuleName   bool `json:"include_rule_name"`
}

type ChannelAffinitySetting struct {
	Enabled            bool                  `json:"enabled"`
	SwitchOnSuccess    bool                  `json:"switch_on_success"`
	MaxEntries         int                   `json:"max_entries"`
	DefaultTTLSeconds  int                   `json:"default_ttl_seconds"`
	LogRequestPrefix   bool                  `json:"log_request_prefix"`
	RequestPrefixChars int                   `json:"request_prefix_chars"`
	Rules              []ChannelAffinityRule `json:"rules"`
}

var codexCliPassThroughHeaders = []string{
	"Originator",
	"Session_id",
	"User-Agent",
	"X-Codex-Beta-Features",
	"X-Codex-Turn-Metadata",
}

var claudeCliPassThroughHeaders = []string{
	"X-Stainless-Arch",
	"X-Stainless-Lang",
	"X-Stainless-Os",
	"X-Stainless-Package-Version",
	"X-Stainless-Retry-Count",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
	"X-Stainless-Timeout",
	"User-Agent",
	"X-App",
	"Anthropic-Beta",
	"Anthropic-Dangerous-Direct-Browser-Access",
	"Anthropic-Version",
}

var codexCliChannelAffinityKeySourceOrder = []ChannelAffinityKeySource{
	{Type: "gjson", Path: "prompt_cache_key"},
	{Type: "gjson", Path: "openai_prompt_cache_key"},
	{Type: "gjson", Path: "metadata.prompt_cache_key"},
	{Type: "gjson", Path: "metadata.openai_prompt_cache_key"},
	{Type: "gjson", Path: "metadata.user_id"},
	{Type: "gjson", Path: "session_id"},
	{Type: "gjson", Path: "sessionId"},
	{Type: "gjson", Path: "conversation_id"},
	{Type: "gjson", Path: "conversationId"},
	{Type: "gjson", Path: "thread_id"},
	{Type: "gjson", Path: "threadId"},
	{Type: "gjson", Path: "request_session_id"},
	{Type: "gjson", Path: "metadata.session_id"},
	{Type: "gjson", Path: "metadata.sessionId"},
	{Type: "gjson", Path: "metadata.conversation_id"},
	{Type: "gjson", Path: "metadata.conversationId"},
	{Type: "gjson", Path: "metadata.thread_id"},
	{Type: "gjson", Path: "metadata.threadId"},
	{Type: "gjson", Path: "metadata.claude_session_id"},
	{Type: "gjson", Path: "metadata.claudeSessionId"},
	{Type: "gjson", Path: "metadata.codex_session_id"},
	{Type: "gjson", Path: "metadata.codexSessionId"},
	{Type: "request_header", Key: "Session_id"},
	{Type: "request_header", Key: "X-Claude-Session-Id"},
	{Type: "request_header", Key: "X-Claude-Code-Session-Id"},
	{Type: "request_header", Key: "X-Codex-Session-Id"},
}

func defaultCodexCliChannelAffinityKeySources() []ChannelAffinityKeySource {
	sources := make([]ChannelAffinityKeySource, 0, len(codexCliChannelAffinityKeySourceOrder))
	sources = append(sources, codexCliChannelAffinityKeySourceOrder...)
	return sources
}

func buildPassHeaderTemplate(headers []string) map[string]interface{} {
	clonedHeaders := make([]string, 0, len(headers))
	clonedHeaders = append(clonedHeaders, headers...)
	return map[string]interface{}{
		"operations": []map[string]interface{}{
			{
				"mode":        "pass_headers",
				"value":       clonedHeaders,
				"keep_origin": true,
			},
		},
	}
}

func buildCodexCliParamOverrideTemplate() map[string]interface{} {
	clonedHeaders := make([]string, 0, len(codexCliPassThroughHeaders))
	clonedHeaders = append(clonedHeaders, codexCliPassThroughHeaders...)
	return map[string]interface{}{
		"operations": []map[string]interface{}{
			{
				"mode":        "pass_headers",
				"value":       clonedHeaders,
				"keep_origin": true,
			},
			codexCliPromptCacheSessionSyncOperation(),
		},
	}
}

func codexCliPromptCacheSessionSyncOperation() map[string]interface{} {
	return map[string]interface{}{
		"mode": "sync_fields",
		"from": "json:prompt_cache_key",
		"to":   "header:session_id",
	}
}

var channelAffinitySetting = ChannelAffinitySetting{
	Enabled:            true,
	SwitchOnSuccess:    true,
	MaxEntries:         100_000,
	DefaultTTLSeconds:  3600,
	LogRequestPrefix:   true,
	RequestPrefixChars: 65536,
	Rules: []ChannelAffinityRule{
		{
			Name:                  "codex cli trace",
			ModelRegex:            []string{"^gpt-.*$"},
			PathRegex:             []string{"/v1/responses", "/v1/messages?"},
			KeySources:            defaultCodexCliChannelAffinityKeySources(),
			ValueRegex:            "",
			TTLSeconds:            0,
			ParamOverrideTemplate: buildCodexCliParamOverrideTemplate(),
			SkipRetryOnFailure:    true,
			IncludeUsingGroup:     true,
			IncludeRuleName:       true,
			UserAgentInclude:      nil,
		},
		{
			Name:       "claude cli trace",
			ModelRegex: []string{"^claude-.*$"},
			PathRegex:  []string{"/v1/messages"},
			KeySources: []ChannelAffinityKeySource{
				{Type: "gjson", Path: "metadata.user_id"},
			},
			ValueRegex:            "",
			TTLSeconds:            0,
			ParamOverrideTemplate: buildPassHeaderTemplate(claudeCliPassThroughHeaders),
			SkipRetryOnFailure:    true,
			IncludeUsingGroup:     true,
			IncludeRuleName:       true,
			UserAgentInclude:      nil,
		},
	},
}

func init() {
	config.GlobalConfig.Register("channel_affinity_setting", &channelAffinitySetting)
}

func GetChannelAffinitySetting() *ChannelAffinitySetting {
	ensureChannelAffinitySettingCompatibility(&channelAffinitySetting)
	return &channelAffinitySetting
}

func ensureChannelAffinitySettingCompatibility(setting *ChannelAffinitySetting) {
	if setting == nil {
		return
	}
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		if !strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli trace") {
			continue
		}
		appendMissingString(&rule.PathRegex, "/v1/messages?")
		ensureCodexCliChannelAffinityKeySources(&rule.KeySources)
		ensureCodexCliParamOverrideTemplate(&rule.ParamOverrideTemplate)
	}
}

func ensureCodexCliParamOverrideTemplate(template *map[string]interface{}) {
	if template == nil {
		return
	}
	if *template == nil {
		*template = buildCodexCliParamOverrideTemplate()
		return
	}

	operations := cloneParamOverrideOperations((*template)["operations"])
	if hasPromptCacheSessionSyncOperation(operations) {
		(*template)["operations"] = operations
		return
	}
	operations = append(operations, codexCliPromptCacheSessionSyncOperation())
	(*template)["operations"] = operations
}

func cloneParamOverrideOperations(value interface{}) []map[string]interface{} {
	switch operations := value.(type) {
	case []map[string]interface{}:
		cloned := make([]map[string]interface{}, 0, len(operations))
		for _, operation := range operations {
			cloned = append(cloned, cloneParamOverrideOperation(operation))
		}
		return cloned
	case []interface{}:
		cloned := make([]map[string]interface{}, 0, len(operations))
		for _, operation := range operations {
			if operationMap, ok := operation.(map[string]interface{}); ok {
				cloned = append(cloned, cloneParamOverrideOperation(operationMap))
			}
		}
		return cloned
	default:
		return []map[string]interface{}{}
	}
}

func cloneParamOverrideOperation(operation map[string]interface{}) map[string]interface{} {
	cloned := make(map[string]interface{}, len(operation))
	for key, value := range operation {
		cloned[key] = value
	}
	return cloned
}

func hasPromptCacheSessionSyncOperation(operations []map[string]interface{}) bool {
	for _, operation := range operations {
		if !strings.EqualFold(operationString(operation["mode"]), "sync_fields") {
			continue
		}
		from := normalizeParamOverrideTarget(operationString(operation["from"]))
		to := normalizeParamOverrideTarget(operationString(operation["to"]))
		if (from == "json:prompt_cache_key" && to == "header:session_id") ||
			(from == "header:session_id" && to == "json:prompt_cache_key") {
			return true
		}
	}
	return false
}

func operationString(value interface{}) string {
	if str, ok := value.(string); ok {
		return strings.TrimSpace(str)
	}
	return ""
}

func normalizeParamOverrideTarget(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func ensureCodexCliChannelAffinityKeySources(values *[]ChannelAffinityKeySource) {
	if values == nil {
		return
	}
	for _, source := range codexCliChannelAffinityKeySourceOrder {
		appendMissingKeySource(values, source)
	}
	existing := *values
	ordered := make([]ChannelAffinityKeySource, 0, len(existing))
	used := make([]bool, len(existing))
	for _, expected := range codexCliChannelAffinityKeySourceOrder {
		for i, source := range existing {
			if used[i] || !channelAffinityKeySourceEqual(source, expected) {
				continue
			}
			ordered = append(ordered, expected)
			used[i] = true
			break
		}
	}
	for i, source := range existing {
		if used[i] {
			continue
		}
		ordered = append(ordered, source)
	}
	*values = ordered
}

func appendMissingString(values *[]string, value string) {
	if values == nil || strings.TrimSpace(value) == "" {
		return
	}
	for _, existing := range *values {
		if strings.EqualFold(strings.TrimSpace(existing), strings.TrimSpace(value)) {
			return
		}
	}
	*values = append(*values, value)
}

func appendMissingKeySource(values *[]ChannelAffinityKeySource, value ChannelAffinityKeySource) {
	if values == nil || (strings.TrimSpace(value.Type) == "" && strings.TrimSpace(value.Key) == "" && strings.TrimSpace(value.Path) == "") {
		return
	}
	for _, existing := range *values {
		if channelAffinityKeySourceEqual(existing, value) {
			return
		}
	}
	*values = append(*values, value)
}

func channelAffinityKeySourceEqual(left ChannelAffinityKeySource, right ChannelAffinityKeySource) bool {
	return strings.EqualFold(strings.TrimSpace(left.Type), strings.TrimSpace(right.Type)) &&
		strings.EqualFold(strings.TrimSpace(left.Key), strings.TrimSpace(right.Key)) &&
		strings.EqualFold(strings.TrimSpace(left.Path), strings.TrimSpace(right.Path))
}
