# Quality Guidelines

> Backend quality standards and review checks for this repository.

---

## Overview

The backend is a Go API gateway built with Gin and GORM. The current `go.mod` declares Go `1.25.1`, while project-level guidance supports Go `1.22+`.

The most important quality constraints are:

- preserve cross-database compatibility: SQLite, MySQL, and PostgreSQL;
- use the project JSON wrappers for new marshal/unmarshal work;
- preserve upstream relay request semantics, especially explicit zero values;
- keep route/controller/service/model responsibilities separated;
- protect secrets and sensitive data in logs and errors;
- match existing response/error shapes for the API family being changed.

---

## Required Patterns

### JSON

For new business code, use wrappers in `common/json.go`:

- `common.Marshal`
- `common.Unmarshal`
- `common.UnmarshalJsonStr`
- `common.DecodeJson`
- `common.GetJsonType`

`encoding/json` type references such as `json.RawMessage` and `json.Number` are acceptable as types. Direct `json.Marshal`, `json.Unmarshal`, or `json.NewDecoder` calls exist in legacy/provider-specific code, but new code should prefer `common.*` wrappers unless there is a documented compatibility reason.

Examples of wrapper usage:

- `model.ChannelInfo.Value` / `Scan` in `model/channel.go`;
- relay request conversion in `relay/chat_completions_via_responses.go`;
- request validation in `relay/helper/valid_request.go`;
- provider adapters such as `relay/channel/chatgptimg/*` and task adapters under `relay/channel/task/*`.

### Optional upstream request fields

For request structs parsed from client JSON and re-marshaled upstream, optional scalar fields must use pointers with `omitempty`:

```go
MaxTokens *int     `json:"max_tokens,omitempty"`
Stream    *bool    `json:"stream,omitempty"`
Temp      *float64 `json:"temperature,omitempty"`
```

This preserves:

- absent field -> `nil` -> omitted upstream;
- explicit `0` / `false` -> non-nil pointer -> still sent upstream.

Regression examples:

- `dto/openai_request_zero_value_test.go`;
- `dto/gemini_generation_config_test.go`;
- `dto/gemini_isstream_test.go`.

### Database compatibility

Follow `.trellis/spec/backend/database-guidelines.md` and the project-level DB rules. New DB code must work on SQLite, MySQL, and PostgreSQL, or include explicit guarded branches.

### SMTP email TLS modes

#### 1. Scope / Trigger

- Trigger: any change to `common.SendEmail`, SMTP settings, registration email verification, password-reset email, or email notification delivery.
- The `SMTPSSLEnabled` option means “use a secure SMTP connection”, but SMTP servers expose two different secure modes.

#### 2. Signatures

- Sender entrypoint: `common.SendEmail(subject string, receiver string, content string) error`
- Runtime settings:
  - `common.SMTPServer`
  - `common.SMTPPort`
  - `common.SMTPSSLEnabled`
  - `common.SMTPAccount`
  - `common.SMTPFrom`
  - `common.SMTPToken`
  - `common.SMTPForceAuthLogin`

#### 3. Contracts

- Port `465` uses implicit TLS: connect with TLS from the first byte, then create the SMTP client.
- Non-465 ports with `SMTPSSLEnabled=true` use explicit STARTTLS: connect in plaintext, read SMTP greeting/EHLO, require `STARTTLS`, then upgrade.
- Non-465 ports with `SMTPSSLEnabled=false` may use `smtp.SendMail`; Go's `net/smtp` can still auto-upgrade when the server advertises STARTTLS.

#### 4. Validation & Error Matrix

- `SMTPServer == "" && SMTPAccount == ""` -> return `SMTP 服务器未配置`.
- `SMTPFrom` without an email domain -> return `invalid SMTP account`.
- `SMTPSSLEnabled=true`, non-465 port, server lacks `STARTTLS` -> return `SMTP server does not support STARTTLS`.
- Never use implicit TLS on port `587`; that causes `tls: first record does not look like a TLS handshake` with STARTTLS submission servers.

#### 5. Good/Base/Bad Cases

- Good: `SMTPPort=465`, `SMTPSSLEnabled=true` -> implicit TLS.
- Good: `SMTPPort=587`, `SMTPSSLEnabled=true` -> plaintext SMTP greeting followed by STARTTLS.
- Base: `SMTPPort=587`, `SMTPSSLEnabled=false` -> `smtp.SendMail` path may opportunistically STARTTLS.
- Bad: `SMTPPort=587`, `SMTPSSLEnabled=true`, direct `tls.Dial` before SMTP greeting.

#### 6. Tests Required

- Add a regression test with a fake STARTTLS SMTP server asserting non-465 `SMTPSSLEnabled=true` sends plaintext `EHLO` first, then `STARTTLS`, and completes a message after TLS upgrade.

#### 7. Wrong vs Correct

Wrong:

```go
if SMTPPort == 465 || SMTPSSLEnabled {
    conn, err := tls.Dial("tcp", addr, tlsConfig)
}
```

Correct:

```go
if SMTPPort == 465 {
    // implicit TLS
} else if SMTPSSLEnabled {
    // plaintext SMTP greeting, then STARTTLS
}
```

### Relay/provider changes

When adding or modifying a channel:

- follow the existing provider adapter shape under `relay/channel/<provider>/`;
- return `*types.NewAPIError` from relay helpers/adapters;
- confirm whether the provider supports `StreamOptions`;
- update stream support registration if needed;
- add focused tests near the adapter.

### ChatGPT Web session reuse

ChatGPT Web upstream conversation reuse should prefer an explicit prompt cache
key or session identifier from the request JSON body, form/multipart fields, or
headers. When the client omits an explicit session key, conversation reuse may
fall back to stable relay identity (`token_id`, then `user_id`) after the
ChatGPT Web channel has already been selected. It must never use a heuristic
that hashes the first user message.

Why:

- hashing the first user message makes independent requests with the same
  opening text share one cached conversation;
- that can reuse a stale conversation ID across unrelated requests and produce
  403/fallback failures that look like channel problems;
- explicit session keys should come from `prompt_cache_key`,
  `openai_prompt_cache_key`, `metadata.user_id`, or the existing session header
  aliases already handled by `ExtractOpenAICompatPromptCacheKeyFromRawBody`;
- relay identity fallback is scoped by user, token, selected channel, multi-key
  index, and ChatGPT Web account fingerprint, so it continues the same upstream
  browser conversation without merging different channels/accounts.

Good:

```go
sessionKey := service.ExtractOpenAICompatPromptCacheKeyFromRawBody(rawBody, headers)
if sessionKey == "" {
    sessionKey, sessionSource = chatGPTWebSessionRouteIdentityFallback(info)
}
if sessionKey == "" {
    return chatGPTWebSessionRoute{}
}
```

Bad:

```go
if sessionKey == "" {
    sessionKey = deriveChatGPTWebSessionKeyFromMessages(req)
}
```

#### ChatGPT Web session channel affinity

1. Scope / Trigger

- Trigger: changes to ChatGPT Web session routing, channel selection, or any
  cache keyed by OpenAI-compatible `prompt_cache_key` / session aliases.

2. Signatures

- Selection hook:
  `service.GetPreferredChatGPTWebSessionChannelByAffinity(c, modelName, usingGroup) (int, bool)`.
- Record hook:
  `service.RecordChatGPTWebSessionChannelAffinity(c, channel)`.
- Clear hook:
  `service.ClearCurrentChatGPTWebSessionChannelAffinity(c)`.

3. Contracts

- Existing configured channel affinity (`GetPreferredChannelByAffinity`) has
  priority. The ChatGPT Web session-channel cache is a fallback only when the
  configured affinity layer did not select a channel.
- The ChatGPT Web session-channel cache uses its own namespace and Gin context
  keys. It must not write `ginKeyChannelAffinityMeta`, apply channel-affinity
  override templates, or change `ShouldSkipRetryAfterChannelAffinityFailure`.
- Record only enabled ChatGPT Web channels (`constant.ChannelTypeChatGPTImage`)
  after the request is considered successful by the same
  `shouldRecordChannelAffinity` gate used by configured affinity.
- The key source is the same explicit session extractor used by conversation
  reuse: `service.ExtractOpenAICompatPromptCacheKeyFromRawBody(rawBody, headers)`.
  Do not derive a channel-affinity key from message text.
- Multipart `/v1/images/edits` requests must participate in the same explicit
  session-key extraction when the client sends `prompt_cache_key`,
  `openai_prompt_cache_key`, `session_id`, `conversation_id`, or another
  supported explicit alias as a form field. Do not require image clients to move
  the same key into headers.
- Cached session-channel IDs must be validated before use: channel exists,
  channel type is ChatGPT Web, channel is enabled, and it is enabled for the
  requested group/model. Invalid stale entries are cleared and normal channel
  selection continues.

4. Validation & Error Matrix

- Missing explicit session key -> no ChatGPT Web session-channel affinity.
- Multipart image edit form contains an explicit session key -> ChatGPT Web
  session-channel affinity uses that key after configured affinity misses.
- Configured channel affinity hit -> use configured affinity; do not override
  it with ChatGPT Web session affinity.
- Cached channel missing/disabled/wrong type/not enabled for group+model ->
  clear the ChatGPT Web session-channel cache entry and fall back to ordinary
  channel selection.
- Image request cached channel is currently busy -> skip this temporary hit
  and fall back to ordinary idle-channel selection; do not clear the cache.

5. Good/Base/Bad Cases

- Good: first successful ChatGPT Web request with `prompt_cache_key=abc` records
  channel `#10`; the next request with the same key is routed to `#10`, so the
  per-channel conversation route can find the cached conversation ID.
- Base: configured Codex/Claude channel affinity still behaves exactly as
  before because its cache namespace and priority are unchanged.
- Bad: adding a new default `channel_affinity_setting` rule for ChatGPT Web;
  this can conflict with operator-configured affinity. Use the dedicated
  ChatGPT Web session-channel cache instead.
- Bad: selecting a cached ChatGPT Web channel before checking configured
  channel affinity; this silently changes existing affinity semantics.

6. Tests Required

- `service`: regression tests that explicit `prompt_cache_key` records and
  later returns the ChatGPT Web channel ID.
- `service`: regression test that multipart image edit `prompt_cache_key` form
  fields record and later return the ChatGPT Web channel ID.
- `service`: regression tests that missing explicit session key does not record
  or hit session-channel affinity.
- `service` or `middleware`: regression tests that configured channel affinity
  and ChatGPT Web session-channel affinity use separate namespaces and do not
  override each other.
- `middleware`: regression test that the distributor uses the ChatGPT Web
  session-channel fallback after configured affinity misses.

7. Wrong vs Correct

Wrong:

```go
// Changes configured channel affinity defaults and may override operator rules.
operation_setting.GetChannelAffinitySetting().Rules = append(
    []operation_setting.ChannelAffinityRule{chatGPTWebRule},
    operation_setting.GetChannelAffinitySetting().Rules...,
)
```

Correct:

```go
if channel == nil {
    if id, found := service.GetPreferredChatGPTWebSessionChannelByAffinity(c, model, usingGroup); found {
        // validate id and use it only as a fallback after configured affinity misses
    }
}
```

### Responses WebSocket relay

#### 1. Scope / Trigger

- Trigger: changes to `GET /v1/responses`, `controller.ResponsesWebSocket`, `relay/responses_websocket.go`, or upstream Responses WebSocket framing.
- The legacy generic HTTP-to-WebSocket conversion path is forbidden. Ordinary HTTP relay requests must not be converted by sending their JSON body as one WebSocket text frame.

#### 2. Signatures

- HTTP Responses remains `POST /v1/responses` and must continue through the normal HTTP relay path.
- Responses WebSocket is `GET /v1/responses` with `Upgrade: websocket` and `Sec-WebSocket-Protocol: responses` when the client provides a subprotocol.
- The first client message must be a JSON event with `type: "response.create"`. It may either wrap the Responses payload under `response` or provide a flat payload at the top level.

#### 3. Contracts

- Channel selection for Responses WebSocket happens after parsing the first `response.create` event because the model is in the WebSocket payload, not the HTTP handshake. Do not attach the normal `Distribute()` middleware to this route.
- Before forwarding upstream, normalize the payload to an upstream `response.create` event and remove transport-only fields such as `event_id`, `stream`, `stream_options`, and `background`.
- Preserve provider adapter conversion, model mapping, disabled-field removal, parameter override, affinity debug, quota pre-consume, and final usage settlement.
- Model request rate limiting for WebSocket must be checked per `response.create`; the handshake itself must not consume the model request quota.
- First-response timing for Responses WebSocket is `frt`, a first-byte/first
  upstream-frame latency metric. Set it when the first upstream WebSocket
  message arrives for the in-flight `response.create`, including protocol-only
  lifecycle frames such as `response.created`, `response.in_progress`, or empty
  item skeletons. Do not delay `frt` until semantic assistant output such as
  `response.output_text.delta` or `response.function_call_arguments.delta`;
  that makes WebSocket logs incomparable with HTTP/SSE first-byte timing.
- Upstream Responses WebSocket state errors that arrive as either terminal
  events or early assistant text are relay errors, not successful assistant
  content. This includes usage-limit/rate-limit text and
  `status_code=429, Previous response with id ... not found`.
- Upstream Responses WebSocket account/status notices such as weekly-limit
  warning banners are transport noise for API clients. Filter them before
  forwarding. They may still mark `frt` if they are the first upstream frame,
  because `frt` is a transport first-byte metric rather than a semantic-output
  metric.
- `previous_response_id` is bound to one upstream account/conversation. A
  fallback-channel retry may drop it only when the proxy can replay the missing
  state locally, for example by prepending a previously observed
  `function_call` item before the client's `function_call_output`. If the
  required state was not observed, stop and return the upstream state error;
  blindly dropping it produces `No tool call found`, while blindly sending it to
  another channel commonly produces `previous response ... not found`.

#### 4. Validation & Error Matrix

