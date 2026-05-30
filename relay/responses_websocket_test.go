package relay

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/dto"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

func TestNormalizeResponsesWSCreateEventWrapper(t *testing.T) {
	message := []byte(`{
		"type": "response.create",
		"event_id": "evt_1",
		"generate": false,
		"response": {
			"model": "gpt-5.3-codex-spark",
			"input": "hi",
			"fast": true,
			"store": false,
			"stream": true,
			"stream_options": {"include_usage": true}
		}
	}`)

	create, eventID, err := normalizeResponsesWSCreateEvent(message)
	if err != nil {
		t.Fatalf("normalizeResponsesWSCreateEvent() error = %v", err)
	}
	req := create.Request
	if eventID != "evt_1" {
		t.Fatalf("eventID = %q, want evt_1", eventID)
	}
	if req.Model != "gpt-5.3-codex-spark" {
		t.Fatalf("model = %q", req.Model)
	}
	if strings.TrimSpace(string(create.Generate)) != "false" {
		t.Fatalf("generate = %s, want false", create.Generate)
	}
	var raw map[string]any
	if err := common.Unmarshal(create.Raw, &raw); err != nil {
		t.Fatalf("unmarshal raw request: %v", err)
	}
	if raw["fast"] != true {
		t.Fatalf("raw request should preserve compatibility fields such as fast: %s", create.Raw)
	}
	if req.Stream != nil {
		t.Fatalf("stream = %v, want nil", req.Stream)
	}
	if req.StreamOptions != nil {
		t.Fatalf("stream_options = %#v, want nil", req.StreamOptions)
	}
	if strings.TrimSpace(string(req.Store)) != "false" {
		t.Fatalf("store = %s, want false", req.Store)
	}
}

func TestNormalizeResponsesWSCreateEventFlat(t *testing.T) {
	message := []byte(`{
		"type": "response.create",
		"event_id": "evt_2",
		"model": "gpt-5.3-codex-spark",
		"input": "hi",
		"generate": false,
		"stream": true,
		"background": true,
		"stream_options": {"include_usage": true}
	}`)

	create, eventID, err := normalizeResponsesWSCreateEvent(message)
	if err != nil {
		t.Fatalf("normalizeResponsesWSCreateEvent() error = %v", err)
	}
	req := create.Request
	if eventID != "evt_2" {
		t.Fatalf("eventID = %q, want evt_2", eventID)
	}
	if req.Model != "gpt-5.3-codex-spark" {
		t.Fatalf("model = %q", req.Model)
	}
	if strings.TrimSpace(string(create.Generate)) != "false" {
		t.Fatalf("generate = %s, want false", create.Generate)
	}
	if req.Stream != nil {
		t.Fatalf("stream = %v, want nil", req.Stream)
	}
	if req.StreamOptions != nil {
		t.Fatalf("stream_options = %#v, want nil", req.StreamOptions)
	}
}

