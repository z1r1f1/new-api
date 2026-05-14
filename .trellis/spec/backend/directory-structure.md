# Directory Structure

> Backend organization conventions observed in this repository.

---

## Overview

The backend follows a layered Go/Gin/GORM structure:

```text
router/      -> HTTP route registration and middleware chains
controller/  -> Gin handlers, request parsing, HTTP responses
service/     -> business logic, billing, token counting, external HTTP helpers
model/       -> GORM models, migrations, database reads/writes, persistence helpers
relay/       -> AI provider relay orchestration and provider-specific adapters
middleware/  -> authentication, CORS, decompression, rate limits, channel distribution
setting/     -> runtime settings and config serialization helpers
common/      -> shared primitives: JSON wrappers, env, cache, Redis, quota, validation
dto/         -> request/response DTOs used across controllers, services, and relay
constant/    -> stable constants and context keys
types/       -> cross-layer type definitions, especially relay error/request types
i18n/        -> backend translation messages
oauth/       -> OAuth provider implementations
pkg/         -> internal packages with clearer standalone boundaries
logger/      -> request-aware logger implementation
```

Representative files:

- `router/api-router.go` groups dashboard/API routes under `/api` and attaches middleware such as `UserAuth`, `AdminAuth`, `RootAuth`, rate limits, gzip, and body cleanup.
- `router/relay-router.go` registers OpenAI/Claude/Gemini-compatible relay routes and routes all relay calls through `controller.Relay`.
- `controller/channel.go` is a large admin API handler file that parses query/body data, calls `model`/`service`, and returns `gin.H` JSON responses.
- `service/billing_session.go`, `service/pre_consume_quota.go`, and `service/tiered_settle.go` hold business rules that should not live in route files.
- `model/main.go` initializes GORM, chooses SQLite/MySQL/PostgreSQL, and runs migrations.
- `relay/channel/*` contains provider adapters such as `relay/channel/openai`, `relay/channel/claude`, `relay/channel/gemini`, and task/video adapters under `relay/channel/task/*`.

---

## Layer Responsibilities

### `router/`

Use `router/` only for route registration and middleware composition.

Patterns:

- Register API route groups in `router/api-router.go`.
- Register relay-compatible endpoint families in `router/relay-router.go`.
- Keep inline handlers tiny; delegate actual behavior to `controller` or `relay`.
- Attach auth/rate-limit/distribution middleware at the route group closest to the protected endpoints.

Example: `router/relay-router.go` maps `/v1/chat/completions`, `/v1/responses`, `/v1/messages`, image/audio/embedding/rerank routes, and model routes to `controller.Relay` with the appropriate `types.RelayFormat`.

### `controller/`

Use `controller/` for Gin handlers and HTTP-level concerns:

- read query/path/body parameters;
- call `service` or `model`;
- translate domain errors into HTTP response bodies;
- set response status and shape.

Most dashboard/admin responses use `c.JSON(..., gin.H{"success": bool, "message": string, "data": ...})`, often with `http.StatusOK` even for business-level failures. Relay responses instead preserve OpenAI/Claude-compatible error shapes; see `controller/relay.go`.

Keep new handlers near existing feature files, for example:

- channel management: `controller/channel.go`, `controller/channel-test.go`;
- user/auth: `controller/user.go`, `controller/oauth.go`, `controller/passkey.go`;
- subscription/payment: `controller/subscription*.go`, `controller/topup_*.go`;
- relay entry points: `controller/relay.go`.

### `service/`

Use `service/` for reusable business logic and workflows that are not HTTP-specific:

- quota pre-consumption/settlement: `service/pre_consume_quota.go`, `service/billing_session.go`, `service/tiered_settle.go`;
- token counting and text quota: `service/token_counter.go`, `service/text_quota.go`;
- channel selection and affinity: `service/channel_select.go`, `service/channel_affinity.go`;
- external HTTP helpers: `service/http.go`, `service/http_client.go`;
- provider-agnostic request conversion: `service/convert.go`, `service/openaicompat/*`.

