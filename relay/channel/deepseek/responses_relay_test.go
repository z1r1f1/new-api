package deepseek

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func resetDeepSeekResponsesSessionsForTest() {
	deepSeekResponsesSessionStore.Lock()
	defer deepSeekResponsesSessionStore.Unlock()
	deepSeekResponsesSessionStore.Entries = make(map[string]deepSeekResponsesSessionEntry)
	deepSeekResponsesSessionStore.Order = nil
}

func newDeepSeekResponsesTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set(common.RequestIdKey, "test-request")
	return c, recorder
}

func newDeepSeekResponsesRelayInfo(stream bool) *relaycommon.RelayInfo {
	return &relaycommon.RelayInfo{
		RelayFormat:     types.RelayFormatOpenAIResponses,
		RelayMode:       relayconstant.RelayModeResponses,
		IsStream:        stream,
		OriginModelName: "deepseek-chat",
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "deepseek-chat",
		},
	}
}

func TestConvertOpenAIResponsesRequestToDeepSeekChat(t *testing.T) {
	resetDeepSeekResponsesSessionsForTest()
	c, _ := newDeepSeekResponsesTestContext()
	stream := true
	maxOutputTokens := uint(16)
	info := newDeepSeekResponsesRelayInfo(stream)

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
		Model:           "deepseek-chat",
		Input:           []byte(`"hello"`),
		Instructions:    []byte(`"be concise"`),
		Stream:          &stream,
		MaxOutputTokens: &maxOutputTokens,
		Tools: []byte(`[
			{"type":"function","name":"read_file","description":"read","parameters":{"type":"object"}},
			{"type":"namespace","name":"mcp__fs__","tools":[{"type":"function","name":"list","parameters":{"type":"object"}}]},
			{"type":"web_search_preview"}
		]`),
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}
	chatReq, ok := converted.(*dto.GeneralOpenAIRequest)
	if !ok {
		t.Fatalf("converted request type = %T, want *dto.GeneralOpenAIRequest", converted)
	}
	if len(chatReq.Messages) != 2 {
		t.Fatalf("messages len = %d, want 2: %+v", len(chatReq.Messages), chatReq.Messages)
	}
	if chatReq.Messages[0].Role != "system" || chatReq.Messages[0].StringContent() != "be concise" {
		t.Fatalf("unexpected system message: %+v", chatReq.Messages[0])
	}
	if chatReq.Messages[1].Role != "user" || chatReq.Messages[1].StringContent() != "hello" {
		t.Fatalf("unexpected user message: %+v", chatReq.Messages[1])
	}
	if chatReq.StreamOptions == nil || !chatReq.StreamOptions.IncludeUsage {
		t.Fatalf("expected stream_options.include_usage=true, got %+v", chatReq.StreamOptions)
	}
	if len(chatReq.Tools) != 2 {
		t.Fatalf("tools len = %d, want 2: %+v", len(chatReq.Tools), chatReq.Tools)
	}
	if chatReq.Tools[0].Function.Name != "read_file" || chatReq.Tools[1].Function.Name != "mcp__fs__list" {
		t.Fatalf("unexpected tool names: %+v", chatReq.Tools)
	}
	if _, exists := c.Get(deepSeekResponsesRelayContextKey); !exists {
		t.Fatal("expected relay state to be stored on gin context")
	}
}

