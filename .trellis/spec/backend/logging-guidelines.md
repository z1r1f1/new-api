# Logging Guidelines

> Logging conventions observed in the backend.

---

## Overview

The project has two related logging systems:

1. Process/request logs written to Gin writers through `common` and `logger`.
2. Persistent usage/error/management logs stored in the database through `model.Log` and `LOG_DB`.

Representative files:

- `common/sys_log.go` writes process-level system logs.
- `logger/logger.go` writes request-aware logs with levels and request IDs.
- `model/log.go` persists user/admin/consume/error logs to `LOG_DB`.
- `controller/relay.go` logs relay failures and records error logs.
- `service/download.go` shows sensitive URL masking before logging.

---

## Process Logger

`logger/logger.go` defines request-aware log helpers:

- `logger.LogInfo(ctx, msg)`
- `logger.LogWarn(ctx, msg)`
- `logger.LogError(ctx, msg)`
- `logger.LogDebug(ctx, msg, args...)`
- `logger.LogJson(ctx, msg, obj)` for tests/debugging only

Log format:

```text
[LEVEL] 2006/01/02 - 15:04:05 | <request-id-or-SYSTEM> | <message>
```

`logger.LogDebug` only writes when `common.DebugEnabled` is true.

Use `logger.*` when a `context.Context` or `*gin.Context` is available, because it includes the request ID from `common.RequestIdKey`.

---

## System Logger

`common/sys_log.go` defines process-level helpers:

- `common.SysLog`
- `common.SysError`
- `common.FatalLog`
- `common.LogStartupSuccess`

`SysLog` and `SysError` write to Gin default writers with a `[SYS]` prefix. `FatalLog` writes `[FATAL]` and exits the process.

Use `common.SysLog` for startup, migration, background job, or system events where there is no request context. Examples:

- `model/main.go` logs database selection, migration start, setup state, and migration warnings.
- `model/log.go` logs DB failures when persistent log creation fails.
- `service/error.go` logs upstream/network-like failures before masking.

Do not use `common.FatalLog` for recoverable request-level failures.

---

## Log Output and Rotation

`logger.SetupLogger` redirects Gin writers to both stdout/stderr and a file under `common.LogDir` when a log directory is configured.

Important details:

- `common.LogWriterMu` protects concurrent writer swaps.
- Current log file path is exposed through `logger.GetCurrentLogPath`.
- Log rotation is triggered after `maxLogCount`.

When adding logging code, write through the helpers instead of writing directly to `gin.DefaultWriter` or `gin.DefaultErrorWriter`.

---

## Persistent Logs

`model/log.go` defines `Log` and log types:

- `LogTypeUnknown`
- `LogTypeTopup`
- `LogTypeConsume`
- `LogTypeManage`
- `LogTypeSystem`
- `LogTypeError`
- `LogTypeRefund`

The comment says not to use `iota` because log type values must remain stable.

Persistent log helpers include:

- `RecordLog`
- `RecordLogWithAdminInfo`
- `RecordTopupLog`
- `RecordErrorLog`
- `RecordConsumeLog`

Use persistent logs for user-visible audit/history data, billing/consume records, top-up/payment history, and relay error records. Use process logs for operational diagnostics.

`LOG_DB` may be separate from `DB`, so log schema changes belong in `migrateLOGDB` or both migration paths as appropriate.

Consume-log timing fields use mixed precision:

- `Log.UseTime` / `use_time_seconds` is stored as whole seconds for legacy UI and statistics.
- `other.frt` is stored as first-response latency in milliseconds.

When deriving `use_time_seconds` from `RelayInfo.StartTime`, round any positive millisecond remainder up to the next second. Do not use `end.Unix() - start.Unix()` for consume-log timing, because it floors total duration while `frt` keeps millisecond precision and can make the displayed first-response time appear greater than the total request time.

---

## What to Log

Log events that help operate or audit the gateway:

- database selection and migration start/failure (`model/main.go`);
- setup/bootstrap state (`model/main.go`);
- relay errors with channel/status context (`controller/relay.go`);
- persistent consume/error records (`model/log.go`);
- upstream/network errors after masking (`service/error.go`, `service/download.go`);
- background task failures and external integration errors.

For request-scoped logs, prefer messages that include stable IDs such as user ID, channel ID, token ID, model name, request ID, status code, or task ID.

---

## What Not to Log

Do not log raw secrets or sensitive payloads:

