# Error Handling

> Error types, propagation, and response patterns used by the backend.

---

## Overview

There are two main error-handling families:

1. Dashboard/admin API handlers return Gin JSON responses, usually with a `success` boolean and a human-readable `message`.
2. Relay handlers return OpenAI/Claude/Midjourney-compatible error shapes using `types.NewAPIError` and wrapper DTOs.

Representative files:

- `controller/relay.go` centralizes relay error rendering.
- `types/error.go` defines `NewAPIError`, error codes, error types, masking, and OpenAI/Claude conversion.
- `service/error.go` wraps provider/upstream errors and converts upstream response bodies.
- `middleware/auth.go` and `middleware/distributor.go` abort request pipelines with auth/distribution errors.
- `model/errors.go` defines model-level sentinel errors.

---

## Relay Error Types

Use `types.NewAPIError` for relay-path errors.

`types/error.go` defines:

- `OpenAIError`
- `ClaudeError`
- `ErrorType`
- `ErrorCode`
- `NewAPIError`
- constructors such as `NewError`, `NewOpenAIError`, `NewErrorWithStatusCode`, and `InitOpenAIError`
- options such as skip-retry / record-error-log control

Relay code should return `*types.NewAPIError`, not write HTTP responses directly, unless it is already at the controller response boundary.

Examples:

- `relay/image_handler.go`, `relay/audio_handler.go`, `relay/rerank_handler.go`, and `relay/compatible_handler.go` return `types.NewError(...)` or `types.NewOpenAIError(...)` from helper failures.
- `service/pre_consume_quota.go` returns `types.NewErrorWithStatusCode(..., http.StatusForbidden, ...)` for quota failures.
- `relay/channel/*` adapters convert provider-specific failures into `types.NewError`/`types.NewOpenAIError`.

Use specific `ErrorCode` constants instead of arbitrary strings. Add a new `ErrorCode` only when the code is part of a stable cross-layer contract.

---

## Relay Error Rendering

`controller.Relay` in `controller/relay.go` defers a centralized renderer:

- logs relay errors with `logger.LogError`;
- appends request ID via `common.MessageWithRequestId`;
- renders Claude format for `types.RelayFormatClaude`;
- renders OpenAI format for most relay requests;
- sends websocket errors through helper functions for realtime relay.

Do not duplicate relay error rendering inside provider adapters. Return `*types.NewAPIError` and let the controller render it.

`controller.RelayMidjourney` is a separate path because Midjourney-compatible endpoints use their own error fields (`description`, `type`, `code`) and task error DTOs.

---

## Upstream Error Wrapping

`service/error.go` contains wrappers for provider/upstream failures:

- `ClaudeErrorWrapper` and `ClaudeErrorWrapperLocal`;
- `TaskErrorWrapper` and `TaskErrorWrapperLocal`;
- `RelayErrorHandler` for non-2xx upstream HTTP responses;
- `ResetStatusCode` for configured status-code mapping.

Important conventions:

- Network-like errors (`post`, `dial`, `http`) are logged and may be replaced or masked before returning to clients.
- `TaskErrorWrapper` uses `common.MaskSensitiveInfo` to avoid exposing sensitive upstream details.
- `RelayErrorHandler` parses known upstream error bodies through `dto.GeneralErrorResponse` and returns provider-compatible `NewAPIError` values.
- `NewAPIError.ToOpenAIError` and `ToClaudeError` mask messages except for selected internal cases such as token counting.

When adding a new provider adapter, prefer converting provider response errors to existing `types.NewAPIError` forms and reuse `service.RelayErrorHandler` when it matches the upstream response shape.

---

## Responses Stream Failure Events

### 1. Scope / Trigger

- Trigger: any change to OpenAI Responses SSE handling in `relay/channel/openai`.
- Applies to both native `/v1/responses` streams and bridge paths such as
  Claude Messages -> OpenAI Responses -> Chat/Claude stream conversion.

### 2. Signatures

- Native stream handler:
  `OaiResponsesStreamHandler(c, info, resp) (*dto.Usage, *types.NewAPIError)`.
- Chat/Claude bridge stream handler:
  `OaiResponsesToChatStreamHandler(c, info, resp) (*dto.Usage, *types.NewAPIError)`.
- Shared failure formatter:
  `newResponsesStreamAPIError(dto.ResponsesStreamResponse, string) *types.NewAPIError`.