func TestBuildResponsesWSCreateEventIsFlat(t *testing.T) {
	payload := []byte(`{
		"model": "gpt-5.3-codex-spark",
		"input": "hi",
		"store": false,
		"event_id": "evt_upstream",
		"stream": true,
		"background": true,
		"stream_options": {"include_usage": true}
	}`)

	got, err := buildResponsesWSCreateEvent(payload, json.RawMessage(`false`))
	if err != nil {
		t.Fatalf("buildResponsesWSCreateEvent() error = %v", err)
	}
	var data map[string]any
	if err := common.Unmarshal(got, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if data["type"] != responsesWSEventTypeResponseCreate {
		t.Fatalf("type = %#v", data["type"])
	}
	if data["model"] != "gpt-5.3-codex-spark" || data["input"] != "hi" || data["store"] != false {
		t.Fatalf("unexpected flat event fields: %s", got)
	}
	if data["generate"] != false {
		t.Fatalf("generate = %#v, want false", data["generate"])
	}
	for _, key := range []string{"response", "event_id", "stream", "background", "stream_options"} {
		if _, ok := data[key]; ok {
			t.Fatalf("field %q should not be present in upstream event: %s", key, got)
		}
	}
}

func TestHTTPResponsesRequestDoesNotMarshalGenerate(t *testing.T) {
	var req dto.OpenAIResponsesRequest
	if err := common.Unmarshal([]byte(`{"model":"gpt-5.3-codex-spark","input":"hi","generate":false}`), &req); err != nil {
		t.Fatalf("unmarshal request: %v", err)
	}
	got, err := common.Marshal(req)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	var data map[string]any
	if err := common.Unmarshal(got, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if _, ok := data["generate"]; ok {
		t.Fatalf("generate leaked into HTTP request JSON: %s", got)
	}
}

func TestBuildResponsesWSErrorPayloadIncludesStatus(t *testing.T) {
	payload, err := buildResponsesWSErrorPayload("evt_err", types.NewErrorWithStatusCode(
		errors.New("model is required"),
		types.ErrorCodeInvalidRequest,
		http.StatusBadRequest,
		types.ErrOptionWithSkipRetry(),
	))
	if err != nil {
		t.Fatalf("buildResponsesWSErrorPayload() error = %v", err)
	}
	var data struct {
		Type    string             `json:"type"`
		Status  int                `json:"status"`
		EventID string             `json:"event_id"`
		Error   *types.OpenAIError `json:"error"`
	}
	if err := common.Unmarshal(payload, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if data.Type != "error" || data.Status != http.StatusBadRequest || data.EventID != "evt_err" {
		t.Fatalf("unexpected error event: %s", payload)
	}
	if data.Error == nil || data.Error.Code != string(types.ErrorCodeInvalidRequest) {
		t.Fatalf("unexpected error body: %#v", data.Error)
	}
}

func TestResponsesWSUpstreamUsageLimitErrorUses429(t *testing.T) {
	message := []byte(`{
		"type": "response.failed",
		"response": {
			"status": "failed",
			"error": {
				"message": "You've hit your usage limit. To continue using Codex, try again later.",
				"type": "rate_limit_error",
				"code": "usage_limit"
			}
		}
	}`)
	var streamResp dto.ResponsesStreamResponse
	if err := common.Unmarshal(message, &streamResp); err != nil {
		t.Fatalf("unmarshal stream response: %v", err)
	}

	apiErr := newResponsesWSUpstreamAPIError(streamResp, message)
	if apiErr == nil {
		t.Fatal("apiErr is nil")
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
	if !strings.Contains(apiErr.Error(), "usage limit") {
		t.Fatalf("error should include upstream detail, got %q", apiErr.Error())
	}
}

func TestResponsesWSTopLevelErrorPreservesStatus(t *testing.T) {
	message := []byte(`{
		"type": "error",
		"status": 400,
		"error": {
			"message": "Unsupported parameter: stream_options",
			"type": "invalid_request_error",
			"code": "unsupported_parameter"
		}
	}`)
	var streamResp dto.ResponsesStreamResponse
	if err := common.Unmarshal(message, &streamResp); err != nil {
		t.Fatalf("unmarshal stream response: %v", err)
	}

	apiErr := newResponsesWSUpstreamAPIError(streamResp, message)
	if apiErr == nil {
		t.Fatal("apiErr is nil")
	}
	if apiErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, http.StatusBadRequest)
	}
	if !strings.Contains(apiErr.Error(), "Unsupported parameter") {
		t.Fatalf("error should include top-level detail, got %q", apiErr.Error())
	}
}

func TestResponsesWSUsageLimitTextError(t *testing.T) {
	apiErr := newResponsesWSUsageLimitTextError("You've hit your usage limit. Try again later.")
	if apiErr == nil {
		t.Fatal("apiErr is nil")
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
}

func TestResponsesWSTextualPreviousResponseNotFoundErrorUses429(t *testing.T) {
	apiErr := newResponsesWSTextualUpstreamError("status_code=429, Previous response with id 'resp_123' not found.")
	if apiErr == nil {
		t.Fatal("apiErr is nil")
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want %d", apiErr.StatusCode, http.StatusTooManyRequests)
	}
	if !strings.Contains(apiErr.Error(), "Previous response") {
		t.Fatalf("error should include upstream detail, got %q", apiErr.Error())
	}
}

func TestResponsesWSTextualErrorDoesNotTreatPlain429MentionAsFailure(t *testing.T) {
	if apiErr := newResponsesWSTextualUpstreamError("HTTP 429 is a numeric status code in many APIs."); apiErr != nil {
		t.Fatalf("plain explanatory 429 text should not be treated as upstream failure: %v", apiErr)
	}
	if apiErr := newResponsesWSTextualUpstreamError("status_code=429, 已处理这个问题，并已经重建并重启当前容器。"); apiErr != nil {
		t.Fatalf("assistant explanation containing status_code=429 should not be treated as upstream failure: %v", apiErr)
	}
}

func TestPrepareResponsesWSRetryCreateKeepsPreviousResponseIDForStateError(t *testing.T) {
	create := responsesWSCreateRequest{
		Request: dto.OpenAIResponsesRequest{
			Model:              "gpt-5.5",
			PreviousResponseID: "resp_123",
		},
		Raw: json.RawMessage(`{"model":"gpt-5.5","previous_response_id":"resp_123","input":"hi"}`),
	}
	apiErr := newResponsesWSTextualUpstreamError("status_code=429, Previous response with id 'resp_123' not found.")

	got := prepareResponsesWSRetryCreate(create, apiErr)

	if got.Request.PreviousResponseID != "resp_123" {
		t.Fatalf("PreviousResponseID = %q, want resp_123", got.Request.PreviousResponseID)
	}
	if !strings.Contains(string(got.Raw), "previous_response_id") {
		t.Fatalf("previous_response_id should remain because it is upstream state, got: %s", got.Raw)
	}
}

func TestPrepareResponsesWSRetryCreateKeepsPreviousResponseIDForNonStateError(t *testing.T) {
	create := responsesWSCreateRequest{
		Request: dto.OpenAIResponsesRequest{
			Model:              "gpt-5.5",
			PreviousResponseID: "resp_123",
		},
		Raw: json.RawMessage(`{"model":"gpt-5.5","previous_response_id":"resp_123","input":"hi"}`),
	}
	apiErr := types.NewOpenAIError(errors.New("upstream failed"), types.ErrorCodeBadResponseStatusCode, http.StatusInternalServerError)

	got := prepareResponsesWSRetryCreate(create, apiErr)

	if got.Request.PreviousResponseID != "resp_123" {
		t.Fatalf("PreviousResponseID = %q, want resp_123", got.Request.PreviousResponseID)
	}
	if !strings.Contains(string(got.Raw), "previous_response_id") {
		t.Fatalf("previous_response_id should remain in raw retry payload: %s", got.Raw)
	}
}

func TestPrepareResponsesWSRetryCreateKeepsPreviousResponseIDForFunctionCallOutput(t *testing.T) {
	create := responsesWSCreateRequest{
		Request: dto.OpenAIResponsesRequest{
			Model:              "gpt-5.5",
			PreviousResponseID: "resp_123",
			Input:              json.RawMessage(`[{"type":"function_call_output","call_id":"call_1","output":"ok"}]`),
		},
		Raw: json.RawMessage(`{"model":"gpt-5.5","previous_response_id":"resp_123","input":[{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`),
	}
	apiErr := newResponsesWSTextualUpstreamError("status_code=429, Previous response with id 'resp_123' not found.")

	got := prepareResponsesWSRetryCreate(create, apiErr)

	if got.Request.PreviousResponseID != "resp_123" {
		t.Fatalf("function_call_output retries must keep PreviousResponseID, got %q", got.Request.PreviousResponseID)
	}
	if !strings.Contains(string(got.Raw), "previous_response_id") {
		t.Fatalf("previous_response_id should remain when input depends on a tool call: %s", got.Raw)
	}
}

func TestPrepareResponsesWSRetryCreateReplaysStoredFunctionCallOutputState(t *testing.T) {
	session := &responsesWSSession{toolCallsByResponseID: map[string][]dto.ResponsesOutput{
		"resp_123": {{Type: "function_call", ID: "fc_1", CallId: "call_1", Name: "exec_command", Arguments: json.RawMessage(`"{\"cmd\":\"pwd\"}"`)}},
	}}
	create := responsesWSCreateRequest{
		Request: dto.OpenAIResponsesRequest{
			Model:              "gpt-5.5",
			PreviousResponseID: "resp_123",
			Input:              json.RawMessage(`[{"type":"function_call_output","call_id":"call_1","output":"/tmp"}]`),
		},
		Raw: json.RawMessage(`{"model":"gpt-5.5","previous_response_id":"resp_123","input":[{"type":"function_call_output","call_id":"call_1","output":"/tmp"}]}`),
	}
	apiErr := newResponsesWSTextualUpstreamError("status_code=429, Previous response with id 'resp_123' not found.")

	got := session.prepareResponsesWSRetryCreate(create, apiErr)

	if got.Request.PreviousResponseID != "" {
		t.Fatalf("PreviousResponseID = %q, want empty after replay expansion", got.Request.PreviousResponseID)
	}
	var raw map[string]any
	if err := common.Unmarshal(got.Raw, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, ok := raw["previous_response_id"]; ok {
		t.Fatalf("previous_response_id should be removed from replay retry payload: %s", got.Raw)
	}
	items, ok := raw["input"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("input should contain replayed function_call plus output, got %#v", raw["input"])
	}
	first, _ := items[0].(map[string]any)
	if first["type"] != "function_call" || first["call_id"] != "call_1" || first["name"] != "exec_command" {
		t.Fatalf("unexpected replayed function call: %#v", first)
	}
	second, _ := items[1].(map[string]any)
	if second["type"] != "function_call_output" || second["call_id"] != "call_1" {
		t.Fatalf("unexpected function call output: %#v", second)
	}
}

func TestResponsesWSToolCallArgumentsDeltasSurviveItemIDToCallIDMapping(t *testing.T) {
	state := &responsesWSCallState{}

	state.appendResponsesWSToolCallArguments("fc_1", `{"cmd":`)
	state.observeResponsesWSToolCallItem(&dto.ResponsesOutput{
		Type:   "function_call",
		ID:     "fc_1",
		CallId: "call_1",
		Name:   "exec_command",
	})
	state.appendResponsesWSToolCallArguments("fc_1", `"pwd"}`)

	call, ok := state.toolCalls["call_1"]
	if !ok {
		t.Fatal("tool call was not recorded under canonical call_id")
	}
	if got := call.ArgumentsString(); got != `{"cmd":"pwd"}` {
		t.Fatalf("arguments = %q, want full merged JSON string", got)
	}
}

func TestResponsesWSCanRetryStateErrorRejectsPreviousResponseContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session := &responsesWSSession{c: ctx, client: &websocket.Conn{}}
	state := &responsesWSCallState{create: responsesWSCreateRequest{Request: dto.OpenAIResponsesRequest{
		Model:              "gpt-5.5",
		PreviousResponseID: "resp_123",
		Input:              json.RawMessage(`[{"type":"function_call_output","call_id":"call_1","output":"ok"}]`),
	}}}
	apiErr := newResponsesWSTextualUpstreamError("status_code=429, Previous response with id 'resp_123' not found.")

	if session.canRetryTerminalUpstreamError(state, apiErr) {
		t.Fatal("previous_response_id continuations depend on prior upstream state and must not retry on a fallback channel")
	}
}

func TestResponsesWSCanRetryStateErrorAllowsReplayablePreviousResponseContinuation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	session := &responsesWSSession{
		c:      ctx,
		client: &websocket.Conn{},
		toolCallsByResponseID: map[string][]dto.ResponsesOutput{
			"resp_123": {{Type: "function_call", ID: "fc_1", CallId: "call_1", Name: "exec_command", Arguments: json.RawMessage(`"{}"`)}},
		},
	}
	state := &responsesWSCallState{create: responsesWSCreateRequest{Request: dto.OpenAIResponsesRequest{
		Model:              "gpt-5.5",
		PreviousResponseID: "resp_123",
		Input:              json.RawMessage(`[{"type":"function_call_output","call_id":"call_1","output":"ok"}]`),
	}}}
	apiErr := newResponsesWSTextualUpstreamError("status_code=429, Previous response with id 'resp_123' not found.")

	if !session.canRetryTerminalUpstreamError(state, apiErr) {
		t.Fatal("stored function_call state should allow retry by replaying the missing tool call")
	}
}

func TestResponsesWSInvalidRequestErrorUsesBadRequestStatus(t *testing.T) {
	payload, err := buildResponsesWSErrorPayload("", newResponsesWSInvalidRequestError(errors.New("bad event")))
	if err != nil {
		t.Fatalf("buildResponsesWSErrorPayload() error = %v", err)
	}
	var data struct {
		Status int `json:"status"`
	}
	if err := common.Unmarshal(payload, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if data.Status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", data.Status, http.StatusBadRequest)
	}
}

func TestRemoveResponsesWSTransportFields(t *testing.T) {
	payload := []byte(`{
		"model": "gpt-5.3-codex-spark",
		"stream": true,
		"background": true,
		"stream_options": {"include_usage": true},
		"store": false
	}`)

	got, err := removeResponsesWSTransportFields(payload)
	if err != nil {
		t.Fatalf("removeResponsesWSTransportFields() error = %v", err)
	}
	var data map[string]any
	if err := common.Unmarshal(got, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	for _, key := range []string{"stream", "background", "stream_options"} {
		if _, ok := data[key]; ok {
			t.Fatalf("transport field %q still present in %s", key, got)
		}
	}
	if data["store"] != false {
		t.Fatalf("store = %#v, want false", data["store"])
	}
}

func TestToWebSocketURL(t *testing.T) {
	tests := map[string]string{
		"https://api.openai.com/v1/responses":             "wss://api.openai.com/v1/responses",
		"http://127.0.0.1:3000/v1/responses":              "ws://127.0.0.1:3000/v1/responses",
		"wss://chatgpt.com/backend-api/codex/responses":   "wss://chatgpt.com/backend-api/codex/responses",
		"ws://127.0.0.1:3000/backend-api/codex/responses": "ws://127.0.0.1:3000/backend-api/codex/responses",
	}

	for input, want := range tests {
		if got := toWebSocketURL(input); got != want {
			t.Fatalf("toWebSocketURL(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestHandleTargetWriteFailureWithStateReleasesCurrentAndClearsTarget(t *testing.T) {
	target, cleanup := newTestResponsesWSTarget(t)
	defer cleanup()

	var committed *bool
	session := &responsesWSSession{target: target}
	state := &responsesWSCallState{
		info: &relaycommon.RelayInfo{},
		commitRate: func(success bool) {
			committed = &success
		},
	}
	session.current = state

	apiErr := session.handleTargetWriteFailureWithState(state, errors.New("write failed"))

	if apiErr == nil {
		t.Fatal("apiErr is nil")
	}
	if session.target != nil {
		t.Fatal("target was not cleared")
	}
	if session.getCurrent() != nil {
		t.Fatal("current response was not released")
	}
	if committed == nil || *committed {
		t.Fatalf("commit success = %v, want false", committed)
	}
}

func TestHandleControlEventWriteFailureSendsResponsesError(t *testing.T) {
	clientConn, serverConn, cleanupClient := newTestWebSocketPair(t)
	defer cleanupClient()
	target, cleanupTarget := newTestResponsesWSTarget(t)
	defer cleanupTarget()

	session := &responsesWSSession{
		client: serverConn,
		target: target,
	}
	apiErr := session.handleControlEventWriteFailure(errors.New("write failed"))
	if apiErr != nil {
		t.Fatalf("handleControlEventWriteFailure() error = %v", apiErr)
	}
	if session.target != nil {
		t.Fatal("target was not cleared")
	}

	if err := clientConn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, payload, err := clientConn.ReadMessage()
	if err != nil {
		t.Fatalf("read responses error event: %v", err)
	}
	var data struct {
		Type   string `json:"type"`
		Status int    `json:"status"`
	}
	if err := common.Unmarshal(payload, &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if data.Type != "error" || data.Status == 0 {
		t.Fatalf("unexpected error event: %s", payload)
	}
}

func TestObserveUpstreamFailedReleasesCurrent(t *testing.T) {
	var committed *bool
	session := &responsesWSSession{}
	state := &responsesWSCallState{
		info: &relaycommon.RelayInfo{},
		commitRate: func(success bool) {
			committed = &success
		},
	}
	session.current = state

	session.observeUpstreamMessage([]byte(`{"type":"response.failed"}`))

	if session.getCurrent() != nil {
		t.Fatal("current response was not released")
	}
	if committed == nil || *committed {
		t.Fatalf("commit success = %v, want false", committed)
	}
}

func newTestResponsesWSTarget(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()
	target, _, cleanup := newTestWebSocketPair(t)
	return target, cleanup
}

func newTestWebSocketPair(t *testing.T) (*websocket.Conn, *websocket.Conn, func()) {
	t.Helper()
	upgrader := websocket.Upgrader{}
	serverConnCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		serverConnCh <- conn
	}))

	targetURL := "ws" + strings.TrimPrefix(server.URL, "http")
	target, _, err := websocket.DefaultDialer.Dial(targetURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket: %v", err)
	}
	serverConn := <-serverConnCh
	cleanup := func() {
		_ = target.Close()
		_ = serverConn.Close()
		server.Close()
	}
	return target, serverConn, cleanup
}
