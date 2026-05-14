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