- First event missing `type` -> send WebSocket `error` event with HTTP-style status `400`.
- First event type other than `response.create` -> send WebSocket `error` event with status `400`.
- Missing `model` in `response.create` -> send WebSocket `error` event with status `400`.
- New `response.create` while another response is in progress -> send/return conflict status `409`.
- Unsupported channel type for Responses WebSocket -> fail the selected channel attempt and retry according to normal relay retry policy.
- First upstream WebSocket frame for the in-flight response -> update `frt`
  once, even if it is a protocol-only frame before model output.
- Later `response.output_text.delta`, non-empty
  `response.function_call_arguments.delta`, meaningful `response.output_item.done`,
  terminal completion, or terminal error -> must not change `frt` after the
  first upstream frame has already set it.
- Weekly-limit warning text such as `Heads up, you have less than 25%/20%/10%/5%
  of your weekly limit left. Run /status for a breakdown.` -> do not forward.
  It may update `frt` if it is the first upstream frame.
- Text delta beginning with `status_code=429` plus usage-limit/rate-limit or
  previous-response-not-found details -> convert to `types.NewAPIError` with
  status `429`, process channel error, and retry when retry policy permits.
- Retryable upstream state error with `previous_response_id` present and a
  locally stored matching function-call item -> build a stateless replay input,
  remove `previous_response_id`, then retry according to normal policy.
- Retryable upstream state error with `previous_response_id` present but no
  replayable stored state -> do not cross-channel retry; return the upstream
  state error to the client.

#### 5. Good/Base/Bad Cases

- Good: `GET /v1/responses` WebSocket, first frame `{"type":"response.create","response":{"model":"gpt-5.5","input":"hi"}}`, then upstream receives a normalized `response.create` frame.
- Good: upstream sends `response.created` at 200 ms and first
  `response.output_text.delta` at 5 s; request logs record FRT around 0.2 s,
  matching first-byte HTTP/SSE timing, not semantic output latency.
- Good: upstream sends only a weekly-limit warning banner before real output;
  the banner is suppressed for the client, and the banner arrival may control
  FRT because it was the first upstream frame.
- Good: upstream emits text `status_code=429, Previous response with id ... not found`; relay records a 429-style channel error. If the request is stateless, normal retry policy may try another channel. If it carries `function_call_output + previous_response_id` and the prior function call was observed in this WebSocket session, relay prepends the stored `function_call`, removes `previous_response_id`, and retries as a stateless input.
- Base: `POST /v1/responses` continues through the normal HTTP `ResponsesHelper` path with no WebSocket conversion.
- Base: normal assistant text that merely mentions `HTTP 429` is not treated as
  an upstream failure unless it contains explicit upstream-error markers such as
  `status_code=429`, usage-limit/rate-limit text, or previous-response-not-found.
- Bad: `POST /v1/responses` with a normal HTTP JSON body is rewritten to `wss://...` and sent as a raw text frame; this causes upstream protocol/parsing failures and must not be reintroduced.
- Bad: dropping `previous_response_id` and retrying a tool-output continuation;
  upstream can reject it with `No tool call found` because the tool call belongs
  to the previous response state.
- Bad: retrying a fallback channel with the original `previous_response_id`;
  the fallback channel cannot resolve another upstream account's response ID.

#### 6. Tests Required

- `relay`: normalize wrapper and flat `response.create` frames, remove transport fields, build error events with status.
- `relay`: first upstream WebSocket frame should mark FRT; later output
  text/function arguments/terminal events must not change it.
- `relay`: weekly-limit warning deltas/items should be suppressed without
  closing the upstream reader; if they are the first upstream frame, they may
  mark FRT.
- `relay`: textual 429 usage-limit / previous-response-not-found errors become
  429 relay errors; retry is allowed for stateless requests and for stateful
  tool-output continuations only when the proxy can replay the matching stored
  `function_call` before removing `previous_response_id`.
- `relay`: upstream write/control failures clear current state and release/refund the in-flight call.
- `service`: Responses usage mapping copies input/output token details and fallback completion details.
- `middleware`: WebSocket handshake skips the ordinary middleware quota and per-event commit records success only after a successful response.
- `router/controller`: `GET /v1/responses` routes to `ResponsesWebSocket`; `POST /v1/responses` routes to the HTTP Responses relay.

#### 7. Wrong vs Correct

##### Wrong

```go
// Do not convert ordinary HTTP bodies into upstream WebSocket frames.
resp, err := doRequestWithOptionalHTTPToWebsocket(c, info, adaptor, requestBody)
```

##### Correct

```go
// HTTP stays HTTP. Native Responses WebSocket is handled by GET /v1/responses.
resp, err := adaptor.DoRequest(c, info, requestBody)
```

### Playground async image task recovery

#### 1. Scope / Trigger

- Trigger: changes to `web/default/src/features/playground/hooks/use-chat-handler.ts`,
  playground image localStorage keys, or `/pg/images/generations/:task_id`
  polling/display behavior.
- The playground image endpoint returns a task id immediately and stores the
  final image result in the backend task table. The frontend owns polling and
  page-navigation recovery.

#### 2. Signatures

- Pending task storage item: `PendingImageGenerationTask`
  (`taskId`, `messageKey`, `sessionId`, `debugId`, `startedAt`, `updatedAt`).
- Text-to-image submit endpoint: `POST /pg/images/generations` -> `202` with
  `task_id`, `status`, and `poll_url`.
- Image-edit submit endpoint: `POST /pg/images/edits` -> `202` with `task_id`,
  `status`, and a `poll_url` that still points at
  `/pg/images/generations/:task_id`.
- Poll endpoint: `GET /pg/images/generations/:task_id` -> task status plus
  optional OpenAI-image-compatible `data`.

#### 3. Contracts

- A pending task must remain in localStorage while its assistant message is
  still `loading` or `streaming`; this is the durable client-side pointer that
  lets the playground resume after route changes or refreshes.
- Long-running playground chat and image callbacks must update the originating
  session/message by stable ids, not the currently visible session. Persist the
  update through `updateStoredSessionMessages` so a route change or remount does
  not strand the assistant message in `loading`/`streaming`.
- Streaming playground chat must register the originating assistant message as
  an active chat message before opening the SSE request and clear that marker on
  completion, error, or explicit stop. On normal completion or stop, keep the
  marker until after the terminal message update has been committed; otherwise
  the reload that precedes `updateStoredSessionMessages` can sanitize the
  message to an interrupted error before the terminal updater runs. Session
  reload/sanitization may convert stale `loading`/`streaming` messages to an
  interrupted error only when no active chat marker or pending image task
  protects that message.
- Streaming playground chat must keep its SSE source in module-level state rather
  than hook-instance state. Route changes unmount the playground component; if
  the SSE object is only held by the unmounted hook, the browser can close the
  request and the backend records `client_gone` / `context canceled`.
- Streaming playground chat must construct `sse.js` sources with `start: false`,
  attach `open`/`message`/`error`/`readystatechange` listeners, and only then
  call `stream()`. Fast responses can otherwise finish before listeners are
  attached: the browser network tab shows chunks and `[DONE]`, but React never
  receives the terminal event and later marks the message interrupted.
- Playground assistant messages whose status is `error` are UI artifacts and
  must not be sent back as assistant context in subsequent chat-completion
  payloads.
- Legacy/orphan wait messages that still display `Task ID:` / `任务 ID：` may be
  used to recreate the pending task marker and run one recovery poll.
- Terminal task handling must update the assistant message to `complete` or
  `error` before the pending task is cleaned up. If the page unmounts during a
  terminal poll, keep the pending task so the next mount can poll once more and
  render the completed image.
- Waiting text should be refreshed from the original `startedAt` timestamp, not
  reset on remount, so elapsed time remains monotonic. Use an active-page timer
  as well as the polling loop so elapsed text continues to repaint between poll
  requests.
- Completed task responses with `b64_json` should render a markdown image using
  `/pg/images/generations/:task_id/image/:index` instead of embedding large
  base64 strings in saved chat messages.
- Playground image-to-image requests must submit to `/pg/images/edits`, not
  `/pg/images/generations`. The frontend should treat any image payload field
  (`image`, `images`, or `reference_images`) as an edit request. This matters
  for ChatGPT Web image channels: sending a referenced image through the
  generations path can reuse/preview the source image and return the same bytes
  instead of an edited result.
- `/pg/images/edits` uses the same async task storage and polling endpoints as
  `/pg/images/generations`; only the submit route changes so relay mode becomes
  image edit (`RelayModeImagesEdits`) before provider conversion.

#### 4. Validation & Error Matrix

- Pending task exists and matching assistant message is pending -> resume
  polling on playground mount.
- No pending task exists but an assistant wait message contains a `task_...`
  task id -> recreate the pending marker and resume polling.
- Pending task exists but matching assistant message is complete/error/missing
  -> remove the stale pending task.
- Streaming/non-streaming chat completes after navigating away -> saved session
  messages must still receive the final assistant content or error.
- SPA route changes while a streaming playground request is active -> the client
  connection should remain open; the server must not see `client_gone` merely
  because the playground component unmounted.
- Poll returns `succeeded`/`success`/`completed` -> render image markdown and
  mark the message complete.
- Poll returns failure status -> render the provider/task failure message and
  mark the message error.
- Component unmounts or aborts while polling -> do not remove the pending task
  from storage.
- Payload has no image/reference fields -> submit to `/pg/images/generations`.
- Payload has `image`, `images`, or `reference_images` -> submit to
  `/pg/images/edits`; the returned task is still polled through
  `/pg/images/generations/:task_id`.

#### 5. Good/Base/Bad Cases

- Good: user leaves the playground while an image task is running, returns after
  backend success, and the image appears after the resume poll.
- Good: a pre-fix browser state with stale “Generating image” text and an
  embedded task id recovers on the next playground mount.
- Good: wait text updates elapsed seconds while polling and preserves the
  original task start time across remounts.
- Good: a text chat request started in the playground keeps writing to the
  original session after route changes; returning to the playground shows the
  latest streamed/final content instead of an abandoned loading message.
- Good: Playground text-to-image sends `POST /pg/images/generations`, then polls
  `/pg/images/generations/:task_id`.
- Good: Playground image-to-image sends `POST /pg/images/edits` with `image` or
  `reference_images`, then polls `/pg/images/generations/:task_id`; a real
  smoke check should verify the edited image bytes differ from the source image
  when the prompt asks for a visible change.
- Good: the sidebar/session header displays the current playground session title
  directly; do not reintroduce the default-playground session dropdown unless a
  separate session-browser UX is intentionally designed.
- Base: synchronous image-generation responses without a task id still render
  directly from the response payload.
- Bad: sending image-to-image payloads to `/pg/images/generations`; ChatGPT Web
  can return the referenced/source image again.
- Bad: deleting the pending task before the message update is committed; a route
  change can strand the saved chat message at “Generating image”.

#### 6. Tests Required

- `web/default`: unit test for completed task `b64_json` -> task image markdown
  URL.
- `web/default`: unit test for wait text elapsed calculation from `startedAt`.
- `web/default`: unit test for extracting localized task ids from wait text.
- `web/default`: unit test for updating inactive session messages in durable
  storage.
- `web/default`: unit test that image payloads are classified as edit requests,
  while text-only image prompts remain generation requests.
- `router`: regression test that `POST /pg/images/edits` is registered.
- `web/default`: run typecheck and a production build for hook changes.

#### 7. Wrong vs Correct

Wrong:

```typescript
sendImageGeneration(payloadWithImage) // posts to /pg/images/generations
```

Correct:

```typescript
const endpoint = isImageEditRequestPayload(payloadWithImage)
  ? API_ENDPOINTS.IMAGE_EDITS
  : API_ENDPOINTS.IMAGE_GENERATIONS
```

Wrong:

```typescript
removePendingImageTask(taskId, storageUserId)
completeImageGenerationMessage(messageKey, markdown)
```

Correct:

```typescript
completeImageGenerationMessage(messageKey, markdown)
// Cleanup is driven by the rendered message becoming non-pending.
```


### Relay first-byte timeout and channel retry

#### 1. Scope / Trigger

- Trigger: changes to channel tests, `relay/channel/api_request.go`, upstream HTTP request creation, relay retry behavior, or the monitoring/alarm setting `ChannelDisableThreshold`.
- This is a relay boundary contract: runtime setting -> outbound upstream request wait -> relay error code -> channel retry/disable behavior.

#### 2. Signatures

- Runtime setting: `common.ChannelDisableThreshold` in seconds.
- Channel-test helpers: `channelTestTimeoutDuration()`, `shouldSkipChannelTestTimeout(channel *model.Channel)`, and `applyChannelTestTimeout(req *http.Request, channel *model.Channel)`.
- Normal relay helpers: `upstreamFirstByteTimeoutDuration()`, `shouldApplyUpstreamFirstByteTimeout(info *relaycommon.RelayInfo)`, and `upstreamFirstByteTimeoutError(timeout time.Duration)`.
- Retry error: `types.ErrorCodeChannelResponseTimeExceeded` with HTTP status `408`.

#### 3. Contracts

- Model/channel tests must use `ChannelDisableThreshold` as their timeout budget so an upstream that never starts responding cannot leave a test running for many minutes.
- Normal relay calls must apply the threshold only while waiting for the upstream HTTP response to start. Once upstream headers/first byte have arrived, the timeout timer must be stopped so long streaming responses are not cut off merely because their total duration exceeds the threshold.
- Normal relay timeout must be represented as `channel:response_time_exceeded` with HTTP `408` so retry logic can switch to another channel. HTTP `408` is retry-only and must not auto-disable the channel, even if it is configured in automatic-disable status codes or matches automatic-disable keywords.
- Image generation/edit, async task, realtime/websocket, channel-test paths, and the local `codex-to-claude` bridge channel must not use the normal relay first-byte timer. Image/task requests can legitimately wait longer and have their own task/polling lifecycle; the local bridge has its own upstream lifecycle and should not be cut off by the gateway first-byte guard.
- Channel tests should use `ChannelDisableThreshold` by default, except the local `codex-to-claude` bridge channel; that bridge should not be wrapped by the channel-test timeout either.
- Outbound requests created from Gin handlers should use `http.NewRequestWithContext(ginRequestContext(c), ...)` so client cancellation and test timeouts propagate to the upstream request.