- API keys and channel keys;
- access tokens, refresh tokens, cookies, auth headers;
- payment secrets and webhook secrets;
- private user data unless explicitly required by an audit log;
- full upstream request/response bodies if they may contain user prompts or credentials.

Use `common.MaskSensitiveInfo` before logging strings that may contain secrets. Existing examples include `service.TaskErrorWrapper`, `NewAPIError` conversions in `types/error.go`, and `service/download.go`.

`model.formatUserLogs` removes admin-only fields such as `admin_info` and `stream_status` before presenting logs to users; preserve this separation between admin-only diagnostic data and user-visible logs.

### Channel Affinity Diagnostics

Channel affinity details stored under `Other.admin_info.channel_affinity` are admin-only diagnostics. They may include:

- `rule_name`, `using_group`, `selected_group`, `request_path`, and `channel_id` for route/rule context;
- `key_source`, `key_key`, and `key_path` for the matched key source metadata;
- `key_hint` and `key_fp` for safe identification of the matched key value.

Do **not** persist or display the raw matched affinity value in usage logs. Use `key_hint`/`key_fp` for value identification, and use `key_source` + `key_key`/`key_path` when the UI needs to explain which configured source matched.

## Scenario: Admin-only request headers in usage/error log details

### 1. Scope / Trigger

- Trigger: changes that add request metadata to persistent consume/error/task logs or the usage-log details UI.
- Purpose: let admins diagnose client routing/proxy issues from request headers without exposing authentication data or user-only log views.

### 2. Signatures

- Backend append helper: `service.AppendRequestHeadersAdminInfo(ctx *gin.Context, adminInfo map[string]interface{})`.
- Stored field: `Log.Other.admin_info.request_headers`, encoded as a JSON object of header name to string value.
- Frontend display: default usage-log details reads `other.admin_info.request_headers` and renders it only when `props.isAdmin` is true.

### 3. Contracts

- Request headers must be stored only under `admin_info`, never as top-level `Other.request_headers`.
- Non-admin log serialization already strips `admin_info`; keep request-header diagnostics behind that boundary.
- Filter authentication and credential-like headers before persistence, including at least authorization/proxy authorization, cookies, API-key headers, token-like headers, secret-like headers, and websocket auth protocol headers.
- Empty header names/values are skipped.

### 4. Validation & Error Matrix

- Admin log details + safe headers -> display sorted header rows.
- Non-admin log details -> no request-header rows even if the backend payload accidentally includes admin info.
- Request contains `Authorization`, `Cookie`, `Api-Key`, token, or secret headers -> those keys are absent from `admin_info.request_headers`.
- Request contains no safe headers -> omit `request_headers`.

### 5. Good/Base/Bad Cases

- Good: admins can see `Content-Type`, `User-Agent`, or `X-Client-Request-Id` in a failing request detail.
- Base: existing `admin_info.use_channel` and channel affinity diagnostics remain unchanged.
- Bad: saving request headers at top-level `Other`, because user log responses can expose top-level fields.
- Bad: persisting raw auth headers, cookies, API keys, tokens, or secrets for convenience.

### 6. Tests Required

- `service`: regression test proving `GenerateTextOtherInfo` stores safe request headers under `admin_info.request_headers`, filters sensitive headers, and does not write a top-level `request_headers` field.
- `web/default`: usage-log format test proving request-header rows are returned for admins only and blank keys/values are filtered.
- Run targeted Go service tests and default frontend typecheck after touching this contract.

### 7. Wrong vs Correct

Wrong:

```go
other["request_headers"] = headers
```

Correct:

```go
service.AppendRequestHeadersAdminInfo(c, adminInfo)
other["admin_info"] = adminInfo
```

---

## Log Levels

Use levels according to observed intent:

- `INFO`: normal operational events and persistent log creation diagnostics.
- `WARN`: unusual but non-fatal events, such as sensitive words detected.
- `ERR`: request failures, upstream errors, database write failures.
- `DEBUG`: verbose diagnostics guarded by `common.DebugEnabled`.
- `SYS`: startup/migration/background system messages without request context.
- `FATAL`: unrecoverable startup/configuration failure that should terminate the process.

---

## Common Mistakes to Avoid

- Do not use `log.Println` for new request-level code; use `logger.*` or `common.SysLog`.
- Do not expose `admin_info` or raw `Other` fields directly to non-admin users.
- Do not log unmasked URLs, keys, credentials, request headers, or prompt bodies.
- Do not create new persistent log type numeric values with `iota`.
- Do not forget that `LOG_DB` may differ from `DB`.
