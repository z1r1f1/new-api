package openaicompat

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
)

func TestChatCompletionsRequestToResponsesRequestPreservesCacheTierAndEffort(t *testing.T) {
	serviceTier, _ := common.Marshal("priority")
	retention, _ := common.Marshal("24h")
	safetyIdentifier, _ := common.Marshal("safe-user")
	req := &dto.GeneralOpenAIRequest{
		Model:                "gpt-5.5",
		Messages:             []dto.Message{{Role: "user", Content: "hi"}},
		ReasoningEffort:      "medium",
		ServiceTier:          serviceTier,
		PromptCacheKey:       "session-123",
		PromptCacheRetention: retention,
		SafetyIdentifier:     safetyIdentifier,
		StreamOptions:        &dto.StreamOptions{IncludeUsage: true},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	if out.ServiceTier != "priority" {
		t.Fatalf("expected service tier priority, got %q", out.ServiceTier)
	}
	var promptCacheKey string
	if err := common.Unmarshal(out.PromptCacheKey, &promptCacheKey); err != nil || promptCacheKey != "session-123" {
		t.Fatalf("expected prompt_cache_key session-123, got %q err=%v", promptCacheKey, err)
	}
	if string(out.PromptCacheRetention) != string(retention) {
		t.Fatalf("expected prompt cache retention to be preserved, got %s", out.PromptCacheRetention)
	}
	if string(out.SafetyIdentifier) != string(safetyIdentifier) {
		t.Fatalf("expected safety identifier to be preserved, got %s", out.SafetyIdentifier)
	}
	if out.StreamOptions == nil || !out.StreamOptions.IncludeUsage {
		t.Fatalf("expected stream_options.include_usage to be preserved, got %#v", out.StreamOptions)
	}
	if out.Reasoning == nil || out.Reasoning.Effort != "medium" {
		t.Fatalf("expected reasoning effort medium, got %#v", out.Reasoning)
	}
}

func TestChatCompletionsRequestToResponsesRequestNormalizesLiteralFastServiceTier(t *testing.T) {
	serviceTier, _ := common.Marshal("fast")
	req := &dto.GeneralOpenAIRequest{
		Model:       "gpt-5.5",
		Messages:    []dto.Message{{Role: "user", Content: "hi"}},
		ServiceTier: serviceTier,
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}
	if out.ServiceTier != "priority" {
		t.Fatalf("expected literal fast service_tier to normalize to priority, got %q", out.ServiceTier)
	}
}

func TestChatCompletionsRequestToResponsesRequestClosesFunctionToolParameters(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-5.5",
		Messages: []dto.Message{{Role: "user", Content: "read file"}},
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        "Read",
					Description: "Read a file",
					Parameters: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"file_path": map[string]any{"type": "string"},
							"offset":    map[string]any{"type": "integer"},
							"limit":     map[string]any{"type": "integer"},
						},
						"required": []any{"file_path"},
					},
				},
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	var tools []map[string]any
	if err := common.Unmarshal(out.Tools, &tools); err != nil {
		t.Fatalf("failed to decode tools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected one tool, got %d", len(tools))
	}
	params, ok := tools[0]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected parameters object, got %#v", tools[0]["parameters"])
	}
	if params["additionalProperties"] != false {
		t.Fatalf("expected converted tool schema to reject extra parameters, got %#v", params["additionalProperties"])
	}
}

func TestChatCompletionsRequestToResponsesRequestNormalizesFunctionToolParameters(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-5.5",
		Messages: []dto.Message{{Role: "user", Content: "read file"}},
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:        "Read",
					Description: "Read a file",
					Parameters: map[string]any{
						"type":     "object",
						"required": nil,
						"properties": map[string]any{
							"file_path": map[string]any{"type": "string", "default": nil},
							"options": map[string]any{
								"type":     "object",
								"required": "invalid",
								"properties": map[string]any{
									"limit": map[string]any{"type": "integer"},
								},
							},
						},
					},
				},
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	var tools []map[string]any
	if err := common.Unmarshal(out.Tools, &tools); err != nil {
		t.Fatalf("failed to decode tools: %v", err)
	}
	params, ok := tools[0]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected parameters object, got %#v", tools[0]["parameters"])
	}
	if _, exists := params["required"]; exists {
		t.Fatalf("expected nil required field to be removed, got %#v", params["required"])
	}
	properties, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties object, got %#v", params["properties"])
	}
	filePath, ok := properties["file_path"].(map[string]any)
	if !ok {
		t.Fatalf("expected file_path schema object, got %#v", properties["file_path"])
	}
	if _, exists := filePath["default"]; exists {
		t.Fatalf("expected nil nested default field to be removed, got %#v", filePath["default"])
	}
	options, ok := properties["options"].(map[string]any)
	if !ok {
		t.Fatalf("expected options schema object, got %#v", properties["options"])
	}
	if _, exists := options["required"]; exists {
		t.Fatalf("expected invalid nested required field to be removed, got %#v", options["required"])
	}
	if options["additionalProperties"] != false {
		t.Fatalf("expected nested object schema to reject extra parameters, got %#v", options["additionalProperties"])
	}
}

func TestChatCompletionsRequestToResponsesRequestDefaultsInvalidFunctionToolParameters(t *testing.T) {
	req := &dto.GeneralOpenAIRequest{
		Model:    "gpt-5.5",
		Messages: []dto.Message{{Role: "user", Content: "run tool"}},
		Tools: []dto.ToolCallRequest{
			{
				Type: "function",
				Function: dto.FunctionRequest{
					Name:       "NoArgs",
					Parameters: []any{"invalid"},
				},
			},
		},
	}

	out, err := ChatCompletionsRequestToResponsesRequest(req)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	var tools []map[string]any
	if err := common.Unmarshal(out.Tools, &tools); err != nil {
		t.Fatalf("failed to decode tools: %v", err)
	}
	params, ok := tools[0]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected invalid function parameters to become an object schema, got %#v", tools[0]["parameters"])
	}
	if params["type"] != "object" {
		t.Fatalf("expected default object schema type, got %#v", params["type"])
	}
	if params["additionalProperties"] != false {
		t.Fatalf("expected default object schema to reject extra parameters, got %#v", params["additionalProperties"])
	}
}