#### 4. Validation & Error Matrix

- `ChannelDisableThreshold <= 0` -> no first-byte timeout.
- Channel test for channel name `codex-to-claude` -> no channel-test timeout.
- Normal non-image relay and upstream does not start responding before the threshold -> cancel the upstream request, return `channel:response_time_exceeded`, HTTP `408`, retry another eligible channel, and do not auto-disable the timed-out channel.
- Normal relay receives upstream headers before the threshold -> stop the timer; continue reading/streaming the response body normally.
- Image generation/edit, task relay, or the local `codex-to-claude` bridge channel exceeds the threshold -> do not abort via the normal first-byte timer.
- Any relay error with HTTP status `408` -> retry/switch channel when retries remain; do not auto-disable the channel.
- Client/request context is canceled before the outbound request -> preserve cancellation behavior; do not classify it as a channel response-time timeout unless the gateway timer fired.

#### 5. Good/Base/Bad Cases

- Good: `/v1/responses` waits 180 seconds for upstream to start responding; if no response starts, the gateway records a 408 timeout error and retries the next channel without auto-disabling the current channel.
- Good: a stream starts within 2 seconds and continues for 5 minutes; the first-byte timer has already stopped and the stream is governed by stream idle/error handling, not the channel-disable threshold.
- Base: `ChannelDisableThreshold=0` preserves legacy no-timeout behavior.
- Bad: setting `http.Client.Timeout` globally; this also limits image/task calls and long successful streams.
- Bad: wrapping the entire normal relay request with `context.WithTimeout`; the context can expire after the first byte and incorrectly abort an otherwise healthy stream.

#### 6. Tests Required

- `controller`: regression test that channel-test timeout duration follows `ChannelDisableThreshold`.
- `controller`: regression test that channel-test timeout skips channel name `codex-to-claude`.
- `relay/channel`: regression test that `DoApiRequest` uses the Gin request context.
- `relay/channel`: regression test that normal relay first-byte timeout returns `channel:response_time_exceeded` / `408`.
- `relay/channel`: regression test that receiving headers before the threshold does not cancel later response-body reads.
- `relay/channel`: regression test that image/test/task paths are excluded from the normal first-byte timer.

#### 7. Wrong vs Correct

Wrong:

```go
ctx, cancel := context.WithTimeout(c.Request.Context(), threshold)
c.Request = c.Request.WithContext(ctx)
// A streaming response that lasts longer than threshold can be canceled mid-stream.
```

Correct:

```go
reqCtx, cancel := context.WithCancel(req.Context())
timer := time.AfterFunc(threshold, cancel)
resp, err := client.Do(req.WithContext(reqCtx))
timer.Stop() // upstream started responding; do not cap the full stream duration
resp.Body = &cancelOnCloseBody{ReadCloser: resp.Body, cancel: cancel}
```

### Admin channel list filters

#### 1. Scope / Trigger

- Trigger: changes to `GET /api/channel`, `GET /api/channel/search`, or the default frontend channel table filters.

#### 2. Signatures

- List API: `GET /api/channel?status_code=<100-599>`
- Search API: `GET /api/channel/search?status_code=<100-599>`
- Frontend URL state key: `statusCode`; backend query parameter: `status_code`.

#### 3. Contracts

- `status_code` filters by `other_info.last_status_code`, which is updated by relay/test logging paths.
- Invalid or out-of-range values must be ignored rather than returning an error, matching existing optional filter behavior.
- The filter must apply to paginated list totals and search totals so pagination stays consistent.

#### 4. Validation & Error Matrix

- `status_code` absent/empty/invalid -> no status-code filtering.
- `status_code` in `100..599` -> include only channels whose last recorded status code equals the value.

#### 5. Good/Base/Bad Cases

- Good: `/api/channel?status_code=429` returns only channels with `other_info.last_status_code == 429`.
- Base: `/api/channel` preserves the unfiltered channel list.
- Bad: client-side-only filtering after pagination; this hides matching channels on other pages and gives wrong totals.

#### 6. Tests Required

- Controller tests for list and search endpoints filtering by `status_code`.
- Frontend typecheck/lint after adding channel table URL/filter fields.

#### 7. Wrong vs Correct

Wrong:

```ts
// Filters only the current page after the server already paginated.
rows.filter((row) => row.lastStatusCode === 429)
```

Correct:

```ts
getChannels({ status_code: '429', p, page_size })
```

### API key access restrictions

#### 1. Scope / Trigger

- Trigger: changes to API key/token create or edit UI, `controller/token.go`, `model/token.go`, `middleware/auth.go`, or relay model-limit enforcement.
- This is a cross-layer contract: frontend form values -> `/api/token/` payload -> `tokens` DB fields -> token auth/distributor context -> relay authorization.

#### 2. Signatures

- Create: `POST /api/token/`
- Update: `PUT /api/token/`
- Relevant payload/storage fields:
  - `model_limits_enabled bool`
  - `model_limits string` — comma-separated model names in storage/API payload.
  - `allow_ips string` — newline and comma separated IP/CIDR allowlist.
- Runtime enforcement:
  - `model.Token.GetModelLimits()` / `GetModelLimitsMap()`
  - `model.Token.GetIpLimits()`
  - `middleware.SetupContextForToken(...)`
  - `middleware.TokenAuth()`
  - `middleware.Distribute()` model-limit check.

#### 3. Contracts

- Frontend must allow API-key model restrictions to be selected from available models **and** manually typed/pasted. Some valid models may not appear in the current group-derived option list.
- Frontend must trim and de-duplicate model names before sending.
- Backend must trim, drop empty values, and de-duplicate parsed `model_limits`.
- Backend model limit parsing must accept comma and newline delimiters so old/new UI and manual API clients round-trip safely.
- Backend IP allowlist parsing must accept newline and comma delimiters, trim spaces, and preserve CIDR strings.
- Empty `model_limits` means `model_limits_enabled=false` and all models are allowed.
- Empty `allow_ips` means no IP restriction.

#### 4. Validation & Error Matrix

- `model_limits=""` -> no model restriction.
- `model_limits=" gpt-5.5, ,gpt-4o\n"` -> limits `gpt-5.5` and `gpt-4o`.
- request model not in token limit map -> relay returns forbidden token-model-access error.
- `allow_ips=""` -> no IP restriction.
- client IP not matching any parsed IP/CIDR -> relay returns HTTP 403 access denied.
- invalid IP/CIDR entries are ignored by `common.IsIpInCIDRList`; they must not grant access.

#### 5. Good/Base/Bad Cases

- Good: user types `gpt-5.5`, presses Enter, saves, and later `/v1/responses` for `gpt-5.5` is allowed.
- Good: user pastes `gpt-5.5,gpt-4o\nclaude-sonnet-4`; all three values are saved once.
- Good: user enters `127.0.0.1, 10.0.0.0/8`; both the exact IP and CIDR are enforced.
- Base: no model limits and no IP allowlist preserves legacy unrestricted API key behavior.
- Bad: treating the model-limit input as search-only UI; typed text disappears and the saved payload has `model_limits=""`.
- Bad: splitting IP allowlist only by newline and deleting commas; `127.0.0.1,10.0.0.0/8` becomes an invalid single token.

#### 6. Tests Required

- `model`: `Token.GetModelLimits()` trims, de-duplicates, and accepts comma/newline delimiters.
- `model`: `Token.GetIpLimits()` accepts comma/newline IP/CIDR values.
- `middleware`: `SetupContextForToken()` sets normalized model-limit context.
- `controller`: `UpdateToken()` persists `model_limits_enabled`, `model_limits`, and `allow_ips`.
- `web/default`: run `bun run typecheck`, `bun run lint`, and `bun run build` after changing the API-key form or multi-select component.

#### 7. Wrong vs Correct

Wrong:

```typescript
// Search text is not a selected value; pressing Save sends an empty limit.
<MultiSelect selected={field.value} onChange={field.onChange} />
```

Correct:

```typescript
// API-key model limits must support custom typed/pasted model IDs.
<MultiSelect selected={field.value} onChange={field.onChange} allowCustomValues />
```

Wrong:

```go
return strings.Split(token.ModelLimits, ",")
```

Correct:

```go
// Trim, drop empties, de-duplicate, and accept comma/newline delimiters.
return splitAccessRestrictionValues(token.ModelLimits, func(r rune) bool {
    return r == ',' || r == '\n' || r == '\r'
})
```

### Global IP blacklist

#### 1. Scope / Trigger

- Trigger: changes to `middleware.IPBlacklist`, router middleware ordering, `ip_blacklist_setting.*`, or the default frontend Security & Limits IP blacklist settings.
- The blacklist is a site-wide access-control layer and must run before auth, relay routing, dashboard APIs, and static frontend routing.

#### 2. Signatures

- Runtime settings:
  - `ip_blacklist_setting.enabled`
  - `ip_blacklist_setting.list`
  - `ip_blacklist_setting.auto_ban_enabled`
  - `ip_blacklist_setting.auto_ban_rpm`
  - `ip_blacklist_setting.auto_ban_whitelist`
  - `ip_blacklist_setting.auto_ban_scope`
  - `ip_blacklist_setting.auto_ban_path_prefixes`
- Shared parser: `common.SplitIPList(raw string) []string`
- Matcher: `common.IsIpInCIDRList(ip net.IP, cidrList []string) bool`

#### 3. Contracts

- `enabled=false` only disables manually configured blacklist blocking when
  automatic ban is also disabled. When automatic ban is enabled, existing
  auto-banned entries in `list` remain enforced so the ban survives the request
  that added it.
- The list supports single IPs and CIDR ranges.
- Operators may separate entries with newlines, commas, or semicolons; parsing must trim whitespace and drop duplicates.
- Matched requests return HTTP 403 and abort the Gin chain before downstream middleware runs.
- Automatic ban is in-memory per process for request counting. It prefers the
  canonical `c.ClientIP()` candidate, but if that candidate is whitelisted
  because it is a local/reverse-proxy address, it may fall through to the next
  parsed forwarded candidate and count the first non-whitelisted client IP.
  Persist triggered IPs by updating `ip_blacklist_setting.list`.
- Automatic ban counts only successful requests that persisted a consume-log
  row. `model.RecordConsumeLog` must set `ContextKeyConsumeLogRecorded` after
  `LOG_DB.Create` succeeds, and `middleware.IPBlacklist` may increment the
  auto-ban counter only after `c.Next()` when the response status is 2xx and
  that context marker is present. Early relay failures such as 503 "no
  available channel", auth rejects, and dashboard/API requests without a
  consume log must not count toward the automatic-ban RPM.
- `auto_ban_enabled` defaults to true while `auto_ban_rpm=0` keeps the feature
  inert by default. This preserves compatibility for deployments that set a
  positive threshold before the explicit enable key existed.
- `auto_ban_whitelist` only exempts IPs from automatic ban; it must not bypass explicit manual blacklist entries.
- `auto_ban_scope` controls which request paths are counted for automatic ban
  RPM, but it must not scope manual blacklist enforcement:
  - `relay` is the default and only counts model/relay API paths such as
    `/v1`, `/v1beta`, `/pg/chat/completions`, `/pg/images`, `/mj`, `/suno`,
    `/kling/v1`, and `/jimeng`.
  - `all` preserves the legacy behavior and counts every HTTP request,
    including frontend page loads and dashboard APIs.
  - `custom` only counts paths matching `auto_ban_path_prefixes`; prefixes use
    newline/comma/semicolon separators and are matched with `strings.HasPrefix`.
- Empty or unknown `auto_ban_scope` values must normalize to `relay` to avoid
  false-positive bans from ordinary dashboard traffic on upgraded deployments.

#### 4. Validation & Error Matrix

- `enabled=false`, `auto_ban_enabled=false` -> no blocking.
- `enabled=false`, `auto_ban_enabled=true`, and an IP is already in `list` -> block the IP so automatically banned entries remain effective.
- `enabled=true`, `list=""` -> no blocking.
- `enabled=true`, malformed client IP -> HTTP 403 `无法解析客户端 IP 地址`.
- `enabled=true`, client IP in list -> HTTP 403 `当前 IP 已被禁止访问`.
- `auto_ban_enabled=true`, `auto_ban_rpm<=0` -> no automatic ban.
- `auto_ban_enabled=true`, successful consume-log count for the selected
  non-whitelisted client IP reaches `auto_ban_rpm` in the current minute ->
  append the IP to `list` and persist `ip_blacklist_setting.list`; the
  threshold-crossing request has already completed successfully, so subsequent
  matching requests return HTTP 403 `当前 IP 已被禁止访问`.
- Matching path returns 4xx/5xx or does not persist a consume log -> do not
  count toward automatic ban.
- Client IP in `auto_ban_whitelist` -> skip automatic ban counting for that IP, but still enforce the manual blacklist.
- `auto_ban_scope=relay`, `/api/status` -> do not count toward automatic ban.
- `auto_ban_scope=relay`, `/v1/chat/completions` -> count toward automatic ban.
- `auto_ban_scope=all`, `/api/status` -> count toward automatic ban.
- `auto_ban_scope=custom`, path not matching `auto_ban_path_prefixes` -> do not count.
- Invalid entries in the blacklist are ignored by `common.IsIpInCIDRList`; do not fail startup or option loading.

#### 5. Good/Base/Bad Cases

- Good: `203.0.113.8` with `203.0.113.0/24` is blocked.
- Good: `198.51.100.10` with `203.0.113.0/24` is allowed.
- Good: `auto_ban_rpm=60` adds `203.0.113.9` to the persisted blacklist when
  the same selected client IP reaches 60 successful consume-log requests in one
  minute.
- Good: `203.0.113.10` in `auto_ban_whitelist` is not auto-banned even when it exceeds the RPM threshold.
- Good: `127.0.0.1` in `auto_ban_whitelist` with `X-Real-IP: 203.0.113.9`
  counts and bans `203.0.113.9`, not the local reverse proxy.
