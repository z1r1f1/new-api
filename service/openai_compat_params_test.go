package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestApplyOpenAICompatRequestParamsFromRawBodyMapsFastEffortAndCacheKey(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}
	body := []byte(`{
		"fast": true,
		"think_effort": "high",
		"metadata": {
			"user_id": "{\"device_id\":\"device-1\",\"session_id\":\"session-123\"}"
		}
	}`)

	ApplyOpenAICompatRequestParamsFromRawBody(req, body, nil)

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "priority" {
		t.Fatalf("expected service_tier priority, got %q", got)
	}
	if req.ReasoningEffort != "high" {
		t.Fatalf("expected effort high, got %q", req.ReasoningEffort)
	}
	if req.PromptCacheKey != "session-123" {
		t.Fatalf("expected derived session prompt_cache_key, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyNormalizesLiteralFastServiceTier(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}
	body := []byte(`{"service_tier":"fast"}`)

	ApplyOpenAICompatRequestParamsFromRawBody(req, body, nil)

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "priority" {
		t.Fatalf("expected literal fast service_tier to normalize to priority, got %q", got)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyUsesHeadersWhenBodyOmitsCacheKey(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5"}`), map[string]string{
		"Session_id":                            "header-session",
		"X-Claude-Code-Proxy-Fast":              "true",
		"X-Claude-Code-Proxy-Fast-Service-Tier": "priority",
	})

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "priority" {
		t.Fatalf("expected service_tier priority from fast header, got %q", got)
	}
	if req.PromptCacheKey != "header-session" {
		t.Fatalf("expected prompt_cache_key from Session_id header, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyUsesClaudeCodeSessionHeaderFallback(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5"}`), map[string]string{
		"X-Claude-Code-Session-Id": "claude-code-session",
	})

	if req.PromptCacheKey != "claude-code-session" {
		t.Fatalf("expected prompt_cache_key from X-Claude-Code-Session-Id header, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyUsesProxyFastServiceTierHeader(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5"}`), map[string]string{
		"X-Claude-Code-Proxy-Fast-Service-Tier": "priority",
	})

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "priority" {
		t.Fatalf("expected service_tier priority from proxy fast service tier header, got %q", got)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyDefaultsClaudeCodeCLIToPriority(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5"}`), map[string]string{
		"User-Agent": "claude-cli/1.0.0",
	})

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "priority" {
		t.Fatalf("expected Claude Code CLI default service_tier priority, got %q", got)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyDefaultsClaudeCodeSessionHeaderToPriority(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5"}`), map[string]string{
		"X-Claude-Code-Session-Id": "claude-session-123",
	})

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "priority" {
		t.Fatalf("expected Claude Code session header to default service_tier priority, got %q", got)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyExplicitFastFalseDisablesClaudeCodeDefault(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5","fast":false}`), map[string]string{
		"User-Agent": "claude-code/2.0.0",
	})

	if got := normalizeServiceTierRaw(req.ServiceTier); got != "" {
		t.Fatalf("expected explicit fast false to disable Claude Code default service_tier, got %q", got)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyUsesSessionAliasesForCacheKey(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{
		"model":"gpt-5.5",
		"session_id":"top-session",
		"metadata":{"codex_session_id":"metadata-session"}
	}`), nil)

	if req.PromptCacheKey != "top-session" {
		t.Fatalf("expected prompt_cache_key from top-level session_id, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyMatchesProxyPromptCachePriority(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{
		"model":"gpt-5.5",
		"session_id":"top-session",
		"metadata":{
			"openai_prompt_cache_key":"metadata-cache",
			"user_id":"{\"session_id\":\"metadata-user-session\"}"
		}
	}`), nil)

	if req.PromptCacheKey != "metadata-cache" {
		t.Fatalf("expected metadata prompt_cache_key to match claude-code-proxy priority, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyUsesMetadataUserIDBeforeSessionFallback(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}

	ApplyOpenAICompatRequestParamsFromRawBody(req, []byte(`{
		"model":"gpt-5.5",
		"session_id":"top-session",
		"metadata":{
			"user_id":"{\"device_id\":\"device-1\",\"session_id\":\"metadata-user-session\"}"
		}
	}`), nil)

	if req.PromptCacheKey != "metadata-user-session" {
		t.Fatalf("expected metadata.user_id session to match claude-code-proxy priority, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAICompatRequestParamsFromRawBodyDoesNotDeriveThreadIDFromMetadataUserID(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{Model: "gpt-5.5"}
	userID := `{"device_id":"device-1","thread_id":"thread-only"}`
	body, err := common.Marshal(map[string]interface{}{
		"model": "gpt-5.5",
		"metadata": map[string]interface{}{
			"user_id": userID,
		},
	})
	if err != nil {
		t.Fatalf("failed to marshal test body: %v", err)
	}

	ApplyOpenAICompatRequestParamsFromRawBody(req, body, nil)

	if req.PromptCacheKey != userID {
		t.Fatalf("expected metadata.user_id fallback to keep full user_id like claude-code-proxy, got %q", req.PromptCacheKey)
	}
}

func TestApplyOpenAIResponsesCompatRequestParamsFromRawBodyMapsAliases(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{Model: "gpt-5.5", ServiceTier: "fast"}
	body := []byte(`{"metadata":{"openai_prompt_cache_key":"metadata-cache"},"output_config":{"effort":"medium"}}`)

	ApplyOpenAIResponsesCompatRequestParamsFromRawBody(req, body, nil)

	if req.ServiceTier != "priority" {
		t.Fatalf("expected service_tier priority, got %q", req.ServiceTier)
	}
	if req.Reasoning == nil || req.Reasoning.Effort != "medium" {
		t.Fatalf("expected reasoning effort medium, got %#v", req.Reasoning)
	}
	var promptCacheKey string
	if err := common.Unmarshal(req.PromptCacheKey, &promptCacheKey); err != nil || promptCacheKey != "metadata-cache" {
		t.Fatalf("expected prompt_cache_key metadata-cache, got %q err=%v", promptCacheKey, err)
	}
}

func TestApplyOpenAIResponsesCompatRequestParamsFromRawBodyDefaultsClaudeCodeCLIToPriority(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{Model: "gpt-5.5"}

	ApplyOpenAIResponsesCompatRequestParamsFromRawBody(req, []byte(`{"model":"gpt-5.5"}`), map[string]string{
		"User-Agent": "claude_code/1.0.0",
	})

	if req.ServiceTier != "priority" {
		t.Fatalf("expected Claude Code CLI default service_tier priority, got %q", req.ServiceTier)
	}
}

func TestApplyOpenAIResponsesCompatRequestParamsFromRawBodyMatchesProxyPromptCachePriority(t *testing.T) {
	req := &dto.OpenAIResponsesRequest{Model: "gpt-5.5"}
	body := []byte(`{
		"session_id":"top-session",
		"metadata":{
			"prompt_cache_key":"metadata-cache",
			"user_id":"{\"session_id\":\"metadata-user-session\"}"
		}
	}`)

	ApplyOpenAIResponsesCompatRequestParamsFromRawBody(req, body, nil)

	var promptCacheKey string
	if err := common.Unmarshal(req.PromptCacheKey, &promptCacheKey); err != nil || promptCacheKey != "metadata-cache" {
		t.Fatalf("expected metadata prompt_cache_key to match claude-code-proxy priority, got %q err=%v", promptCacheKey, err)
	}
}