func TestDeepSeekResponsesBlockingConvertsChatResponseAndSavesHistory(t *testing.T) {
	resetDeepSeekResponsesSessionsForTest()
	c, recorder := newDeepSeekResponsesTestContext()
	stream := false
	info := newDeepSeekResponsesRelayInfo(stream)

	converted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
		Model:  "deepseek-chat",
		Input:  []byte(`"hello"`),
		Stream: &stream,
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}
	if converted == nil {
		t.Fatal("converted request is nil")
	}

	chatBody := `{"id":"chatcmpl_1","object":"chat.completion","created":123,"model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":"hi there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
	usageAny, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(chatBody)),
	}, info)
	if apiErr != nil {
		t.Fatalf("DoResponse returned error: %v", apiErr)
	}
	usage := usageAny.(*dto.Usage)
	if usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}

	var payload map[string]any
	if err := common.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode response payload: %v\n%s", err, recorder.Body.String())
	}
	responseID, _ := payload["id"].(string)
	if !strings.HasPrefix(responseID, "resp_") {
		t.Fatalf("unexpected response id: %q", responseID)
	}
	if payload["object"] != "response" || payload["status"] != "completed" {
		t.Fatalf("unexpected response metadata: %#v", payload)
	}
	if !strings.Contains(recorder.Body.String(), `"type":"output_text"`) || !strings.Contains(recorder.Body.String(), "hi there") {
		t.Fatalf("expected output_text response, got:\n%s", recorder.Body.String())
	}

	history := getDeepSeekResponsesHistory(responseID)
	if len(history) != 2 {
		t.Fatalf("history len = %d, want 2: %+v", len(history), history)
	}
	if history[1].Role != "assistant" || history[1].StringContent() != "hi there" {
		t.Fatalf("unexpected saved assistant history: %+v", history[1])
	}

	nextCtx, _ := newDeepSeekResponsesTestContext()
	nextConverted, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(nextCtx, info, dto.OpenAIResponsesRequest{
		Model:              "deepseek-chat",
		Input:              []byte(`"continue"`),
		PreviousResponseID: responseID,
		Stream:             &stream,
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest with previous_response_id returned error: %v", err)
	}
	nextChatReq := nextConverted.(*dto.GeneralOpenAIRequest)
	if len(nextChatReq.Messages) != 3 {
		t.Fatalf("continued messages len = %d, want 3: %+v", len(nextChatReq.Messages), nextChatReq.Messages)
	}
	if nextChatReq.Messages[1].Role != "assistant" || nextChatReq.Messages[1].StringContent() != "hi there" {
		t.Fatalf("continued request did not replay assistant history: %+v", nextChatReq.Messages)
	}
}

func TestDeepSeekResponsesBlockingConvertsToolCall(t *testing.T) {
	resetDeepSeekResponsesSessionsForTest()
	c, recorder := newDeepSeekResponsesTestContext()
	stream := false
	info := newDeepSeekResponsesRelayInfo(stream)
	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
		Model:  "deepseek-chat",
		Input:  []byte(`"call a tool"`),
		Stream: &stream,
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	chatBody := `{"id":"chatcmpl_1","object":"chat.completion","created":123,"model":"deepseek-chat","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"mcp__fs__list","arguments":"{\"path\":\".\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`
	_, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(chatBody)),
	}, info)
	if apiErr != nil {
		t.Fatalf("DoResponse returned error: %v", apiErr)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		`"type":"function_call"`,
		`"namespace":"mcp__fs__"`,
		`"name":"list"`,
		`"call_id":"call_1"`,
		`"arguments":"{\"path\":\".\"}"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected response body to contain %q, got:\n%s", want, body)
		}
	}
}

func TestDeepSeekResponsesStreamConvertsChatSSE(t *testing.T) {
	resetDeepSeekResponsesSessionsForTest()
	originalTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = originalTimeout })

	c, recorder := newDeepSeekResponsesTestContext()
	stream := true
	info := newDeepSeekResponsesRelayInfo(stream)
	_, err := (&Adaptor{}).ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest{
		Model:  "deepseek-chat",
		Input:  []byte(`"hello"`),
		Stream: &stream,
	})
	if err != nil {
		t.Fatalf("ConvertOpenAIResponsesRequest returned error: %v", err)
	}

	streamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":123,"model":"deepseek-chat","choices":[{"index":0,"delta":{"content":"hi"},"finish_reason":null}]}`,
		``,
		`data: {"id":"chatcmpl_1","object":"chat.completion.chunk","created":123,"model":"deepseek-chat","choices":[],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`,
		``,
		`data: [DONE]`,
		``,
	}, "\n")
	usageAny, apiErr := (&Adaptor{}).DoResponse(c, &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(streamBody)),
	}, info)
	if apiErr != nil {
		t.Fatalf("DoResponse returned error: %v", apiErr)
	}
	usage := usageAny.(*dto.Usage)
	if usage.PromptTokens != 3 || usage.CompletionTokens != 2 || usage.TotalTokens != 5 {
		t.Fatalf("unexpected usage: %+v", usage)
	}
	body := recorder.Body.String()
	for _, want := range []string{
		"event: response.created",
		"event: response.output_item.added",
		"event: response.output_text.delta",
		"event: response.output_item.done",
		"event: response.completed",
		`"delta":"hi"`,
		`"total_tokens":5`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("expected stream body to contain %q, got:\n%s", want, body)
		}
	}
}
