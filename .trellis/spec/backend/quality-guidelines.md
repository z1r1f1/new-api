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
- Shared parser: `common.SplitIPList(raw string) []string`
- Matcher: `common.IsIpInCIDRList(ip net.IP, cidrList []string) bool`

#### 3. Contracts

- Disabled blacklist or empty list -> allow the request to continue.
- The list supports single IPs and CIDR ranges.
- Operators may separate entries with newlines, commas, or semicolons; parsing must trim whitespace and drop duplicates.
- Matched requests return HTTP 403 and abort the Gin chain before downstream middleware runs.

#### 4. Validation & Error Matrix

- `enabled=false` -> no blocking.
- `enabled=true`, `list=""` -> no blocking.
- `enabled=true`, malformed client IP -> HTTP 403 `无法解析客户端 IP 地址`.
- `enabled=true`, client IP in list -> HTTP 403 `当前 IP 已被禁止访问`.
- Invalid entries in the blacklist are ignored by `common.IsIpInCIDRList`; do not fail startup or option loading.

#### 5. Good/Base/Bad Cases

- Good: `203.0.113.8` with `203.0.113.0/24` is blocked.
- Good: `198.51.100.10` with `203.0.113.0/24` is allowed.
- Base: disabled blacklist with any list is allowed.
- Bad: adding the middleware only to `/api`, leaving `/v1` relay or frontend routes unprotected.

#### 6. Tests Required

- Unit-test separator parsing and duplicate removal.
- Middleware-test both blocked and allowed request paths.

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
- Existing explicit `additionalProperties` -> preserve the caller-provided value.

#### 5. Good/Base/Bad Cases

- Good: Claude Code `Read` schema with `file_path`, optional `offset`, and optional `limit` reaches Responses with `additionalProperties:false`, without `required:null`, and without the PDF-only `pages` field.
- Base: a valid chat function schema with an existing `additionalProperties:false` remains valid and stable.
- Bad: forwarding `required:null`, scalar `required`, open object schemas, or `Read.pages`; upstream/model may emit invalid system-tool arguments such as empty PDF `pages` or schema-external page/line parameters.
- Bad: blindly marking every property as required to satisfy strict schema mode; this can break optional local tool inputs.

#### 6. Tests Required

- `service/openaicompat`: regression test that chat function tool parameters gain `additionalProperties:false`.
- `service/openaicompat`: regression test that nil values and invalid `required` values are stripped recursively.
- `service/openaicompat`: regression test that non-object function parameters default to a closed empty object schema.
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
- Runtime stream flag: `relaycommon.RelayInfo.IsStream` plus Gin context key `constant.ContextKeyIsStream`.

#### 3. Contracts

- For Codex normal Responses requests (`RelayModeResponses`), an omitted `stream` field must be normalized to `stream:true` before sending to upstream.
- When the adaptor normalizes or receives `stream:true`, it must also mark `RelayInfo.IsStream=true` and update `ContextKeyIsStream`, so response handling, stream status tracking, and consume logs all use streaming semantics.
- `RelayModeResponsesCompact` must not get this default; compact requests keep their own endpoint semantics.
- Explicit `stream:false` remains explicit client intent and must not be silently reinterpreted as an omitted value.

#### 4. Validation & Error Matrix

- `RelayModeResponses`, `stream` omitted -> upstream body includes `"stream":true`; downstream handled as stream.
- `RelayModeResponses`, `stream:true` -> preserve true and downstream handled as stream.
- `RelayModeResponses`, `stream:false` -> preserve false; upstream may reject according to Codex backend rules.
- `RelayModeResponsesCompact`, `stream` omitted -> leave omitted.

#### 5. Good/Base/Bad Cases

- Good: Claude-compatible `/v1/messages` request without `stream` is converted through OpenAI Responses and Codex adaptor adds `stream:true` before upstream.
- Good: OpenAI-compatible `/v1/responses` request without `stream` on Codex does not fail with upstream `stream must set to be true`.
- Base: compact request to `/v1/responses/compact` remains non-stream-defaulted.
- Bad: adding only `request.Stream=true` without syncing `RelayInfo.IsStream`; response code can parse the upstream SSE with a non-stream handler or log incorrect stream state.

#### 6. Tests Required

- `relay/channel/codex`: regression test that omitted `stream` defaults to true for `RelayModeResponses`.
- `relay/channel/codex`: regression test that `RelayInfo.IsStream` and `ContextKeyIsStream` are true after the default is applied.
- `relay/channel/codex`: regression test that compact requests do not receive the default.

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