Service functions may receive `context.Context`/`*gin.Context` when request-scoped logging, translations, or context keys are needed, but they should not register routes.

### `model/`

Use `model/` for persistence:

- GORM structs and database tags;
- migrations and schema compatibility;
- CRUD/query helpers;
- transaction boundaries for multi-row or quota-affecting updates.

Examples:

- `model/main.go` owns DB selection, connection pool settings, `AutoMigrate`, and DB-specific migration branches.
- `model/channel.go`, `model/token.go`, `model/user.go`, `model/subscription.go`, and `model/topup.go` combine model definitions with query/update helpers.
- `model/log.go` defines usage/error/management logs and writes to `LOG_DB`.

Do not put HTTP response formatting in `model/`.

### `relay/`

Use `relay/` for AI gateway behavior:

- `relay/*_handler.go` orchestrates relay modes and provider-independent handling.
- `relay/common/` stores relay request metadata and cross-provider helpers.
- `relay/helper/` handles request validation, response streaming, billing expression extraction, and pricing helpers.
- `relay/channel/<provider>/` contains provider-specific conversions, request construction, and response parsing.

When adding a provider/channel, follow existing adapter layout under `relay/channel/*`, and update stream option support if the provider supports it.

### `setting/`

Use `setting/` for runtime configuration domains and string/JSON conversion helpers. Examples:

- `setting/ratio_setting/*` for model/group/cache/audio/image ratios;
- `setting/operation_setting/*` for payment, quota display, and operational toggles;
- `setting/system_setting/*`, `setting/performance_setting/*`, and `setting/config/*` for system/performance/config structures.

### `common/`, `constant/`, `types/`, `dto/`

- `common/` is for low-level shared utilities only. Prefer existing helpers before adding a new utility.
- `constant/` stores stable enum-like values and context keys shared across layers.
- `types/` stores cross-layer types, especially relay error and request metadata types.
- `dto/` stores JSON DTOs. For upstream relay request DTOs, optional scalar request fields must preserve explicit zero values by using pointer types with `omitempty`.

---

## Where to Put New Work

| Work type | Put it in |
|---|---|
| New HTTP endpoint | `router/*` for route registration, `controller/<feature>.go` for handler |
| New business rule reused by handlers/relay | `service/<feature>.go` |
| New DB-backed entity or query helper | `model/<entity>.go`, plus migration registration in `model/main.go` when needed |
| New provider adapter | `relay/channel/<provider>/` plus relay registration/mapping as needed |
| New relay-wide helper | `relay/common/` or `relay/helper/` depending on whether it is metadata/core helper vs request/response helper |
| New config domain | `setting/<domain>_setting/` or existing setting package |
| New shared low-level helper | Search first; if truly shared, add to `common/` or a focused `pkg/` package |
| Backend translations | `i18n/` message definitions and translation files |
| Frontend UI | `web/default/` (default frontend) and follow `web/default/AGENTS.md` |

---

## Naming and File Patterns

- Go packages use short lowercase names.
- Most feature files use snake_case or existing feature naming (`channel_affinity.go`, `pre_consume_quota.go`, `topup_stripe.go`).
- Tests are colocated with the package as `*_test.go`.
- Large provider integrations are grouped into subdirectories, for example `relay/channel/chatgptimg/`, `relay/channel/task/gemini/`, and `service/passkey/`.

---

## Anti-Patterns to Avoid

- Do not put business rules in `router/`; keep routers declarative.
- Do not put persistence details in React/frontend code or controller response DTOs.
- Do not add a new helper without searching for an existing helper first.
- Do not duplicate provider conversion code when a relay/common/helper pattern already exists.
- Do not modify project/organization branding or metadata.
- Do not bypass established cross-layer contracts such as `router -> controller -> service -> model` or relay-compatible error shapes.