- Good: default `auto_ban_scope=relay` prevents frequent dashboard/API page
  loads from automatically banning a user who has no model relay traffic.
- Good: `auto_ban_scope=custom` with `/api/token` only counts token endpoints,
  not unrelated `/api/status` checks.
- Good: repeated `/v1/responses` requests that fail before consume-log
  persistence, for example 503 no-available-channel responses, do not trigger
  automatic ban even if their raw GIN RPM exceeds the threshold.
- Base: disabled manual blacklist with automatic ban disabled and any list is allowed.
- Bad: adding the middleware only to `/api`, leaving `/v1` relay or frontend routes unprotected.
- Bad: auto-banning every `X-Forwarded-For` value; spoofed headers could ban unrelated victims. Select one non-whitelisted client candidate for automatic counting.
- Bad: using `all` as the upgraded default; normal browser boot/login/API polling
  can exceed low RPM thresholds and cause false automatic bans.

#### 6. Tests Required

- Unit-test separator parsing and duplicate removal.
- Middleware-test both blocked and allowed request paths.
- Middleware-test automatic ban threshold behavior, whitelist exemption, and
  reverse-proxy fallback from a whitelisted proxy IP to a forwarded client IP.
- Middleware-test automatic ban scope behavior for default/relay, all, and
  custom-prefix modes.
- Middleware-test that 4xx/5xx relay failures and 2xx responses without
  `ContextKeyConsumeLogRecorded` do not count toward automatic ban.
- Model-test that `RecordConsumeLog` marks `ContextKeyConsumeLogRecorded` only
  after successful log persistence.

#### 7. Wrong vs Correct

Wrong:

```go
apiRouter.Use(middleware.IPBlacklist())
```

Correct:

```go
func SetRouter(router *gin.Engine, assets ThemeAssets) {
    router.Use(middleware.IPBlacklist())
    // register API, relay, dashboard, and web routes after this
}
```

Wrong:

```go
// Do not auto-ban every forwarded header value; clients can spoof them.
for _, ip := range strings.Split(c.GetHeader("X-Forwarded-For"), ",") {
    countAndMaybeBan(ip)
}
```

Correct:

```go
// Prefer the canonical client IP, but skip whitelisted local/reverse-proxy
// candidates so the real forwarded client can be counted.
clientIP, ok := selectAutoBanClientIP(blacklistClientIPCandidates(c), whitelist)
if !ok {
    return
}
if c.Writer.Status() >= http.StatusOK &&
    c.Writer.Status() < http.StatusMultipleChoices &&
    common.GetContextKeyBool(c, constant.ContextKeyConsumeLogRecorded) {
    countAndMaybeBan(clientIP.ip.String())
}
```

### ChatGPT Web image requests and playground async image tasks

#### 1. Scope / Trigger

- Trigger: any change to `relay/channel/chatgptimg/`, `relay/helper/valid_request.go` image parsing, or the default frontend playground image-generation flow.
- This is a cross-layer contract: client request parsing -> provider adapter response format -> playground task polling -> frontend rendering.

#### 2. Signatures

- Parser: `helper.GetAndValidOpenAIImageRequest(c *gin.Context, relayMode int) (*dto.ImageRequest, error)`
- Adapter: `(*chatgptimg.Adaptor).ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error)`
- Playground poll API: `GET /pg/images/generations/:task_id`
- Playground image content API: `GET /pg/images/generations/:task_id/image/:index`

#### 3. Contracts

- Multipart `/v1/images/edits` must preserve `response_format` from form fields; do not drop it while parsing `prompt`, `model`, `n`, `quality`, `size`, `image`, or `watermark`.
- Multipart `/v1/images/edits` must also preserve explicit unknown form fields
  in `dto.ImageRequest.Extra` so ChatGPT Web image conversion can see
  `conversation_id`, `fallback_prompt`, `fallback_reference_images`,
  `reference_images`, `prompt_cache_key`, and related compatibility fields.
- ChatGPT Web image generation/edit requests should reuse the same
  session-route cache as chat requests when an explicit session key is present:
  resolve the cached conversation for the selected channel/account before
  calling `/backend-api/f/conversation`, and record the returned conversation id
  after a successful image request.
- ChatGPT Web image requests default to `response_format=b64_json` when the client does not specify a format. Image clients such as Cherry Studio expect `b64_json` to be pure base64 media data, not a gateway URL.
- Explicit `response_format` values must be preserved for normal image models. Exception: ChatGPT Web `gpt-image-2` / `chatgpt-image-2` (including model aliases mapped upstream to those names) must force `b64_json`, because downstream image-edit clients reuse the generated image bytes and fail when the gateway returns only a URL.
- The ChatGPT Web `gpt-image-2` / `chatgpt-image-2` force-to-base64 rule must be enforced at both request normalization and response construction. A stale or overridden `response_format=url` must not cause `url` to be emitted for these models.
- OpenAI-compatible image JSON must omit empty image fields. A base64 image item should serialize as `b64_json` without an empty `url` key, so clients do not choose the wrong representation for follow-up image edits.
- ChatGPT Web reference image uploads must complete the full web upload chain: `POST /backend-api/files`, blob `PUT`, `POST /backend-api/files/{file_id}/uploaded`, then `POST /backend-api/files/process_upload_stream`. The process step should store `extra.metadata_object_id` as the uploaded file's library id.
- ChatGPT Web image polling must use the conversation mapping first and periodically fall back to `POST /backend-api/files/library`, filtering by `origination_thread_id`, ready image state/category, and excluding both uploaded `file_id` and uploaded library id so reference images are not returned as generated images.
- ChatGPT Web image endpoints may use SSE `file-service://` / `sediment://`
  refs as a low-latency hint, but must filter refs already present in the
  pre-request conversation baseline and uploaded reference file ids before
  accepting SSE results. The follow-up download-URL repoll must carry the same
  baseline and uploaded-reference exclusions; otherwise a second edit can return
  a previous output or the input reference image.
- Playground async image tasks persist task ids client-side while the assistant message is loading/streaming, and resume polling after reload if the same session/message is still pending.
- Playground wait text is user-facing UI and must go through frontend i18n; do not display provider progress percentages as real progress unless the backend can prove they are meaningful.

#### 4. Validation & Error Matrix

- Multipart parse failure -> `failed to parse image edit form request: ...`
- Missing/invalid task id on playground poll/content routes -> HTTP 400 with `error`
- Missing task or inaccessible task -> HTTP 404 with `error`
- Terminal failed task -> frontend displays `fail_reason` / upstream error from poll response.
- Long-running non-terminal task timeout -> frontend displays a localized timeout plus last known status and task id.

#### 5. Good/Base/Bad Cases

- Good: multipart edit form contains `response_format=b64_json`; parser stores it and ChatGPT Web response omits `url`.
- Good: multipart edit form contains `conversation_id`, `fallback_prompt`,
  `fallback_reference_images`, or `prompt_cache_key`; parser preserves those
  fields in `ImageRequest.Extra`, and ChatGPT Web image conversion can continue
  or recover the intended conversation.
- Good: second `gpt-image-2` edit with the same explicit session key lands on
  the same ChatGPT Web channel, resolves the cached conversation for that
  channel/account, filters prior baseline image refs from SSE, and returns only
  newly generated refs.
- Good: `gpt-image-2` request, or a mapped alias whose upstream model is `gpt-image-2`, contains `response_format=url`; ChatGPT Web overrides it to `b64_json` so follow-up image-to-image clients receive base64 media.
- Good: `gpt-image-2` response payload contains `data[].b64_json` and no `data[].url` field, even if a stale request body or override tried to force `url`.
- Good: uploaded reference images record both `file_id` and `library_file_id`; polling excludes both values and can still find generated images from `/backend-api/files/library` when the conversation mapping has not exposed a final file id yet.
- Base: image form/body omits `response_format`; ChatGPT Web image conversion defaults to `b64_json`.
- Bad: edit response returns only a gateway URL to an image-model client expecting base64; clients can throw `Invalid data content. Content string is not a base64-encoded media.`
- Bad: accepting the first SSE `sediment://` or `file-service://` ref without
  baseline filtering; follow-up edits can return the previous generated image.
- Bad: download-URL repoll without baseline/upload exclusions; when a preview
  ref is not yet downloadable, the repoll can append stale mapping/library refs.

#### 6. Tests Required

- `relay/helper`: regression test that multipart image edits preserve `response_format`.
- `relay/helper`: regression test that multipart image edits preserve
  compatibility extra fields such as `conversation_id`,
  `fallback_reference_images`, and `prompt_cache_key`.
- `relay/channel/chatgptimg`: regression tests that image generations/edits default to `b64_json` and explicit formats are preserved.
- `relay/channel/chatgptimg`: regression tests that reference uploads call `process_upload_stream`, parse `metadata_object_id`, exclude uploaded library ids, and use `/backend-api/files/library` as a generated-image fallback.
- `relay/channel/chatgptimg`: regression test that an image generation/edit
  continuation filters baseline file and sediment refs from SSE before falling
  back to polling for the new result.
- `web/default`: run `bun run typecheck` and `bun run lint` after changing playground state, i18n, or image markdown helpers.

#### 7. Wrong vs Correct

Wrong:

```go
imageRequest.Prompt = formData.Get("prompt")
imageRequest.Model = formData.Get("model")
// response_format silently lost
```

Correct:

```go
imageRequest.Prompt = formData.Get("prompt")
imageRequest.Model = formData.Get("model")
imageRequest.ResponseFormat = formData.Get("response_format")
```

### OpenAI-compatible image generation drawing logs

#### 1. Scope / Trigger

- Trigger: any change to `/v1/images/generations` or `/v1/images/edits` relay handling, OpenAI-compatible image response handlers, or drawing-log persistence for generated images.
- This is a cross-layer persistence contract: relay response body -> request-scoped captured image response -> `model.Midjourney` drawing log -> admin/user drawing-log pages.

#### 2. Signatures

- Relay handler: `relay.ImageHelper(c *gin.Context, info *relaycommon.RelayInfo) *types.NewAPIError`.
- Response capture key: `constant.ContextKeyImageGenerationResponse`.
- OpenAI-compatible response handler: `openai.OpenaiHandlerWithUsage(c, info, resp)`.
- Drawing-log persistence model: `model.Midjourney.Insert()`.

#### 3. Contracts

- Successful OpenAI-compatible image responses with non-empty `data[]` must be captured on the Gin context under `ContextKeyImageGenerationResponse`.
- `ImageHelper` must mirror successful non-test `/v1/images/generations` / image edit responses into drawing logs when the captured response contains at least one `url` or `b64_json`.
- Drawing-log `image_url` stores `data[].url` when present; otherwise it stores `data:image/png;base64,` plus `data[].b64_json` unless the value is already a data URL.
- ChatGPT Web image requests keep their adaptor-specific drawing-log path so conversation metadata is preserved; the generic `ImageHelper` mirroring must skip `ChannelTypeChatGPTImage` to avoid duplicates.
- Channel tests (`info.IsChannelTest`) must not create drawing-log rows.

#### 4. Validation & Error Matrix

- Empty/malformed image response capture -> no drawing-log row, relay response stays unchanged.
- Response item without `url` and without `b64_json` -> skip that item.
- Drawing-log insert failure -> log server-side diagnostic; do not fail an already-successful image generation response.
- ChatGPT Web image response -> adaptor-specific log only, not generic duplicate.

#### 5. Good/Base/Bad Cases

- Good: `/v1/images/generations` returns `data:[{url: ...}]`; a `midjourneys` row is inserted with action `IMAGINE`, status `SUCCESS`, image URL, model, channel id, and endpoint metadata.
- Good: response returns `data:[{b64_json: ...}]`; the drawing log stores a data URL so the generated image can be displayed.
- Base: image channel test succeeds; no drawing-log row is inserted.
- Bad: only calling `PostTextConsumeQuota` for image responses; the call appears in consume logs but not drawing logs.
- Bad: generic logging of ChatGPT Web images in addition to adaptor logging; this duplicates rows for the same generated image.

#### 6. Tests Required

- `relay`: regression test that `recordImageGenerationDrawingLog` persists an OpenAI-compatible image response into `model.Midjourney`.
- `relay`: regression test that ChatGPT Web is skipped by the generic mirror.
- `relay/channel/openai`: regression test that `OpenaiHandlerWithUsage` captures image responses in `ContextKeyImageGenerationResponse`.
- `relay/channel/chatgptimg`: regression test that stream image responses also expose a captured image response for downstream accounting/logging.

#### 7. Wrong vs Correct

Wrong:

```go
usage, err := adaptor.DoResponse(c, httpResp, info)
service.PostTextConsumeQuota(c, info, usage, logContent)
// Image exists in client response, but drawing logs never see it.
```

Correct:

```go
usage, err := adaptor.DoResponse(c, httpResp, info)
recordImageGenerationDrawingLog(c, info, imageRequest)
service.PostTextConsumeQuota(c, info, usage, logContent)
```

### OpenAI Responses prompt cache key normalization

#### 1. Scope / Trigger

- Trigger: any change that forwards OpenAI-compatible `prompt_cache_key` to upstream, including `/v1/responses`, chat-to-responses conversion, `relay/common.RemoveDisabledFields`, or channel parameter override sync rules.
- This is a relay boundary contract: client / header / param override value -> gateway JSON normalization -> upstream OpenAI-compatible request.

#### 2. Signatures

- Normalizer: `relay/common.NormalizePromptCacheKey(jsonData []byte) ([]byte, error)`
- Field normalizer: `relay/common.normalizePromptCacheKeyValue(value string) string`
- Filter path: `relay/common.RemoveDisabledFields(jsonData []byte, channelOtherSettings dto.ChannelOtherSettings, channelPassThroughEnabled bool) ([]byte, error)`
- Override path: `relay/common.ApplyParamOverrideWithRelayInfo(jsonData []byte, info *RelayInfo) ([]byte, error)`

#### 3. Contracts