### 3. Contracts

- Upstream SSE event types `response.error` and `response.failed` are relay
  errors, not successful zero-token completions.
- Successful Responses final payloads must not expose `response.output` /
  `output` as `null`. Normalize missing or null output arrays to `[]` before
  forwarding native `/v1/responses` bodies or `response.completed` SSE events,
  because SDK clients commonly iterate that field during stream finalization.
- Error details must be extracted from nested `response.error` first, and from
  top-level `error` when present.
- Detail extraction must not require `error.type`; `error.message` or
  `error.code` alone is enough to preserve useful upstream diagnostics.
- Returned errors keep the stable relay code `types.ErrorCodeBadResponse` while
  the message includes the upstream event, message, code, type, and param when
  available.
- Do not persist full SSE payloads or request bodies in error logs.

### 4. Validation & Error Matrix

- `response.failed` with `response.error.message` ->
  `responses stream error: response.failed: <message> (...)`.
- `response.failed` with only `response.error.code` -> include `code=<code>`.
- `response.error` with top-level `error` -> include top-level error details.
- No parseable error object -> fall back to
  `responses stream error: <event_type>`.
- Upstream success body or `response.completed` with `output: null` or missing
  `output` -> forward the same successful response with `output: []`.

### 5. Good/Base/Bad Cases

- Good: upstream sends `{"type":"response.failed","response":{"error":{"message":"Invalid tool","code":"invalid_tool"}}}` and the recorded relay error includes both
  `Invalid tool` and `invalid_tool`.
- Base: normal `response.completed` streams still mark the stream done and
  settle usage.
- Base: upstream `response.completed` may omit `response.output`; downstream
  OpenAI SDK clients must still receive an iterable `output` array.
- Bad: returning only `responses stream error: response.failed`; this loses the
  actual upstream reason.
- Bad: treating `response.failed` as a normal stream EOF and recording a
  successful consume log with zero completion tokens.
- Bad: forwarding `output: null` in a successful Responses final object; Python
  SDK consumers can fail locally with `TypeError: 'NoneType' object is not
  iterable`.

### 6. Tests Required

- `relay/channel/openai`: regression tests for native Responses and bridged
  Responses streams proving `response.failed` includes message/code details.
- `relay/channel/openai`: regression tests for non-stream Responses and
  `response.completed` streams proving null/missing `output` is normalized to
  `[]`.
- `common`: regression test proving known Responses SSE event names such as
  `response.failed` and `response.error` are not masked as plain domains, while
  real domains, URLs, and IPs still are.

### 7. Wrong vs Correct

Wrong:

```go
if oaiErr := streamResp.Response.GetOpenAIError(); oaiErr != nil && oaiErr.Type != "" {
    return types.WithOpenAIError(*oaiErr, http.StatusInternalServerError)
}
return types.NewOpenAIError(fmt.Errorf("responses stream error: %s", streamResp.Type), types.ErrorCodeBadResponse, http.StatusInternalServerError)
```

Correct:

```go
streamErr = newResponsesStreamAPIError(streamResp, data)
sr.Stop(streamErr)
```

---

## Responses WebSocket Terminal Events

### 1. Scope / Trigger

- Trigger: any change to Responses WebSocket relay handling in
  `relay/responses_websocket.go`.
- Applies to upstream events `response.completed`, `response.done`, and
  `response.incomplete` before they are forwarded to the WebSocket client.

### 2. Signatures

- Entry point:
  `controller.ResponsesWebSocket(c *gin.Context)`.
- Relay helper:
  `ResponsesWebSocketHelper(c *gin.Context, client *websocket.Conn) *types.NewAPIError`.
- Upstream observer:
  `(*responsesWSSession).observeUpstreamMessage(message []byte) ([]byte, bool, bool)`.
- Terminal normalizer:
  `normalizeResponsesWSTerminalMessageForClient(message []byte, state *responsesWSCallState, streamResponse *dto.ResponsesStreamResponse) []byte`.

### 3. Contracts

- Successful terminal WebSocket events must forward an iterable
  `response.output`; if upstream omits it or sends `null`, relay clients must
  receive `output: []`.
- Successful terminal WebSocket events must expose usable `response.usage`
  whenever the relay has upstream or locally estimated token usage.
