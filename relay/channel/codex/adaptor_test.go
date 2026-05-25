package codex

import (
	"net/http/httptest"
	"testing"

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

func TestConvertOpenAIResponsesRequestPreservesExplicitFalseStreamForCodex(t *testing.T) {
	stream := false
	info := &relaycommon.RelayInfo{
		RelayMode:   relayconstant.RelayModeResponses,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nil, info, dto.OpenAIResponsesRequest{Model: "gpt-5.5", Stream: &stream})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	req := converted.(dto.OpenAIResponsesRequest)
	if req.Stream == nil || *req.Stream {
		t.Fatalf("expected explicit stream=false to be preserved, got %#v", req.Stream)
	}
	if info.IsStream {
		t.Fatal("expected explicit stream=false not to mark relay info as streaming")
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