- Top-level JSON field: `prompt_cache_key`.
- OpenAI-compatible upstreams reject `prompt_cache_key` strings longer than 64 characters.
- A short `prompt_cache_key` must be preserved exactly.
- A too-long `prompt_cache_key` must be converted to a deterministic SHA-256 hex string of the original value. This keeps a stable cache bucket while satisfying the 64-character upstream limit.
- Normalization must happen for raw pass-through request bodies, raw request fields processed by `RemoveDisabledFields`, and values introduced later by parameter override, especially `sync_fields` rules such as `header:session_id -> json:prompt_cache_key`.
- Codex-compatible `session_id` request headers are prompt-cache related. When channel affinity / parameter override passes `Session_id` to the upstream, the runtime header override value must be normalized with the same rule as `prompt_cache_key`; otherwise ChatGPT Codex upstream can still reject the request as an overlong `prompt_cache_key`.
- Non-string `prompt_cache_key` values are invalid client input for upstream, but this normalizer must not reinterpret them; leave them to existing request validation/upstream error behavior.

#### 4. Validation & Error Matrix

- Missing `prompt_cache_key` -> leave request unchanged.
- String length <= 64 characters and <= 64 bytes -> leave value unchanged.
- String length > 64 characters or > 64 bytes -> replace with SHA-256 hex digest of the original string.
- JSON mutation failure while normalizing after param override -> return the error to the relay handler.

#### 5. Good/Base/Bad Cases

- Good: `prompt_cache_key="short-session"` reaches upstream unchanged.
- Good: a 74-character Codex/session key becomes a 64-character SHA-256 hex string and no longer triggers upstream `string_above_max_length`.
- Base: request has no `prompt_cache_key`; no extra field is added.
- Bad: forwarding a long client/header value unchanged; OpenAI-compatible upstream returns `Invalid 'prompt_cache_key': string too long`.
- Bad: truncating to the first 64 characters; different sessions with a shared prefix can collide and reduce cache behavior quality.

#### 6. Tests Required

- `relay/common`: regression test that `RemoveDisabledFields` hashes too-long `prompt_cache_key` values and preserves short values.
- `relay/common`: regression test that raw pass-through request body readers hash too-long `prompt_cache_key` values before the body is sent upstream.
- `relay/common`: regression test that `ApplyParamOverrideWithRelayInfo` hashes a long `prompt_cache_key` introduced by `sync_fields`.
- `relay/common`: regression test that `ApplyParamOverrideWithRelayInfo` hashes a long `Session_id` header introduced by `pass_headers`.

#### 7. Wrong vs Correct

Wrong:

```go
data["prompt_cache_key"] = headerSessionID // may exceed upstream max length
```

Correct:

```go
data["prompt_cache_key"] = normalizePromptCacheKeyValue(headerSessionID)
```

### Claude/Codex Responses cache routing parity

#### 1. Scope / Trigger

- Trigger: any change to Claude-compatible `/v1/messages` or `/v1/message` conversion into OpenAI Responses, native `/v1/responses` compatibility parameters, Codex channel affinity defaults, or parameter override templates for Codex/ChatGPT Web style upstreams.
- This is a cache-routing boundary contract: Claude Code CLI request body/headers -> OpenAI-compatible intermediate request -> final Responses JSON body -> upstream headers.
- The working reference for Claude Code CLI cache routing is `claude-code-proxy`: it forwards a stable `prompt_cache_key`, mirrors it into the `Session_id` upstream header, and defaults Claude Code CLI fast/service-tier behavior to the configured priority tier.

#### 2. Signatures

- Compatibility extraction:
  - `service.ApplyOpenAICompatRequestParamsFromRawBody(req *dto.GeneralOpenAIRequest, body []byte, headers map[string]string)`
  - `service.ApplyOpenAIResponsesCompatRequestParamsFromRawBody(req *dto.OpenAIResponsesRequest, body []byte, headers map[string]string)`
- Cache/session extraction helpers:
  - `extractOpenAICompatPromptCacheKey(data map[string]json.RawMessage, headers map[string]string) string`
  - `extractOpenAICompatServiceTier(data map[string]json.RawMessage, headers map[string]string) string`
- Channel affinity setting:
  - `setting/operation_setting.GetChannelAffinitySetting()`
  - Codex rule name: `codex cli trace`
  - Codex param override operation: `sync_fields` from `json:prompt_cache_key` to `header:session_id`.
- Runtime header propagation:
  - `relay/common.ApplyParamOverrideWithRelayInfo(jsonData []byte, info *RelayInfo)`
  - `RelayInfo.RuntimeHeadersOverride` / `RelayInfo.UseRuntimeHeadersOverride`.

#### 3. Contracts

- For Claude Code CLI traffic routed to OpenAI/Codex Responses, keep cache identity consistent across body and headers:
  - final JSON body must include the stable `prompt_cache_key` when a supported session/cache source exists;
  - final upstream headers must include `Session_id` with the same normalized value when it was missing from incoming headers.
- Prompt cache key extraction order must stay aligned with `claude-code-proxy`:
  1. top-level `prompt_cache_key`;
  2. top-level `openai_prompt_cache_key`;
  3. `metadata.prompt_cache_key`;
  4. `metadata.openai_prompt_cache_key`;
  5. `metadata.user_id`, deriving nested `session_id` / `sessionId` / `conversation_id` / `conversationId` when the value is a JSON string, otherwise using the full `metadata.user_id` string;
  6. broader new-api session/header aliases only as fallbacks.
- Claude Code CLI requests that omit explicit service tier / fast parameters must default to `service_tier:"priority"` for OpenAI-compatible Responses dispatch. This mirrors the deployment behavior of `claude-code-proxy` with `CLAUDE_CODE_DEFAULT_FAST=true` and `OPENAI_SERVICE_TIER=priority`.
- Explicit `fast:false` must disable the Claude Code CLI default priority behavior; explicit client intent wins.
- Literal `service_tier:"fast"` must be normalized to `service_tier:"priority"` before upstream dispatch, because the upstream rejects unsupported literal `fast`.
- `X-Claude-Code-Session-Id` is a valid fallback session/cache source and must be treated like `X-Claude-Session-Id` / `X-Codex-Session-Id`.
- Do not enable `previous_response_id` / Responses state reuse by default as a cache-hit workaround. The stable cache path is prompt cache key + session header + service-tier parity, not stateful upstream response chaining.

#### 4. Validation & Error Matrix

- No cache/session source in body or supported headers -> do not invent a `prompt_cache_key`.
- Body contains stable cache key but incoming request lacks `Session_id` -> param override must set runtime upstream header `session_id` from `prompt_cache_key`.
- Incoming `Session_id` exists and body lacks `prompt_cache_key` -> sync may populate JSON `prompt_cache_key`, then normal prompt-cache length normalization applies.
- Any `prompt_cache_key` / `Session_id` value over the 64-character upstream limit -> normalize to deterministic SHA-256 as described in the prompt-cache normalization section.
- Claude Code CLI request with no explicit fast/service tier -> request service tier becomes `priority`.
- Claude Code CLI request with `fast:false` -> no default priority service tier is added.
- Client sends `service_tier:"fast"` -> gateway sends `service_tier:"priority"`.

#### 5. Good/Base/Bad Cases

- Good: a Claude Code CLI `/v1/messages` request with `metadata.user_id="{\"session_id\":\"abc\"}"` becomes a final Responses request with `prompt_cache_key:"abc"`, `Session_id: abc`, and `service_tier:"priority"`.
- Good: a native `/v1/responses` Codex request with only `prompt_cache_key` gets the upstream `Session_id` header synthesized by the Codex channel affinity param override template.
- Good: request logs show `request_service_tier=priority`, `request_fast=true`, and `final_request_debug.service_tier=priority` for Claude Code CLI traffic that did not explicitly opt out.
- Base: non-Claude Code clients without fast/service-tier hints keep legacy behavior and do not receive a synthetic cache key.
- Bad: only routing by channel affinity while omitting either `service_tier:"priority"` or the `Session_id` header; upstream may hit a different internal cache lane despite using the same selected channel.
- Bad: relying only on `prompt_cache_key` in body and assuming upstream treats it identically to `Session_id`; observed proxy parity showed the header mirror materially improves cache hit stability.
- Bad: using `previous_response_id` to force continuity; some upstreams reject it and it changes the compatibility contract.

#### 6. Tests Required

- `service`: regression tests that Claude Code CLI user-agent or `X-Claude-Code-Session-Id` defaults to `service_tier:"priority"`.
- `service`: regression test that explicit `fast:false` prevents the Claude Code CLI default priority service tier.
- `service`: regression test that `X-Claude-Code-Session-Id` can become the fallback prompt cache key.
- `service`: regression tests that prompt cache key extraction order matches `claude-code-proxy` priority, especially `metadata.user_id` before broader session aliases.
- `setting/operation_setting`: regression tests that default and legacy `codex cli trace` rules include `sync_fields` from `json:prompt_cache_key` to `header:session_id`.
- `service` or `relay/common`: regression test that applying the Codex channel affinity template to a request with only body `prompt_cache_key` enables `RelayInfo.UseRuntimeHeadersOverride` and sets `RuntimeHeadersOverride["session_id"]`.
- Live/debug validation after deployment: inspect recent logs for `request_service_tier=priority`, `request_fast=true`, `final_request_debug.service_tier=priority`, stable `prompt_cache_key` fingerprint, stable channel ID, and increasing upstream `cached_tokens`.

#### 7. Wrong vs Correct

Wrong:

```go
// Body key exists, but upstream header routing is left to chance.
request.PromptCacheKey = derivedSessionID
// no runtime Session_id header sync
```

Correct:

```go
// Codex affinity template mirrors the final body cache key into upstream headers.
ParamOverrideTemplate: map[string]interface{}{
    "operations": []map[string]interface{}{
        {"mode": "pass_headers", "value": codexCliPassThroughHeaders, "keep_origin": true},
        {"mode": "sync_fields", "from": "json:prompt_cache_key", "to": "header:session_id"},
    },
}
```

Wrong:

```go
// Claude Code CLI request omitted fast, so no service tier is sent.
if fast, ok := extractFast(body); ok && fast {
    req.ServiceTier = "priority"
}
```

Correct:

```go
// Match claude-code-proxy default behavior unless the client explicitly opts out.
if fast, ok := extractFast(body); ok {
    if fast {
        req.ServiceTier = "priority"
    }
} else if isClaudeCodeCLIRequest(headers) {
    req.ServiceTier = "priority"
}
```

### Claude Messages to OpenAI Responses tool schema normalization

#### 1. Scope / Trigger

- Trigger: any change to Claude-compatible `/v1/messages` conversion, OpenAI chat-to-Responses conversion, native `/v1/responses` forwarding, or function/tool schema forwarding.
- This is a relay boundary contract: Claude `tools[].input_schema` -> OpenAI-compatible chat `tools[].function.parameters` -> OpenAI Responses `tools[].parameters`, and native OpenAI Responses `tools` -> upstream OpenAI/Codex Responses `tools`.
- Claude Code system tools such as `Read`, `Edit`, `Grep`, and `Bash` must not receive schema-external parameters after conversion.

#### 2. Signatures

- Claude converter: `service.ClaudeToOpenAIRequest(claudeRequest dto.ClaudeRequest, info *relaycommon.RelayInfo) (*dto.GeneralOpenAIRequest, error)`
- Chat-to-Responses converter: `openaicompat.ChatCompletionsRequestToResponsesRequest(req *dto.GeneralOpenAIRequest) (*dto.OpenAIResponsesRequest, error)`
- Native Responses normalizer: `openaicompat.NormalizeResponsesToolSchemas(raw json.RawMessage) json.RawMessage`
- Codex adaptor boundary: `codex.Adaptor.ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error)`
- OpenAI adaptor boundary: `openai.Adaptor.ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error)`
- Function tool parameter field: `dto.FunctionRequest.Parameters any`
- Responses tool payload field: `dto.OpenAIResponsesRequest.Tools json.RawMessage`

#### 3. Contracts

- Claude `tools[].input_schema` must be carried as OpenAI chat `function.parameters`.
- Native Responses requests may arrive with Chat Completions-style nested function tools (`{"type":"function","function":{"name":...,"parameters":...}}`). Normalize these to Responses-style flat tools (`{"type":"function","name":...,"parameters":...}`) before forwarding to OpenAI/Codex.
- Before marshalling function tools into Responses `tools`, parameter schemas must be normalized with project JSON wrappers (`common.Marshal` / `common.Unmarshal`).
- `nil`, non-object, or malformed function parameter schemas must become a closed empty object schema: `{"type":"object","properties":{},"additionalProperties":false}`.
- Object schemas with `properties` and no explicit `additionalProperties` must gain `additionalProperties:false` recursively, including nested `properties`, `items`, `anyOf`, `oneOf`, and `allOf`.
- Schema entries with `nil` values must be removed before upstream forwarding.
- `required` must be preserved only when it is a JSON array; `required:null` or scalar `required` values must be removed.
- For the local/Codex `Read` function tool, remove the `pages` parameter from the JSON schema and from `required`. The upstream model otherwise tends to send an empty PDF-only `pages` argument for normal text files after Responses schema normalization.
- Do not force optional tool parameters into `required`; tools such as `Read` may have optional `offset`/`limit` fields and local tool runners expect them to remain optional.

#### 4. Validation & Error Matrix

- `parameters == nil` -> closed empty object schema.
- `parameters` cannot be decoded as an object -> closed empty object schema.
- Object schema has `properties` and lacks `additionalProperties` -> add `additionalProperties:false`.
- Nested object schema has `properties` and lacks `additionalProperties` -> add `additionalProperties:false` recursively.
- `required == nil` or non-array -> remove it.
- Function tool name is `Read` and schema has `properties.pages` -> remove `pages` from both `properties` and `required`.
- Function tool has nested `function.name` but no top-level `name` -> copy the nested value to top-level and remove the nested `function` object.
- Existing explicit `additionalProperties` -> preserve the caller-provided value.

#### 5. Good/Base/Bad Cases

