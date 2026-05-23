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
- Normal relay timeout must be represented as `channel:response_time_exceeded` so existing `shouldRetry` / channel-disable logic can switch to another channel.
- Image generation/edit, async task, realtime/websocket, channel-test paths, and the local `codex-to-claude` bridge channel must not use the normal relay first-byte timer. Image/task requests can legitimately wait longer and have their own task/polling lifecycle; the local bridge has its own upstream lifecycle and should not be cut off by the gateway first-byte guard.
- Channel tests should use `ChannelDisableThreshold` by default, except the local `codex-to-claude` bridge channel; that bridge should not be wrapped by the channel-test timeout either.
- Outbound requests created from Gin handlers should use `http.NewRequestWithContext(ginRequestContext(c), ...)` so client cancellation and test timeouts propagate to the upstream request.

#### 4. Validation & Error Matrix

- `ChannelDisableThreshold <= 0` -> no first-byte timeout.
- Channel test for channel name `codex-to-claude` -> no channel-test timeout.
- Normal non-image relay and upstream does not start responding before the threshold -> cancel the upstream request and return `channel:response_time_exceeded`, HTTP `408`.
- Normal relay receives upstream headers before the threshold -> stop the timer; continue reading/streaming the response body normally.
- Image generation/edit, task relay, or the local `codex-to-claude` bridge channel exceeds the threshold -> do not abort via the normal first-byte timer.
- Client/request context is canceled before the outbound request -> preserve cancellation behavior; do not classify it as a channel response-time timeout unless the gateway timer fired.

#### 5. Good/Base/Bad Cases

- Good: `/v1/responses` waits 180 seconds for upstream to start responding; if no response starts, the gateway records a channel error and retries the next channel.
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
- ChatGPT Web image requests default to `response_format=b64_json` when the client does not specify a format. Image clients such as Cherry Studio expect `b64_json` to be pure base64 media data, not a gateway URL.
- Explicit `response_format` values must be preserved for normal image models. Exception: ChatGPT Web `gpt-image-2` / `chatgpt-image-2` (including model aliases mapped upstream to those names) must force `b64_json`, because downstream image-edit clients reuse the generated image bytes and fail when the gateway returns only a URL.
- The ChatGPT Web `gpt-image-2` / `chatgpt-image-2` force-to-base64 rule must be enforced at both request normalization and response construction. A stale or overridden `response_format=url` must not cause `url` to be emitted for these models.
- OpenAI-compatible image JSON must omit empty image fields. A base64 image item should serialize as `b64_json` without an empty `url` key, so clients do not choose the wrong representation for follow-up image edits.
- ChatGPT Web reference image uploads must complete the full web upload chain: `POST /backend-api/files`, blob `PUT`, `POST /backend-api/files/{file_id}/uploaded`, then `POST /backend-api/files/process_upload_stream`. The process step should store `extra.metadata_object_id` as the uploaded file's library id.
- ChatGPT Web image polling must use the conversation mapping first and periodically fall back to `POST /backend-api/files/library`, filtering by `origination_thread_id`, ready image state/category, and excluding both uploaded `file_id` and uploaded library id so reference images are not returned as generated images.
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
- Good: `gpt-image-2` request, or a mapped alias whose upstream model is `gpt-image-2`, contains `response_format=url`; ChatGPT Web overrides it to `b64_json` so follow-up image-to-image clients receive base64 media.
- Good: `gpt-image-2` response payload contains `data[].b64_json` and no `data[].url` field, even if a stale request body or override tried to force `url`.
- Good: uploaded reference images record both `file_id` and `library_file_id`; polling excludes both values and can still find generated images from `/backend-api/files/library` when the conversation mapping has not exposed a final file id yet.
- Base: image form/body omits `response_format`; ChatGPT Web image conversion defaults to `b64_json`.
- Bad: edit response returns only a gateway URL to an image-model client expecting base64; clients can throw `Invalid data content. Content string is not a base64-encoded media.`

#### 6. Tests Required

- `relay/helper`: regression test that multipart image edits preserve `response_format`.
- `relay/channel/chatgptimg`: regression tests that image generations/edits default to `b64_json` and explicit formats are preserved.
- `relay/channel/chatgptimg`: regression tests that reference uploads call `process_upload_stream`, parse `metadata_object_id`, exclude uploaded library ids, and use `/backend-api/files/library` as a generated-image fallback.
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
