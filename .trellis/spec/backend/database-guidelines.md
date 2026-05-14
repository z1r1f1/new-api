# Database Guidelines

> Database patterns and compatibility requirements observed in this project.

---

## Overview

The backend uses GORM v2 with three supported databases:

- SQLite, via `github.com/glebarez/sqlite`;
- MySQL, via `gorm.io/driver/mysql`;
- PostgreSQL, via `gorm.io/driver/postgres`.

The database entry point is `model/main.go`:

- `chooseDB` selects SQLite/MySQL/PostgreSQL from `SQL_DSN` and sets `common.UsingSQLite`, `common.UsingMySQL`, or `common.UsingPostgreSQL`.
- `InitDB` configures pool limits from environment variables and runs migrations on the master node.
- `InitLogDB` optionally uses a separate `LOG_SQL_DSN`, otherwise it points `LOG_DB` at `DB`.
- `migrateDB` and `migrateLOGDB` register models with GORM `AutoMigrate`.

New database code must remain compatible with SQLite, MySQL, and PostgreSQL unless a DB-specific branch is explicitly provided.

---

## Model Definitions

Models live in `model/` and usually combine:

- a GORM struct;
- constants or helper types;
- query/update helpers for that entity.

Examples:

- `model/channel.go` defines `Channel`, `ChannelInfo`, sort helpers, key helpers, group/model filters, and channel mutations.
- `model/token.go` defines token storage and token lookup helpers.
- `model/user.go` defines users, user settings, and user quota/group update helpers.
- `model/log.go` defines `Log` and log record helpers for `LOG_DB`.
- `model/subscription.go` defines subscription plans, orders, user subscriptions, and transactional updates.

Use GORM tags for indexes, defaults, and column types. Prefer portable tags such as `gorm:"type:text"` for JSON-like text blobs instead of database-specific JSONB-only definitions.

When a struct field is stored as JSON in a column, implement `driver.Valuer` and `sql.Scanner` and use the project JSON wrappers. Example: `model.ChannelInfo.Value` and `(*ChannelInfo).Scan` in `model/channel.go` call `common.Marshal` and `common.Unmarshal`.

---

## Query Patterns

Prefer GORM abstractions:

- `DB.Where(...).Find(...)`
- `DB.First(...)`
- `DB.Model(&T{}).Updates(...)`
- `DB.Clauses(clause.OnConflict{...}).Create(...)`
- `DB.Transaction(func(tx *gorm.DB) error { ... })`

Examples:

- `model/channel.go` uses `clause.OrderByColumn` in `ChannelSortOptions.Apply` so caller-controlled sort columns are allowlisted.
- `model/perf_metric.go` uses `clause.OnConflict` with DB-specific increment expressions.
- `model/ability.go` uses `Clauses(clause.OnConflict{DoNothing: true})` for bulk insert conflict handling.
- `model/subscription.go`, `model/topup.go`, `model/twofa.go`, and `model/passkey.go` use `DB.Transaction` for multi-step updates.

Manual `DB.Begin()`/`Commit()`/`Rollback()` also exists in older helpers such as `model/channel.go`, `model/user.go`, `model/redemption.go`, and `model/topup.go`. For new code, prefer `DB.Transaction` unless the existing surrounding code uses manual transactions.

Always return or log transaction errors. Do not ignore `Commit().Error`.

---

## Raw SQL and Cross-Database Compatibility

Raw SQL is allowed only when GORM cannot express the operation cleanly. When raw SQL is used, branch by database type where needed.

### Reserved words and booleans

`model/main.go` initializes helper variables:

- `commonGroupCol`
- `commonKeyCol`
- `commonTrueVal`
- `commonFalseVal`
- `logGroupCol`
- `logKeyCol`

Use these for reserved columns such as `group` and `key` and for boolean literal differences.

Examples:

- `model/channel.go` builds `WHERE` clauses using `commonGroupCol` and `commonKeyCol`.
- `model/user.go` selects `commonGroupCol`.
- `model/ability.go` filters by `commonGroupCol`.
- `model/token.go` uses `commonKeyCol` for token key queries.

### DB-specific string and time functions

Do not assume MySQL syntax works everywhere.

Examples:

- `model/channel.go` branches group matching: MySQL uses `CONCAT(',', group, ',')`, while SQLite/PostgreSQL use `(',' || group || ',')`.
- `model/db_time.go` branches current timestamp SQL: PostgreSQL uses `EXTRACT(EPOCH FROM NOW())::bigint`, SQLite uses `strftime('%s','now')`, and MySQL uses `UNIX_TIMESTAMP()`.
- `model/usedata_rankings.go` has a MySQL-specific path guarded by `common.UsingMySQL`.

### Schema migrations with raw SQL

Migrations in `model/main.go` are intentionally DB-specific:

- `ensureSubscriptionPlanTableSQLite` uses SQLite-compatible `CREATE TABLE` and `ALTER TABLE ... ADD COLUMN`.
- `migrateTokenModelLimitsToText` skips SQLite, uses PostgreSQL `ALTER COLUMN ... TYPE text`, and MySQL `MODIFY COLUMN ... text`.
- `migrateSubscriptionPlanPriceAmount` skips SQLite and branches for PostgreSQL vs MySQL.

Do not use SQLite-unsupported `ALTER COLUMN`. For SQLite, use additive migrations or a custom table creation/column-add path.

---

## Migrations

Register new persistent models in `model/main.go` under `migrateDB` and, if relevant, `migrateDBFast`.

Rules:

1. Prefer GORM `AutoMigrate` for normal schema additions.
2. Add explicit compatibility migrations only when existing deployed schemas need type changes or special handling.
3. SQLite requires special care for schema changes; avoid `ALTER COLUMN`.
4. Keep log database migrations in `migrateLOGDB` when the table belongs to `LOG_DB`.
5. Ensure startup migrations are safe to run repeatedly.

When adding a table that has SQLite-specific creation constraints, follow the pattern in `ensureSubscriptionPlanTableSQLite`.

---

## Naming Conventions

- GORM structs use Go-style names (`Channel`, `Token`, `User`, `SubscriptionPlan`).
- JSON fields and column names usually use snake_case.
- GORM indexes are declared on struct tags when possible.
- Some legacy fields use explicit column names, for example `Channel.BaseURL` has `gorm:"column:base_url"`.
- Reserved columns such as `group` and `key` must be referenced via helper variables in raw SQL fragments.

---

## Tests

Database-sensitive tests often set DB flags explicitly and use in-memory SQLite:

- `controller/token_test.go` verifies token migration behavior across dialect assumptions.
- `controller/model_list_test.go`, `model/task_cas_test.go`, `service/task_billing_test.go`, and `service/waffo_pancake_test.go` initialize SQLite-backed test DBs.
- `model/perf_metric_test.go` toggles `common.UsingPostgreSQL` to verify SQL expression selection.

For new DB behavior, add tests that cover:

- SQLite in-memory behavior where practical;
- any DB-specific SQL branch;
- transaction rollback behavior for quota/accounting changes.

---

## Common Mistakes to Avoid

- Do not write MySQL-only raw SQL without PostgreSQL and SQLite handling.
- Do not use PostgreSQL-only JSONB operators or column types unless guarded with a fallback.
- Do not use `AUTO_INCREMENT` or `SERIAL` manually; let GORM handle primary keys.
- Do not interpolate untrusted user input into raw SQL. Use `?` placeholders and allowlisted column names.
- Do not forget `LOG_DB` when changing log persistence.
- Do not change table/column types without considering existing SQLite deployments.