- Good: Claude Code `Read` schema with `file_path`, optional `offset`, and optional `limit` reaches Responses with `additionalProperties:false`, without `required:null`, and without the PDF-only `pages` field.
- Good: native `/v1/responses` tools sent in Chat Completions nested-function shape are flattened before Codex/OpenAI forwarding, avoiding upstream `tools[0].name` missing errors.
- Base: a valid chat function schema with an existing `additionalProperties:false` remains valid and stable.
- Bad: forwarding `required:null`, scalar `required`, open object schemas, or `Read.pages`; upstream/model may emit invalid system-tool arguments such as empty PDF `pages` or schema-external page/line parameters.
- Bad: blindly marking every property as required to satisfy strict schema mode; this can break optional local tool inputs.

#### 6. Tests Required

- `service/openaicompat`: regression test that chat function tool parameters gain `additionalProperties:false`.
- `service/openaicompat`: regression test that nil values and invalid `required` values are stripped recursively.
- `service/openaicompat`: regression test that non-object function parameters default to a closed empty object schema.
- `service/openaicompat`: regression test that Chat-style nested function tools are flattened to Responses-style function tools.
- `service`: end-to-end conversion test from `ClaudeToOpenAIRequest` through `ChatCompletionsRequestToResponsesRequest` asserting the final Responses tools are normalized.
- `relay/channel/codex`: regression test that native `/v1/responses` `Read` tools remove `pages` before the upstream Codex request is sent.

#### 7. Wrong vs Correct

Wrong:

```go
tools = append(tools, map[string]any{
    "type":       "function",
    "name":       tool.Function.Name,
    "parameters": tool.Function.Parameters, // forwards required:null / open schemas
})
```

Correct:

```go
tools = append(tools, map[string]any{
    "type":       "function",
    "name":       tool.Function.Name,
    "parameters": closeFunctionToolParametersForResponses(tool.Function.Parameters),
})
```

### Channel affinity and stream completion

Channel affinity cache entries represent a successfully usable channel for a
stable request key. For streaming relay requests, HTTP status `200` is not
enough to prove success: upstream streams can terminate with
`scanner_error`, `timeout`, `client_gone`, `panic`, or soft parse errors after
headers have already been sent.

Implementation contract:

- `relay/common.RelayInfo` is stored on the Gin context under
  `constant.ContextKeyRelayInfo`.
- `middleware.shouldRecordChannelAffinity` may record affinity only when:
  - HTTP status is below `400`; and
  - non-streaming request, or stream status is normal and has no soft errors.
- Abnormal stream status must skip recording and clear the current affinity
  cache key via `service.ClearCurrentChannelAffinityCache`.

Regression tests:

- `middleware/distributor_test.go` covers normal, abnormal, and soft-error
  stream status gating.
- `service/channel_affinity_template_test.go` covers clearing the current
  affinity cache key.




### OpenAI/Responses to Claude SSE terminal events

#### 1. Scope / Trigger

- Trigger: any change to `service.StreamResponseOpenAI2Claude`, OpenAI chat streaming, or Responses-to-chat streaming paths that emit Claude-compatible SSE.
- Claude clients require a complete SSE state machine. A HTTP 200 stream that emits text deltas but omits terminal events is treated as an empty or malformed gateway response.

#### 2. Signatures

- Converter: `service.StreamResponseOpenAI2Claude(openAIResponse *dto.ChatCompletionsStreamResponse, info *relaycommon.RelayInfo) []*dto.ClaudeResponse`.
- Responses stream bridge: `openai.OaiResponsesToChatStreamHandler(c, info, resp)`.
- Required terminal Claude events after content/tool blocks: `content_block_stop`, `message_delta`, `message_stop`.

#### 3. Contracts

- Once Claude streaming has emitted `content_block_start` / `content_block_delta`, it must close the active block with `content_block_stop` before ending the message.
- A finish chunk without embedded `usage` may use `info.ClaudeConvertInfo.Usage` when available; it must not defer forever when fallback usage has already been set.
- Only defer closing on a finish chunk when both the chunk usage and `info.ClaudeConvertInfo.Usage` are nil and a later usage-only chunk may still arrive.
- Responses streams that end with `[DONE]`, EOF, or `response.completed` must still produce terminal Claude events.

#### 4. Validation & Error Matrix

- Finish chunk with `openAIResponse.Usage != nil` -> emit `content_block_stop`, `message_delta`, `message_stop`.
- Finish chunk with `openAIResponse.Usage == nil` and `info.ClaudeConvertInfo.Usage != nil` -> emit terminal events using fallback usage.
- Finish chunk with both usage sources nil -> defer terminal close for a later usage-only chunk.
- Stream ends without explicit completed event -> bridge computes fallback usage and emits terminal events.

#### 5. Good/Base/Bad Cases

- Good: `/v1/message` or `/v1/messages` via Codex/Responses returns a full Claude SSE sequence ending with `message_stop`.
- Base: native Claude channels that already emit Anthropic SSE remain unchanged.
- Bad: returning HTTP 200 with only `message_start` and `content_block_delta`; Claude clients report malformed response even though billing logs show success.

#### 6. Tests Required

- `relay/channel/openai`: regression tests for Responses-to-Claude streams ending via `[DONE]`, `response.completed` without usage, and `response.completed` with usage.
- Tests must assert that the body contains `content_block_stop`, `message_delta`, and `message_stop`.

#### 7. Wrong vs Correct

Wrong:

```go
if oaiUsage == nil {
    oaiUsage = info.ClaudeConvertInfo.Usage
    return claudeResponses // returns even when fallback usage exists
}
```

Correct:

```go
if oaiUsage == nil {
    oaiUsage = info.ClaudeConvertInfo.Usage
    if oaiUsage == nil {
        return claudeResponses
    }
}
```

### Claude messages route compatibility alias

#### 1. Scope / Trigger

- Trigger: any change to relay route registration for Claude-compatible Messages endpoints.
- Some clients may accidentally use `/v1/message` while the canonical Anthropic-compatible endpoint is `/v1/messages`.

#### 2. Signatures

- Canonical route: `POST /v1/messages` -> `controller.Relay(c, types.RelayFormatClaude)`.
- Compatibility alias: `POST /v1/message` -> `controller.Relay(c, types.RelayFormatClaude)`.

#### 3. Contracts

- `/v1/message` must be a pure alias of `/v1/messages`; both must share the same auth, distribution, relay format, and downstream processing chain.
- Do not implement a separate handler for the alias; separate logic can drift from canonical Claude behavior.
- The alias exists for client compatibility only. New clients should still prefer `/v1/messages`.

#### 4. Validation & Error Matrix

- `POST /v1/messages` -> Claude relay handler path.
- `POST /v1/message` -> same Claude relay handler path.
- Unsupported methods or other singular/plural variants -> unchanged router behavior.

#### 5. Good/Base/Bad Cases

- Good: a client configured with `/v1/message` can reach the same Claude-to-Codex conversion path as `/v1/messages`.
- Base: standards-compliant clients continue using `/v1/messages` with no behavior change.
- Bad: adding `/v1/message` under a different middleware group, bypassing token auth, distribution, or rate limiting.

#### 6. Tests Required

- `router`: regression test that both `POST /v1/messages` and `POST /v1/message` are registered.

#### 7. Wrong vs Correct

Wrong:

```go
router.POST("/v1/message", customHandler)
```

Correct:

```go
httpRouter.POST("/message", func(c *gin.Context) {
    controller.Relay(c, types.RelayFormatClaude)
})
```

### Codex responses stream default

#### 1. Scope / Trigger

- Trigger: any change to the Codex channel adaptor, `/v1/responses`, `/v1/chat/completions` to Responses compatibility, or `/v1/messages` to Responses compatibility when the selected upstream channel is `ChannelTypeCodex`.
- Codex backend responses endpoints are streaming-only; omitting `stream` on compatible client requests can produce upstream HTTP 400 errors such as `stream must set to be true`.

#### 2. Signatures

- Adaptor hook: `relay/channel/codex.Adaptor.ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error)`.
- Request field: `dto.OpenAIResponsesRequest.Stream *bool \`json:"stream,omitempty"\``.
- Unsupported upstream field: `dto.OpenAIResponsesRequest.StreamOptions *dto.StreamOptions \`json:"stream_options,omitempty"\``.
- Runtime stream flag: `relaycommon.RelayInfo.IsStream` plus Gin context key `constant.ContextKeyIsStream`.

#### 3. Contracts

- For Codex normal Responses requests (`RelayModeResponses`), `stream` must be normalized to `stream:true` before sending to upstream, even when a compatible client explicitly sends `stream:false`.
- Codex backend rejects `stream_options`; the adaptor must drop `StreamOptions` before forwarding both native Responses requests and Chat Completions compatibility requests that carried `stream_options.include_usage`.
- When the adaptor normalizes or receives `stream:true`, it must also mark `RelayInfo.IsStream=true` and update `ContextKeyIsStream`, so response handling, stream status tracking, and consume logs all use streaming semantics.
- `RelayModeResponsesCompact` must not get this default; compact requests keep their own endpoint semantics.
- `stream:false` is not forwardable to Codex normal Responses because the upstream rejects it; compact mode keeps its own endpoint semantics.

#### 4. Validation & Error Matrix

- `RelayModeResponses`, `stream` omitted -> upstream body includes `"stream":true`; downstream handled as stream.
- `RelayModeResponses`, `stream:true` -> preserve true and downstream handled as stream.
- `RelayModeResponses`, `stream:false` -> upstream body is rewritten to `"stream":true`; downstream handled as stream.
- `RelayModeResponsesCompact`, `stream` omitted -> leave omitted.
- Any Codex Responses request with `stream_options` -> upstream body omits `stream_options`.

#### 5. Good/Base/Bad Cases

- Good: Claude-compatible `/v1/messages` request without `stream` is converted through OpenAI Responses and Codex adaptor adds `stream:true` before upstream.
- Good: OpenAI-compatible `/v1/responses` request without `stream` on Codex does not fail with upstream `stream must set to be true`.
- Good: OpenAI-compatible `/v1/responses` request with explicit `stream:false` on Codex does not fail with upstream `stream must set to be true`; the gateway treats it as Codex-required streaming.
- Good: OpenAI-compatible `/v1/chat/completions` with `stream_options.include_usage` can route through Codex responses without upstream `Unsupported parameter: stream_options`.
- Base: compact request to `/v1/responses/compact` remains non-stream-defaulted.
- Bad: adding only `request.Stream=true` without syncing `RelayInfo.IsStream`; response code can parse the upstream SSE with a non-stream handler or log incorrect stream state.

#### 6. Tests Required

- `relay/channel/codex`: regression test that omitted `stream` defaults to true for `RelayModeResponses`.
- `relay/channel/codex`: regression test that explicit `stream:false` is forced to true for `RelayModeResponses`.
- `relay/channel/codex`: regression test that `RelayInfo.IsStream` and `ContextKeyIsStream` are true after the default is applied.
- `relay/channel/codex`: regression test that compact requests do not receive the default.
- `relay/channel/codex`: regression test that `StreamOptions` is dropped before forwarding to Codex.

#### 7. Wrong vs Correct

Wrong:

```go
if request.Stream == nil {
    request.Stream = common.GetPointer(true)
}
// info.IsStream remains false; response handling/logging are inconsistent.
```

Correct:

```go
if request.Stream == nil {
    request.Stream = common.GetPointer(true)
}
if request.Stream != nil && *request.Stream {
    info.IsStream = true
    c.Set(string(constant.ContextKeyIsStream), true)
}
```

### Codex account type cache during channel tests

#### 1. Scope / Trigger

Codex channel rows display account plan (`free`, `plus`, `pro`, `team`,
`enterprise`) from `channel.other_info.codex_account_type` when it is already
cached. If an account upgrades from Free to Plus, the cached value must be
refreshed by successful channel tests, not only by the manual usage dialog.

#### 2. Signatures

- Backend helper: `syncCodexChannelAccountTypeFromUsage(ctx, channel)`.
- Persisted DB field: `channels.other_info` JSON key
  `codex_account_type`, plus `codex_account_type_updated_at`.
- Upstream probe: `GET <channel_base_url>/backend-api/wham/usage` with the
  Codex OAuth `access_token` and `chatgpt-account-id`.

#### 3. Contracts

- Only single-key Codex channels (`ChannelTypeCodex`) participate.
- Successful channel tests must attempt a usage probe and persist the
  normalized plan type when the upstream usage response contains a supported
  value.
- Usage-probe failures must not turn an otherwise successful channel test into
  a failed test.

#### 4. Validation & Error Matrix

- Non-Codex or multi-key channel -> skip without error.
- Missing/invalid Codex OAuth key -> log diagnostics, keep the test result.
- Usage upstream non-2xx or malformed JSON -> log diagnostics, keep the test
  result.
- Unsupported/missing `plan_type` -> leave cached account type unchanged.

#### 5. Good/Base/Bad Cases

- Good: cached `free` plus usage payload `{"plan_type":"plus"}` updates
  `other_info.codex_account_type` to `plus`.
- Base: ordinary OpenAI channel test does not perform any Codex usage probe.
- Bad: only updating the React Query result while leaving `other_info` as
  `free`; refreshed channel lists will still show the stale cached value.

#### 6. Tests Required

- `controller`: regression test that `syncCodexChannelAccountTypeFromUsage`
  persists a plan change from `free` to `plus`.
- `controller`: regression test that non-Codex channels are ignored.

#### 7. Wrong vs Correct

Wrong:

```go
// Successful test updates only response_time/status_code.
channel.UpdateResponseTime(milliseconds)
```

Correct:

```go
syncCodexChannelAccountTypeAfterSuccessfulTest(ctx, channel)
channel.UpdateResponseTime(milliseconds)
```

### Channel test response time semantics

Channel test `response_time` is used by operators to compare perceived channel latency in the channel list.

- Non-streaming channel tests store the full test elapsed time in `channel.response_time`.
- Streaming channel tests store time-to-first-body-write (TTFT) in `channel.response_time`, because waiting for the full stream completion overstates perceived streaming latency.
- If a streaming test does not observe a first body write, fall back to the full elapsed test time instead of writing zero.
- Consume logs for channel tests may continue to record the full test duration; do not silently reinterpret billing/log elapsed time as TTFT.
- Add focused controller tests when changing this behavior.