- Usage sent to WebSocket clients must include Responses-style fields
  `input_tokens`, `output_tokens`, and `total_tokens`; legacy
  `prompt_tokens` and `completion_tokens` may also be present for internal
  billing compatibility.
- Do not overwrite non-zero upstream usage with local estimates. Local
  estimates are a fallback only.

### 4. Validation & Error Matrix

- Terminal event with upstream usage -> forward upstream usage unchanged.
- Terminal event without usage + relay estimated usage -> inject relay usage
  before forwarding to the client.
- Terminal event with missing or null output -> normalize to `output: []`.
- Invalid terminal JSON or missing `response` object -> forward the original
  message rather than failing a successful response.

### 5. Good/Base/Bad Cases

- Good: Codex over WebSocket receives `response.completed.response.usage` and
  can update its context/auto-compact token accounting.
- Base: HTTP/SSE Responses behavior remains unchanged.
- Bad: settling `PostTextConsumeQuota` with estimated usage while forwarding a
  terminal WebSocket event with no `response.usage`; Codex may undercount and
  miss auto-compact before the model rejects the next turn.
- Bad: forwarding `response.output: null`; SDK consumers can crash when they
  iterate output during finalization.

### 6. Tests Required

- `relay`: regression test proving terminal WebSocket normalization injects
  estimated usage and `output: []` when upstream omits them.
- `relay`: regression test proving existing non-zero upstream usage is not
  overwritten by local estimates.
- `go test ./relay/...` and `go test ./...` after changing relay terminal
  event semantics.

### 7. Wrong vs Correct

Wrong:

```go
service.PostTextConsumeQuota(c, info, usage, nil)
return originalTerminalMessage // no response.usage for websocket client
```

Correct:

```go
message = normalizeResponsesWSTerminalMessageForClient(message, state, streamResponse)
service.PostTextConsumeQuota(c, info, state.usage, nil)
return message
```

---

## Dashboard/API Error Responses

Controller/admin endpoints usually respond with `gin.H`.

Observed shapes:

```go
c.JSON(http.StatusOK, gin.H{"success": false, "message": "..."})
c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": data})
```

Examples:

- `controller/channel.go` returns success/message/data for channel list and channel mutations.
- `middleware/auth.go` returns translated auth failures with `success: false`.
- `controller/custom_oauth.go`, `controller/user.go`, and `controller/option.go` follow the same broad shape.

HTTP status usage is mixed by API family:

- Auth missing/malformed cases often use `http.StatusUnauthorized`.
- Internal/database failures may use `http.StatusInternalServerError`.
- Many business-level failures still return `http.StatusOK` with `success: false`.
- Payment/top-up handlers sometimes use `{ "message": "error", "data": ... }` legacy shapes; preserve existing family shape when editing nearby code.

When adding a new endpoint, match the response shape of adjacent endpoints in the same controller file.

### Scenario: Playground debug capture polling

#### 1. Scope / Trigger

- Trigger: changes to `controller.PlaygroundDebug`,
  `service/playground_debug.go`, `/pg/debug/:debug_id`, or the default
  frontend code that polls playground upstream-request debug data.
- This is a cross-layer diagnostic side channel: the primary relay request may
  succeed even when debug data has not been captured yet.

#### 2. Signatures

- Route: `GET /pg/debug/:debug_id`
- Header used by the primary request: `X-Playground-Debug-Id`
- Handler: `controller.PlaygroundDebug(c *gin.Context)`
- Store lookup: `service.GetPlaygroundUpstreamRequestDebug(userID int, debugID string)`

#### 3. Contracts

- Invalid or missing `debug_id` remains an HTTP `400` with
  `{"success": false, "message": "invalid debug id"}`.
- A valid `debug_id` with no captured upstream request is a normal polling
  miss, not an HTTP/network failure. Return HTTP `200` with
  `{"success": false, "message": "debug data not found"}`.
- Captured debug data returns HTTP `200` with
  `{"success": true, "data": {"upstream_request": ..., "body_bytes": ..., "body_truncated": ..., "captured_at": ...}}`.
- Frontend callers that poll this endpoint must suppress global business-error
  handling for `success:false` misses.
- Provider adapters that bypass `relay/channel.DoApiRequest` and create their
  own upstream `http.Request` values must call
  `service.RecordPlaygroundUpstreamRequestDebug` for the final user-facing
  upstream request body. `relay/channel/chatgptimg` is one such adapter because
  it posts ChatGPT Web payloads directly to `/backend-api/f/conversation`.

