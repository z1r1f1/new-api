package service

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
)

func TestClaudeToOpenAIRequestPreservesServiceTierAndOutputConfigEffort(t *testing.T) {
	openAIReq, err := ClaudeToOpenAIRequest(dto.ClaudeRequest{
		Model:        "gpt-5.5",
		ServiceTier:  "priority",
		OutputConfig: []byte(`{"effort":"medium"}`),
		Messages: []dto.ClaudeMessage{{
			Role:    "user",
			Content: "hi",
		}},
	}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex}, OriginModelName: "gpt-5.5"})
	if err != nil {
		t.Fatalf("ClaudeToOpenAIRequest returned error: %v", err)
	}

	if openAIReq.ReasoningEffort != "medium" {
		t.Fatalf("expected reasoning_effort medium, got %q", openAIReq.ReasoningEffort)
	}
	var serviceTier string
	if err := common.Unmarshal(openAIReq.ServiceTier, &serviceTier); err != nil || serviceTier != "priority" {
		t.Fatalf("expected service_tier priority, got %q err=%v", serviceTier, err)
	}
}

func TestClaudeMessagesToResponsesNormalizesToolSchema(t *testing.T) {
	openAIReq, err := ClaudeToOpenAIRequest(dto.ClaudeRequest{
		Model: "gpt-5.5",
		Tools: []dto.Tool{
			{
				Name:        "Read",
				Description: "Read a file",
				InputSchema: map[string]any{
					"type":     "object",
					"required": nil,
					"properties": map[string]any{
						"file_path": map[string]any{"type": "string"},
						"offset":    map[string]any{"type": "integer"},
						"limit":     map[string]any{"type": "integer"},
					},
				},
			},
		},
		Messages: []dto.ClaudeMessage{{
			Role:    "user",
			Content: "read file",
		}},
	}, &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{ChannelType: constant.ChannelTypeCodex}, OriginModelName: "gpt-5.5"})
	if err != nil {
		t.Fatalf("ClaudeToOpenAIRequest returned error: %v", err)
	}

	responsesReq, err := ChatCompletionsRequestToResponsesRequest(openAIReq)
	if err != nil {
		t.Fatalf("ChatCompletionsRequestToResponsesRequest returned error: %v", err)
	}

	var tools []map[string]any
	if err := common.Unmarshal(responsesReq.Tools, &tools); err != nil {
		t.Fatalf("failed to decode responses tools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("expected one responses tool, got %d", len(tools))
	}
	params, ok := tools[0]["parameters"].(map[string]any)
	if !ok {
		t.Fatalf("expected parameters object, got %#v", tools[0]["parameters"])
	}
	if _, exists := params["required"]; exists {
		t.Fatalf("expected nil required field to be removed, got %#v", params["required"])
	}
	if params["additionalProperties"] != false {
		t.Fatalf("expected Claude tool schema to reject extra parameters after Responses conversion, got %#v", params["additionalProperties"])
	}
}
