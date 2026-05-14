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