#### 4. Validation & Error Matrix

- Malformed id such as `bad/id` -> HTTP `400`, no store lookup.
- Valid id but capture has not happened, expired, or provider path does not
  support capture -> HTTP `200`, `success:false`.
- Valid id with stored capture for the authenticated user -> HTTP `200`,
  `success:true`, debug payload.
- Valid id stored for a different user -> HTTP `200`, `success:false`.

#### 5. Good/Base/Bad Cases

- Good: a streaming playground chat can poll `/pg/debug/:debug_id` while SSE is
  still opening without producing browser 404 console errors.
- Good: ChatGPT Web / `chatgptimg` playground requests capture the final
  `/backend-api/f/conversation` payload even though they bypass the common
  channel request helper.
- Base: relay chat/image responses continue to use their OpenAI-compatible
  status and body shapes.
- Bad: returning HTTP `404` for an uncaptured debug record; browsers report the
  expected polling miss as a network error and the frontend may retry loudly.
- Bad: showing toast errors for `success:false` debug polling misses.
- Bad: only capturing bodies inside `relay/channel.DoApiRequest`; custom
  clients that hand-build upstream HTTP requests will keep returning
  `debug data not found` for otherwise successful playground requests.

#### 6. Tests Required

- `controller`: regression test that a valid uncaptured `debug_id` returns HTTP
  `200` with `success:false`.
- `service`: keep normalization, per-user storage, expiry, and data-url
  redaction tests passing.
- `relay/channel/chatgptimg`: regression tests proving `StreamChatConversation`
  and `StreamFConversation` invoke the final upstream request-body capture hook.
- `web/default`: type-check any frontend polling changes and lint the touched
  file when the full repository lint has unrelated existing failures.

#### 7. Wrong vs Correct

Wrong:

```go
c.JSON(http.StatusNotFound, gin.H{
    "success": false,
    "message": "debug data not found",
})
```

Correct:

```go
c.JSON(http.StatusOK, gin.H{
    "success": false,
    "message": "debug data not found",
})
```

---

## Middleware Abort Patterns

Middleware should stop the Gin chain after writing a response.

Examples:

- `middleware/auth.go` writes `c.JSON(...)`, calls `c.Abort()`, and returns for failed auth.
- `middleware/distributor.go` uses helper functions such as `abortWithOpenAiMessage` for relay-compatible failures.

Always return immediately after aborting to avoid continuing the handler chain.

---

## Model and Service Error Propagation

Model/service functions usually return `error` or `*types.NewAPIError` and let callers decide the HTTP response.

Patterns:

- Model query/update helpers return `error` from GORM calls.
- Service functions return typed relay errors when the caller is relay-related.
- Sentinel errors are used where callers need `errors.Is`; for example auth code checks in `middleware/auth.go` distinguish database errors from invalid access tokens.
- `types.NewAPIError.Unwrap` enables `errors.Is` / `errors.As` on wrapped relay errors.

Do not log and return the same low-level error repeatedly unless the log adds context not available to the caller.

---

## Sensitive Data Handling

Errors that may include upstream URLs, credentials, request bodies, or headers must be masked before client exposure.

Existing helpers:

- `common.MaskSensitiveInfo`
- `NewAPIError.MaskSensitiveError`
- `NewAPIError.MaskSensitiveErrorWithStatusCode`
- `NewAPIError.ToOpenAIError`
- `NewAPIError.ToClaudeError`

Examples:

- `service.TaskErrorWrapper` masks network/upstream error strings.
- `controller.Relay` records masked error logs through `model.RecordErrorLog`.
- `service/download.go` logs masked origin URLs.

Never include API keys, tokens, cookies, auth headers, or full upstream request payloads in client-visible errors.

---

## Common Mistakes to Avoid

- Do not write relay HTTP errors directly inside provider adapters; return `*types.NewAPIError`.
- Do not bypass request ID decoration in `controller.Relay`.
- Do not expose raw upstream/network errors without masking.
- Do not introduce a new dashboard response shape when an adjacent controller already has one.
- Do not call `c.Abort()` without returning.
- Do not collapse distinct error codes into generic strings if retry, billing, or log behavior depends on the code.
