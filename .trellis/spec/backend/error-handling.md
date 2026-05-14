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