### OAuth provider callback parity

OAuth providers that use the frontend callback page must keep the authorization
URL and token-exchange `redirect_uri` byte-for-byte compatible.

- Frontend authorization builders under `web/default/src/lib/oauth.ts` should
  send users to the provider with `redirect_uri=<public origin>/oauth/<provider>`
  when the provider requires or validates the redirect URI.
- Backend providers under `oauth/` must send the same `/oauth/<provider>`
  redirect URI during token exchange. Do not switch to `/api/oauth/<provider>`
  unless the authorization URL also uses that API callback directly.
- When the application is behind a reverse proxy, backend redirect URI builders
  should honor `Forwarded`, `X-Forwarded-Proto`, and `X-Forwarded-Host` before
  falling back to the request host/TLS state.
- Token and userinfo response decoding in touched OAuth providers should use
  `common.Unmarshal` / `common.DecodeJson`, not new `encoding/json` decoder
  calls.
- Add focused provider tests that assert auth style, redirect URI, non-2xx
  provider responses, and provider-specific account gating such as trust level
  or suspended/silenced status.

---

### ChatGPT Web Responses endpoint compatibility

#### 1. Scope / Trigger

ChatGPT Web (`relay/channel/chatgptimg`) is backed by the ChatGPT web
conversation API, not a native OpenAI Responses upstream. When a selected
ChatGPT Web channel receives `/v1/responses`, the adapter must emulate the
Responses contract locally instead of returning "endpoint not supported".

#### 2. Signatures

- Request converter:
  `Adaptor.ConvertOpenAIResponsesRequest(c, info, dto.OpenAIResponsesRequest)`.
- Response handler branch:
  `Adaptor.DoResponse(c, resp, info)` with
  `info.RelayMode == relayconstant.RelayModeResponses`.
- Synthetic response stream events use `dto.ResponsesStreamResponse`.

#### 3. Contracts

- Convert `input` and `instructions` into the existing ChatGPT Web
  `chatRequest.Messages` format so `buildChatPrompt` remains the single prompt
  construction path.
- Preserve `stream` and map `text.format` to chat `response_format` when it is
  `json_object` or `json_schema`.
- Recover ChatGPT Web conversation continuity from `conversation_id`,
  `conversation.id`, metadata conversation fields, or the synthetic
  `previous_response_id` prefixes (`resp_chatgptimg-*`,
  `chatcmpl-chatgptimg-*`).
- Return `/v1/responses` shaped JSON/SSE to clients; do not leak
  `chat.completion` bodies on a Responses endpoint.
- When `/v1/responses` includes local function tools and the latest user
  message clearly asks for a local file/directory operation, ChatGPT Web may
  return an empty assistant message instead of a tool call. The adapter should
  use the same preemptive local tool-call bridge as Claude-compatible messages
  and emit a Responses `function_call` item directly, rather than sending the
  prompt to ChatGPT Web and completing with empty text.
- When `/v1/messages` is bridged through OpenAI Responses, the stream bridge
  must recover assistant text from both incremental `response.output_text.delta`
  events and terminal `response.output_item.done` / `response.completed`
  message outputs. Some synthetic or upstream Responses streams only carry the
  final assistant text in `item.content[].text`; dropping that text produces a
  successful Claude stream with only `message_start` / `message_stop`.
- Do not use plain text image-intent keyword heuristics for `/v1/responses`
  text models. Responses clients often discuss image generation as a text task;
  treating those words as a generation request buffers the stream and can wait
  on ChatGPT Web image polling for about a minute. Responses mode should only
  poll for chat-generated images when the model itself is an image model or the
  upstream SSE explicitly reports image generation.
- For `/v1/chat/completions`, plain text image-intent heuristics must inspect
  the latest user-role message in `chatRequest.Messages`, not the rendered
  prompt string and not assistant output text. Rendered prompts contain history,
  system/tool instructions, and assistant text; using them as intent can inject
  image-generation instructions or trigger the final image-poll timeout after a
  normal text answer. Image models and explicit upstream image-generation SSE
  markers still enable image polling.
- Chat-generated image polling for `/v1/chat/completions` text streams is a
  best-effort tail step and must keep `chatGPTWebChatImagePollMaxWait` at 30
  seconds unless product requirements explicitly accept longer final-stream
  latency. Full image generation endpoints use the separate `image_poll_ms`
  lifecycle and are not constrained by this chat-tail timeout.
- ChatGPT Web chat stream parsing must collect generated image references from
  non-user SSE messages before falling back to conversation mapping or polling.
  Track `file-service://...` as file ids and `sediment://...` as `sed:<id>`
  refs, filter refs that existed in the pre-request baseline, and skip
  user-authored upload refs so reference images are not returned as generated
  images. These SSE refs are ChatGPT Web internal pointers; use them as a
  readiness/polling hint, not as the final API-visible image source. Request
  clients should receive image Markdown only after the adapter re-collects the
  result through conversation mapping/polling and resolves it to a downloadable
  or gateway-hosted URL.
- When the upstream stream explicitly reports image generation, the chat-tail
  collector should use the dedicated longer wait for final refs, but it must
  not hold a downloadable stable sediment preview until the whole max-wait
  expires. Once sediment refs are stable for the configured rounds, the
  sediment download endpoint is ready, and `PreviewWait` has elapsed, return the
  preview refs immediately. This avoids returning after the first unstable
  preview ref while also avoiding 30s+ tail latency after the Web UI already has
  a usable image.
- ChatGPT Web session-route reuse is an optimization, not a hard dependency. If
  a cached conversation is readable but `POST /backend-api/f/conversation`
  returns upstream `403` while opening the stream, clear that route cache entry
  and retry once with the stored full-context fallback prompt as a fresh
  conversation. Do not loop indefinitely, and do not apply this fallback to
  non-route fresh-conversation `403` errors.

#### 4. Validation & Error Matrix

- Empty `input` and empty `instructions` -> converter returns
  `chatgpt web channel: responses input is required`.
- Unsupported Responses-only features (tools/background/etc.) -> best-effort
  text conversion unless the existing chat prompt path rejects the request.
- Upstream ChatGPT Web stream/poll errors -> propagate through the existing
  ChatGPT Web error path and relay-compatible renderer.
- `/v1/responses` text model prompt contains words such as "generate image" or
  "生成图片", but upstream does not report image generation -> stream text
  normally and skip image polling.
- `/v1/responses` image model, or upstream SSE reports image generation ->
  preserve image generation instructions and image polling.
- `/v1/chat/completions` latest user message asks to create/draw/generate an
  image -> inject the ChatGPT Web image-generation instruction and allow image
  polling.
- `/v1/chat/completions` latest user message is normal text, while assistant
  history or final assistant output merely mentions image generation -> do not
  inject image-generation instructions and do not enable best-effort image
  polling unless the upstream stream reports an image-generation marker.
- ChatGPT Web SSE includes generated image refs in a non-user message -> record
  the refs as a hint that image generation happened, then collect the API-visible
  image result via conversation mapping/polling. Do not materialize the raw SSE
  `file-service://` or `sediment://` pointer directly for request clients.
- ChatGPT Web chat-tail image collection should not stop at the first preview
  sediment when `has_image_generation=true`; it must wait for a stable sediment
  set long enough to capture multi-image responses and late-arriving final refs.
- ChatGPT Web stable sediment preview with a resolvable download URL -> return
  `preview_only` before `MaxWait` instead of waiting for final `file-service`
  refs that may never appear in API-visible form.
- ChatGPT Web cached conversation route hit + `f/conversation` stream-open
  `403` + available `FallbackPrompt` -> clear the stale session route, send one
  fresh full-context retry without `conversation_id`, and record timing such as
  `session_route_retry_full_context`, `session_route_retry_reason=stream_403`,
  `session_route_cleared`, and `session_route_retry_success`.
- ChatGPT Web fresh conversation `f/conversation` `403`, or route hit without a
  full-context fallback prompt -> propagate the upstream error through the
  normal relay error path; do not retry locally because there is no stale route
  to repair.
- Responses stream has no `response.output_text.delta` but does include a
  completed message item with `content[0].type="output_text"` -> Claude
  `/v1/messages` clients still receive a `content_block_delta` before
  `message_stop`.
- Responses request has tools and latest user text is a local file read/list
  intent -> return a synthetic Responses `function_call`; do not let the ChatGPT
  Web upstream produce a successful empty text response.

#### 5. Good/Base/Bad Cases

- Good: `/v1/responses` with `input: "hello"` on a ChatGPT Web text model
  returns an OpenAI Responses object with `output_text`.
- Good: streaming `/v1/responses` emits `response.output_text.delta`,
  `response.output_item.done`, `response.completed`, then `[DONE]`.
- Good: Claude `/v1/messages` via the Responses bridge receives text even when
  the final message item is the only event carrying `output_text`.
- Good: `/v1/responses` with `tools` and `input: "读取 AGENTS.md"` emits a
  `function_call` item for the matching local tool, so Codex/Claude-style
  clients can execute the tool and continue the turn.
- Base: `/v1/chat/completions` still returns chat completion chunks; only the
  source of text image-intent detection is constrained to latest user text.
- Good: a reused ChatGPT Web session route whose cached conversation can no
  longer be continued falls back to a fresh full-context turn once, then records
  the new conversation id at normal stream completion.
- Bad: returning raw `chat.completion` JSON/SSE from a `/v1/responses` request;
  Responses clients will treat it as malformed.
- Bad: using the chat-completions text keyword heuristic for Responses text
  models; this makes ordinary text analysis prompts wait for image polling.
- Bad: scanning rendered prompts or assistant output for image-generation words;
  ordinary explanations such as "I can help generate images" can otherwise
  trigger `allow_image_poll=true` and add roughly a minute of final-stream
  latency.
- Bad: exposing or directly materializing ChatGPT Web SSE `file-service://` /
  `sediment://` refs as the API image result; those pointers are web-internal
  and may fail outside the web conversation flow.
- Bad: ignoring image refs already present in ChatGPT Web SSE events as a signal
  that generation happened; this can miss image-only SSE turns or make the relay
  treat them as empty responses.
- Bad: waiting until `MaxWait` just because only stable `sediment://` refs are
  present; if the sediment attachment download URL is already available, the
  client should receive the gateway-hosted image without a 30s+ tail delay.
- Bad: repeatedly retrying a stale cached `conversation_id` after upstream
  `403`; this can turn an otherwise recoverable turn into repeated
  `1142->1142->1142` style channel failures.

#### 6. Tests Required

- `relay/channel/chatgptimg`: converter test for instructions, multimodal
  input, `text.format`, stream flag, and synthetic previous response id.
- `relay/channel/chatgptimg`: response handler test proving Responses mode uses
  `OaiResponsesHandler` and returns usage from `input_tokens/output_tokens`.
- `relay/channel/chatgptimg`: stream test proving emitted chunks are Responses
  events, not chat completion chunks.
- `relay/channel/chatgptimg`: regression tests proving Responses text prompts
  do not inject image-generation instructions or enable image polling purely
  from text keywords, while chat-completions latest-user image prompts and
  Responses image models still do.
- `relay/channel/chatgptimg`: regression tests proving assistant history/output
  that mentions image generation does not inject image-generation instructions
  or enable image polling for a normal latest-user text request.
- `relay/channel/chatgptimg`: regression tests proving ChatGPT Web chat SSE
  captures generated file/sediment refs from non-user messages, ignores
  user-uploaded refs, filters baseline refs, and uses SSE refs as conversation
  collection hints rather than directly downloading the raw web-internal refs.
- `relay/channel/chatgptimg`: regression tests proving stable sediment polling
  waits for late-arriving refs when the upstream stream reports real image
  generation, instead of returning the first preview-only ref set too early.
- `relay/channel/chatgptimg`: regression test proving downloadable stable
  sediment previews return before the full poll timeout.
- `relay/channel/chatgptimg`: regression test proving cached-conversation
  stream-open `403` clears the session route and retries exactly once with the
  full-context fallback prompt and no `conversation_id`.
- `relay/channel/openai`: regression test that a Claude stream converted from a
  Responses stream with only `response.output_item.done` message text emits
  `content_block_delta` and does not stop as an empty message.
- `relay/channel/chatgptimg`: regression test that preemptive local tool
  bridging is allowed for OpenAI Responses relay mode, not only Claude relay
  format.

#### 7. Wrong vs Correct

Wrong:

```go
func (a *Adaptor) ConvertOpenAIResponsesRequest(...) (any, error) {
    return nil, errors.New("chatgpt web channel: /v1/responses endpoint not supported")
}
```

Correct:

```go
// Convert Responses input to chatRequest, send it through ChatGPT Web chat,
// then wrap the synthetic result back into Responses JSON/SSE before billing.
```

Wrong:

```go
case "response.output_item.done":
    if streamResp.Item.Type != "function_call" {
        break // drops item.type="message" text-only completions
    }
```

Correct:

```go
case "response.output_item.done":
    if streamResp.Item.Type == "message" {
        sendMissingOutputTextDelta(streamResp.Item.Content)
        break
    }
```

---

### ChatGPT Web deep research internal events

#### 1. Scope / Trigger

- Trigger: changes to `relay/channel/chatgptimg` ChatGPT Web deep-research
  request payloads, SSE patch parsing, conversation mapping recovery, or
  playground deep-research output.

#### 2. Signatures

- Request flag: `chatgpt_web_deep_research` / `deep_research` on compatible
  chat payloads.
- Upstream connector hint:
  `connector:connector_openai_deep_research`.
- SSE/parser entry points:
  `CollectChatSSEEvent`, `collectChatPatchEvent`, `normalizeChatAssistantContent`,
  and `ExtractLatestAssistantTextFromConversation`.

#### 3. Contracts

- Enabling deep research must send ChatGPT Web `system_hints` and user-message
  metadata for `connector:connector_openai_deep_research`.
