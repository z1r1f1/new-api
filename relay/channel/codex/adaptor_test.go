package codex

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"

	"github.com/gin-gonic/gin"
)

func TestConvertOpenAIResponsesRequestDefaultsStreamForCodex(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: "gpt-5.5"})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req, ok := converted.(dto.OpenAIResponsesRequest)
	if !ok {
		t.Fatalf("expected dto.OpenAIResponsesRequest, got %T", converted)
	}
	if req.Stream == nil || !*req.Stream {
		t.Fatalf("expected omitted stream to default to true, got %#v", req.Stream)
	}
	if !info.IsStream {
		t.Fatal("expected relay info to be marked as streaming")
	}
	if got, exists := c.Get(string(constant.ContextKeyIsStream)); !exists || got != true {
		t.Fatalf("expected context stream flag true, got %v exists=%v", got, exists)
	}
}

func TestConvertOpenAIResponsesRequestDropsUnsupportedStreamOptionsForCodex(t *testing.T) {
	stream := true
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model:         "gpt-5.5",
		Stream:        &stream,
		StreamOptions: &dto.StreamOptions{IncludeUsage: true},
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req := converted.(dto.OpenAIResponsesRequest)
	if req.StreamOptions != nil {
		t.Fatalf("expected codex request to drop unsupported stream_options, got %#v", req.StreamOptions)
	}
}

func TestConvertOpenAIResponsesRequestForcesExplicitFalseStreamForCodex(t *testing.T) {
	stream := false
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{Model: "gpt-5.5", Stream: &stream})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req := converted.(dto.OpenAIResponsesRequest)
	if req.Stream == nil || !*req.Stream {
		t.Fatalf("expected explicit stream=false to be forced to true for codex, got %#v", req.Stream)
	}
	if !info.IsStream {
		t.Fatal("expected forced stream=true to mark relay info as streaming")
	}
	if got, exists := c.Get(string(constant.ContextKeyIsStream)); !exists || got != true {
		t.Fatalf("expected context stream flag true, got %v exists=%v", got, exists)
	}
}

func TestConvertOpenAIResponsesRequestDoesNotDefaultStreamForCompact(t *testing.T) {
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponsesCompact,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{Model: "gpt-5.5-compact"})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req := converted.(dto.OpenAIResponsesRequest)
	if req.Stream != nil {
		t.Fatalf("expected compact request stream to stay omitted, got %#v", req.Stream)
	}
	if info.IsStream {
		t.Fatal("expected compact request not to mark relay info as streaming")
	}
}

func TestConvertOpenAIResponsesRequestNormalizesReadToolSchemaForCodex(t *testing.T) {
	tools, _ := common.Marshal([]map[string]any{
		{
			"type": "function",
			"name": "Read",
			"parameters": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"file_path": map[string]any{"type": "string"},
					"offset":    map[string]any{"type": "integer"},
					"limit":     map[string]any{"type": "integer"},
					"pages": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "integer"},
					},
				},
				"required": []any{"file_path", "pages"},
			},
		},
	})
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{
		Model: "gpt-5.5",
		Tools: tools,
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req := converted.(dto.OpenAIResponsesRequest)
	var convertedTools []map[string]any
	if err := common.Unmarshal(req.Tools, &convertedTools); err != nil {
		t.Fatalf("failed to decode tools: %v", err)
	}
	params := convertedTools[0]["parameters"].(map[string]any)
	properties := params["properties"].(map[string]any)
	if _, exists := properties["pages"]; exists {
		t.Fatalf("expected Read.pages to be removed from schema, got %#v", properties["pages"])
	}
	for _, item := range params["required"].([]any) {
		if item == "pages" {
			t.Fatalf("expected Read.pages to be removed from required list, got %#v", params["required"])
		}
	}
	if params["additionalProperties"] != false {
		t.Fatalf("expected Read tool schema to reject extra parameters, got %#v", params["additionalProperties"])
	}
}
