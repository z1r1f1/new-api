package operation_setting

import "testing"

func TestEnsureChannelAffinitySettingCompatibilityExtendsLegacyCodexRule(t *testing.T) {
	setting := &ChannelAffinitySetting{Rules: []ChannelAffinityRule{{
		Name:       "codex cli trace",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []ChannelAffinityKeySource{{Type: "gjson", Path: "prompt_cache_key"}},
	}}}

	ensureChannelAffinitySettingCompatibility(setting)

	rule := setting.Rules[0]
	if !containsStringFold(rule.PathRegex, "/v1/messages?") {
		t.Fatalf("expected /v1/messages? path to be appended, got %#v", rule.PathRegex)
	}
	wantSources := []ChannelAffinityKeySource{
		{Type: "gjson", Path: "openai_prompt_cache_key"},
		{Type: "gjson", Path: "session_id"},
		{Type: "gjson", Path: "request_session_id"},
		{Type: "gjson", Path: "metadata.prompt_cache_key"},
		{Type: "gjson", Path: "metadata.openai_prompt_cache_key"},
		{Type: "gjson", Path: "metadata.codex_session_id"},
		{Type: "gjson", Path: "metadata.claudeSessionId"},
		{Type: "gjson", Path: "metadata.user_id"},
		{Type: "request_header", Key: "Session_id"},
		{Type: "request_header", Key: "X-Claude-Session-Id"},
		{Type: "request_header", Key: "X-Claude-Code-Session-Id"},
		{Type: "request_header", Key: "X-Codex-Session-Id"},
	}
	for _, source := range wantSources {
		if !containsKeySource(rule.KeySources, source) {
			t.Fatalf("expected key source %#v to be appended, got %#v", source, rule.KeySources)
		}
	}
	if !containsPromptCacheSessionSync(rule.ParamOverrideTemplate) {
		t.Fatalf("expected codex param override template to sync prompt_cache_key to Session_id, got %#v", rule.ParamOverrideTemplate)
	}

	pathCount := len(setting.Rules[0].PathRegex)
	keySourceCount := len(setting.Rules[0].KeySources)
	operationCount := len(cloneParamOverrideOperations(setting.Rules[0].ParamOverrideTemplate["operations"]))
	ensureChannelAffinitySettingCompatibility(setting)
	if len(setting.Rules[0].PathRegex) != pathCount ||
		len(setting.Rules[0].KeySources) != keySourceCount ||
		len(cloneParamOverrideOperations(setting.Rules[0].ParamOverrideTemplate["operations"])) != operationCount {
		t.Fatalf("compatibility should be idempotent, got %#v", setting.Rules[0])
	}
}

func TestBuildCodexCliParamOverrideTemplateSyncsPromptCacheKeyToSessionID(t *testing.T) {
	template := buildCodexCliParamOverrideTemplate()

	if !containsPromptCacheSessionSync(template) {
		t.Fatalf("expected default codex template to include prompt_cache_key/session_id sync, got %#v", template)
	}
}

func TestEnsureChannelAffinitySettingCompatibilityPreservesLegacyOpsAndAppendsSync(t *testing.T) {
	setting := &ChannelAffinitySetting{Rules: []ChannelAffinityRule{{
		Name:       "codex cli trace",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []ChannelAffinityKeySource{{Type: "gjson", Path: "prompt_cache_key"}},
		ParamOverrideTemplate: map[string]interface{}{
			"operations": []interface{}{
				map[string]interface{}{
					"mode":        "pass_headers",
					"value":       []interface{}{"Session_id"},
					"keep_origin": true,
				},
			},
		},
	}}}

	ensureChannelAffinitySettingCompatibility(setting)

	operations := cloneParamOverrideOperations(setting.Rules[0].ParamOverrideTemplate["operations"])
	if len(operations) != 2 {
		t.Fatalf("expected legacy pass_headers plus sync_fields operation, got %#v", operations)
	}
	if operationString(operations[0]["mode"]) != "pass_headers" {
		t.Fatalf("expected legacy operation to be preserved first, got %#v", operations)
	}
	if !containsPromptCacheSessionSync(setting.Rules[0].ParamOverrideTemplate) {
		t.Fatalf("expected sync operation to be appended, got %#v", setting.Rules[0].ParamOverrideTemplate)
	}
}

func TestEnsureChannelAffinitySettingCompatibilityOrdersCodexRuleLikeProxyPromptCacheExtraction(t *testing.T) {
	setting := &ChannelAffinitySetting{Rules: []ChannelAffinityRule{{
		Name:       "codex cli trace",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []ChannelAffinityKeySource{{Type: "gjson", Path: "prompt_cache_key"}},
	}}}

	ensureChannelAffinitySettingCompatibility(setting)

	wantPrefix := []ChannelAffinityKeySource{
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
	if len(setting.Rules[0].KeySources) < len(wantPrefix) {
		t.Fatalf("expected at least %d key sources, got %#v", len(wantPrefix), setting.Rules[0].KeySources)
	}
	for i, want := range wantPrefix {
		got := setting.Rules[0].KeySources[i]
		if got != want {
			t.Fatalf("expected key source %d to be %#v, got %#v in %#v", i, want, got, setting.Rules[0].KeySources)
		}
	}
}

func containsStringFold(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func containsKeySource(values []ChannelAffinityKeySource, target ChannelAffinityKeySource) bool {
	for _, value := range values {
		if value.Type == target.Type && value.Key == target.Key && value.Path == target.Path {
			return true
		}
	}
	return false
}

func containsPromptCacheSessionSync(template map[string]interface{}) bool {
	if template == nil {
		return false
	}
	return hasPromptCacheSessionSyncOperation(cloneParamOverrideOperations(template["operations"]))
}