- ChatGPT Web may stream an internal connector payload such as
  `{"path":"/Deep Research App/implicit_link::connector_openai_deep_research/start",...}`.
- That internal `implicit_link` payload is not assistant text and must not be
  forwarded to OpenAI-compatible clients or the playground.
- Suppression must handle both complete message snapshots and split SSE patch
  deltas. Later normal assistant text in the same stream must still be emitted.
- Some streams expose the internal payload as a partial `message` snapshot
  before the JSON object is closed. Snapshot recovery must use the same
  stateful suppression path as patch deltas; line-based filtering is not
  sufficient.
- Ordinary explanatory text that merely mentions
  `implicit_link::connector_openai_deep_research` must be preserved unless it is
  the ChatGPT Web internal `path` JSON object.
- If the upstream stream contains only the deep-research internal start event
  and no final assistant text, return a user-facing pending/status message
  instead of an empty completion.
- Before returning that pending/status message, poll the ChatGPT Web
  conversation mapping for a bounded period when a conversation id is available.
  Deep Research often completes asynchronously after the SSE start event.
- ChatGPT Web may resolve the Deep Research connector to an embedded UI widget
  instead of plain assistant text. If conversation mapping contains an
  `api_tool.widget_state` message with `report_message`, extract that report as
  assistant text; otherwise return an explicit compatibility message rather
  than promising that the same stream will eventually contain a report.
- ChatGPT Web text responses may return a `stream_handoff` event followed by
  `[DONE]` before text deltas are delivered. Treat this as an asynchronous
  handoff, poll conversation mapping briefly, and stream the recovered assistant
  text when it appears.

#### 4. Validation & Error Matrix

- Deep-research internal JSON as one complete line -> remove the line from
  recovered assistant text.
- Deep-research internal JSON split across patch deltas -> suppress every
  fragment until the JSON object is balanced.
- Deep-research internal JSON exposed as a partial message snapshot -> suppress
  it before streaming any delta to clients.
- Normal text after the internal JSON -> emit normally.
- Plain prose mentioning the connector marker -> preserve normally.
- Internal start event with no final report in the same stream -> emit the
  gateway pending/status message.
- Internal start event with no final report yet, but conversation mapping later
  contains final assistant text within the bounded wait -> stream/return that
  final text instead of the pending/status message.
- Internal start event followed by a tool message saying an embedded Deep
  Research UI was displayed, but without `report_message` in widget state ->
  return an explicit compatibility message and do not expose the tool/internal
  JSON as assistant text.
- Internal start event followed by an `api_tool.widget_state.report_message` ->
  return the report message text.
- `stream_handoff` with no text deltas -> wait briefly for conversation mapping
  and return the recovered assistant text instead of an empty completion.

#### 5. Good/Base/Bad Cases

- Good: playground deep research no longer displays raw
  `/Deep Research App/implicit_link::.../start` JSON.
- Good: a later `研究结果已完成` delta after the internal object reaches the
  client as assistant text.
- Good: if no later text arrives, the client sees a pending/status message
  rather than an empty assistant response.
- Base: normal ChatGPT Web chat streams remain unchanged.
- Bad: hiding any text that contains the connector string, because users or
  developers may discuss the marker as ordinary text.

#### 6. Tests Required

- `relay/channel/chatgptimg`: regression test for split deep-research
  `implicit_link` patch suppression.
- `relay/channel/chatgptimg`: regression test for complete mapping/message
  content line suppression.
- `relay/channel/chatgptimg`: regression test that ordinary explanatory text
  containing the connector marker is preserved.

#### 7. Wrong vs Correct

Wrong:

```go
// Treats every `/message/content/parts` patch value as assistant text.
return appendNormalizedChatContent(state, value), false
```

Correct:

```go
// Filter ChatGPT Web internal Deep Research connector payloads before
// appending user-visible assistant text.
value = filterChatGPTWebDeepResearchInternalContent(state, value)
return appendNormalizedChatContent(state, value), false
```

---

### ChatGPT Web latency observability and client reuse

#### 1. Scope / Trigger

Trigger: changes to `relay/channel/chatgptimg` request setup, ChatGPT Web
requirements/PoW flow, image polling/materialization, or consume-log `Other`
generation for ChatGPT Web channels.

#### 2. Signatures

- Request-scoped timing container:
  `service.NewChatGPTWebTiming()`,
  `service.SetChatGPTWebTiming(ctx, timing)`.
- Consume-log append point:
  `service.GenerateTextOtherInfo(...)` writes `other.chatgpt_web_timing`
  when a ChatGPT Web adapter installed timing data on the Gin context.
- Client reuse helper:
  `getCachedClient(opt ClientOptions) (*Client, bool, error)`.
- Requirements flow:
  `(*Client).ChatRequirementsV2(ctx, timings ...*service.ChatGPTWebTiming)`.

#### 3. Contracts

- Timing fields must be coarse diagnostics only: phase durations in
  milliseconds, booleans, counts, request kind, model name, and non-sensitive
  status labels.
- Timing fields must not include prompts, access tokens, refresh/session
  tokens, cookies, auth headers, signed download URLs, full upstream request
  bodies, or full upstream response bodies.
- Client cache keys must isolate credentials and browser identity. At minimum
  include base URL, proxy, access-token fingerprint, device id, session id,
  user-agent/client version/language, and timeout settings.
- Reuse a proof token returned by `ChatRequirementsV2` for the following
  conversation request in the same request setup. Do not solve the same PoW
  twice in the normal V2 path.
- Do not cache or reuse ChatGPT Web `chat-requirements` tokens across separate
  relay requests, even when reusing the same HTTP client/cookie jar. Treat the
  requirements token from both V2 finalize and legacy fallback as request-local
  input for the immediately following `/backend-api/f/conversation` call.
- Preserve fallback behavior: if prepare/finalize fails, fall back to the
  legacy requirements endpoint and allow the caller to solve PoW when the
  fallback response requires it.

#### 4. Validation & Error Matrix

- Missing timing container -> request behavior is unchanged and no
  `chatgpt_web_timing` field is logged.
- Cache miss or expired entry -> create a fresh `Client` and log
  `client_cache_hit=false` when timing exists.
- Matching active cache entry -> reuse `Client` and log
  `client_cache_hit=true` when timing exists.
- Different access token/device/session/proxy/base URL -> must not reuse the
  cached client from another account or browser identity.
- Every `ChatRequirementsV2` call must fetch fresh requirements through
  prepare/finalize or the legacy fallback path; `requirements_cache_hit=true`
  before an `f/conversation` 403 indicates stale request-local state leaked
  across turns.
- V2 prepare/finalize error -> fallback to legacy requirements and record
  fallback timing/status when timing exists.

#### 5. Good/Base/Bad Cases

- Good: an image request records `requirements_*`, `stream_open_ms`,
  `image_poll_ms`, `image_fetch_ms`, and `image_run_total_ms` without logging
  the prompt or signed URLs.
- Good: repeated requests for the same ChatGPT Web credential reuse the
  transport/cookie jar while different credentials remain isolated.
- Good: a second chat turn on a reused client obtains a new requirements token
  before posting `/backend-api/f/conversation`.
- Base: non-ChatGPT Web channels have no `chatgpt_web_timing` in consume-log
  `Other`.
- Bad: using a process-global client keyed only by base URL; that can mix
  cookies across accounts.
- Bad: storing a legacy fallback `chat-requirements` token on the client and
  sending it on the next independent chat/image request; ChatGPT Web may reject
  the subsequent `/backend-api/f/conversation` with 403.
- Bad: adding prompt text or upstream signed image URLs to timing fields; that
  violates log-safety rules.

#### 6. Tests Required

- `service`: `GenerateTextOtherInfo` includes `chatgpt_web_timing` only when a
  timing container was attached to the Gin context.
- `relay/channel/chatgptimg`: client-cache test proves matching options reuse
  the same client and changed auth token creates an isolated client.
- `relay/channel/chatgptimg`: requirements regression test proves a fallback
  `chat-requirements` token is not reused by the next `ChatRequirementsV2`
  call on the same client.
- `relay/channel/chatgptimg`: keep existing ChatGPT Web conversion and image
  materialization tests passing after adding timing parameters.

#### 7. Wrong vs Correct

Wrong:

```go
other["chatgpt_web_debug"] = map[string]any{
    "prompt": prompt,
    "signed_url": signedURL,
}
```

Correct:

```go
timing.ObserveSince("image_poll_ms", pollStart)
timing.Set("image_ref_count", len(fileRefs))
```

---

### Performance metrics summary and model square status

#### 1. Scope / Trigger

- Trigger: changes to `pkg/perf_metrics`, `model/perf_metric.go`,
  `controller/perf_metrics.go`, `/api/perf-metrics/summary`, or the default
  frontend model square/status rendering.

#### 2. Contracts

- The model square status indicator and the model details performance chart
  must use the same bucketed success-rate semantics. Do not recreate a
  different "health bar" from only the aggregate `success_rate`.
- `/api/perf-metrics/summary` should include a per-model `series` array of
  bucket points when bucketed data exists. Aggregate fields such as
  `avg_latency_ms`, `success_rate`, and `avg_tps` remain the card summary, but
  the miniature status chart uses `series`.
- Bucket series must be sorted by bucket timestamp ascending before reaching
  frontend chart components.
- Hot in-memory buckets and persisted DB buckets must merge by
  `{model_name, bucket_ts}` before model totals and chart series are derived.

#### 3. Good/Base/Bad Cases

- Good: the model square shows the same recent success-rate trend as the
  details performance uptime/success chart, just in a compact form.
- Base: when no bucket series exists, the compact chart renders the existing
  empty placeholder instead of fabricating status segments from aggregate data.
- Bad: a five-segment status strip based only on the last 24h aggregate success
  rate; it can disagree with the details page and hides bucket-level failures.

#### 4. Tests Required

- `pkg/perf_metrics`: unit test proving summary models include sorted bucket
  `series`, aggregate success rate is still calculated from totals, and models
  remain ordered by request count.
- `web/default`: typecheck/lint/build after changing summary API types or model
  square rendering.

---

### Billing expression changes

Before changing expression-based/tiered billing, read `pkg/billingexpr/expr.md`. It documents expression variables, token normalization, pre-consume/settlement flow, quota conversion, and expression versioning.

---

## Testing and Verification

There is no top-level `Makefile`; the repository has a lowercase `makefile`.

Observed commands:

- Backend build/run: `go run main.go`, `go build ...`
- Backend tests: `go test ./...`
- Frontend default: `cd web/default && bun run typecheck`, `bun run lint`, `bun run build`
- Frontend classic: `cd web/classic && bun run build`
- Combined dev/build helpers: lowercase `makefile` targets such as `build-frontend`, `build-frontend-classic`, `build-all-frontends`, `dev-api`, `dev-web`, `dev`

Use targeted verification first:

- Changed Go package: `go test ./<package>`
- Relay/channel change: relevant `relay/...` package tests
- DTO zero-value semantics: relevant `dto/*_test.go`
- DB/model change: relevant `model`/`controller`/`service` tests and, when possible, SQLite-backed tests
- Frontend change: use Bun commands from `web/default/package.json`

Then run broader checks when the change scope warrants it.

Representative tests already exist in:

- `common/json_test.go`, `common/url_validator_test.go`;
- `controller/*_test.go`;
- `dto/*_test.go`;
- `model/*_test.go`;
- `relay/**/*_test.go`;
- `service/*_test.go`;
- `setting/**/*_test.go`;
- `pkg/billingexpr/billingexpr_test.go`.

---

## Code Review Checklist

Before considering backend work complete, check:

- [ ] Route registration, controller logic, service logic, and model persistence are in the appropriate layers.
- [ ] JSON marshal/unmarshal uses `common.*` in new business code.
- [ ] Optional upstream scalar fields preserve explicit zero/false values.
- [ ] Database code is cross-compatible or branches on `common.UsingSQLite`, `common.UsingMySQL`, `common.UsingPostgreSQL`.
- [ ] Raw SQL uses placeholders for values and allowlisted/quoted column names.
- [ ] Errors match the surrounding API family: dashboard `success/message/data` vs relay-compatible error shape.
- [ ] Logs do not expose secrets, auth headers, channel keys, tokens, cookies, payment secrets, or full sensitive payloads.
- [ ] New channel/provider behavior has focused tests and StreamOptions support is considered.
- [ ] Billing expression behavior follows `pkg/billingexpr/expr.md`.
- [ ] Frontend changes under `web/default` follow `web/default/AGENTS.md`, use Bun, and update i18n keys/translations.

---

## Forbidden Patterns

- Directly removing or renaming protected project/organization branding or metadata.
- Adding DB-specific SQL without fallbacks for the other supported databases.
- Using SQLite-unsupported `ALTER COLUMN` migrations.
- Manually using `AUTO_INCREMENT` or `SERIAL` instead of GORM primary key handling.
- Returning raw upstream errors containing secrets or full request context to clients.
- Adding new JSON marshal/unmarshal calls in new business code via `encoding/json` when `common.*` can be used.
- Using non-pointer optional scalar fields with `omitempty` in upstream relay request DTOs.
- Adding new frontend dependencies or package-manager commands that bypass Bun for `web/default`.

---

## Existing Technical Debt and Compatibility Notes

This repository contains legacy and provider-specific code that may not fully match newer project rules, especially direct `encoding/json` calls in OAuth/provider adapters and some controller code. Treat these as existing compatibility debt, not a pattern to copy into new code.

When editing nearby legacy code:

1. Preserve behavior first.
2. Prefer local cleanup only when it is clearly safe and covered by tests.
3. Avoid broad opportunistic rewrites.
4. Add focused regression tests for the behavior being touched.

---

## PR and Contribution Expectations

`.github/PULL_REQUEST_TEMPLATE.md` requires:

- human-written summary;
- focused scope;
- local verification evidence;
- no sensitive credentials;
- clear proof of work.

`.github/workflows/pr-check.yml` rejects low-quality AI-generated PRs. Keep generated summaries concise, reviewed, and evidence-based.
