# Merge upstream 09109ec31 and redeploy production

## Goal

Merge the four reviewed upstream commits after 6549a17ed through 09109ec31 into the locally customized gateway and update only the existing production application container.

## Requirements

- Pin the reviewed upstream commit 09109ec315a5086366f94bcce00f177ec831dbe1; merge on a separate integration branch.
- Preserve local gateway diagnostics, logging, IP restrictions, channel UI customizations, and unrelated untracked Trellis tasks.
- Preserve production environment, mounts, command, network, database, Redis, and persisted operational thresholds.
- Back up the production PostgreSQL database, validate the dump, retain the old image and source rollback reference before application cutover.
- Verify backend, standalone relaykit, frontend and affected regressions; exercise real candidate HTTP paths for Responses reasoning summary, group rate isolation, optional AWS Mantle and sustained overload behavior in isolation.
- Exercise channel settings persistence on real SQLite, MySQL and PostgreSQL instances if its JSON contract is affected; do not use production data or credentials for smoke fixtures.
- Recreate only the application container, check health and actual production HTTP/browser surfaces, then integrate the verified source into main without pushing.

## Acceptance Criteria

- [x] Four upstream commits are merged without losing local changes.
- [x] Builds and affected contract tests pass; verification evidence is recorded.
- [x] Isolated candidate requests prove new protocol and middleware behavior without external paid requests or production mutation.
- [x] A readable production database backup and tagged old image/source rollback are available.
- [x] New application is healthy and serves the new version; production environment/mounts and PostgreSQL/Redis identity/start times are preserved.
- [x] Browser/login assets and direct/proxied status surfaces load successfully.

## Out of Scope

- No additional upstream commits, dependency upgrades, production tuning, account/security changes, PostgreSQL/Redis restart, volume deletion or remote push.
- No production overload injection or real provider credentials in smoke fixtures.

## Decision

The user explicitly authorized merging and deployment after a read-only assessment. Reuse the established isolated integration, rollback and application-only cutover workflow. No unresolved user decision.

## Technical Notes

Base HEAD 24787e84e; last upstream base 6549a17ed. Reviewed delta: 24 files, +1483/-68, zero merge-tree textual conflicts. Production CPU threshold 200 remains unchanged. Runtime evidence is stored under `.runtime/prod/deploy-backups/<deployment timestamp>/`.

## Verified Delivery

- Merge/source commit `fa8f7f89f`; deployment contract commit `475aab9fb`; integrated into local main, not pushed.
- Running image `sha256:1069001bb7d1c77ecf54057694af021fb4a6b68ab4a7748e66c91799724b91ca`; application container `e6e2b119dbe1`, healthy, version `v1.0.0-rc.44-local.20261017`.
- Root build and broad backend tests, standalone relaykit build/full tests at Go 1.25.1, tokenkit build, frontend typecheck/build and affected 96 tests passed. Lint: zero errors, 47 warnings. Initial concurrent-build frontend timeout is retained separately; unchanged quiet rerun passed.
- SQLite 3.50.4, MySQL 9.7.2 and PostgreSQL 18.6 passed fresh/release-rc.44-upgrade scenarios, three candidate startups each, settings persistence, rows/indexes/constraints, uniqueness and separate log databases.
- Actual candidate HTTP/TLS and browser proofs cover reasoning summary, independent/shared group counters, all eight Mantle route/auth/stream combinations, cache billing, legacy InvokeModel, finite cgroups and overload recovery.
- Initial immediate Redis success-count burst exposed an unchanged post-response accounting window. Source-tail comparison and a separately recorded exact-counter-barrier probe establish that the new subject isolation works; no unrelated concurrency fix was introduced.
- Production environment/mounts/command/network and PostgreSQL/Redis IDs/start times unchanged. Users/tokens/channels/options remain 8/42/9/107; persisted CPU threshold remains 200.
- Direct/proxied status, home/login, all six JS/CSS assets and actual production login browser passed, with zero browser errors and no production login.
- Verified PostgreSQL 16.13 custom dump: 2,447,439,938 bytes, SHA-256 `d4e7d488587b4bcca4a84f31500ea05d99ddba78836be337521a1b06b510a9c9`.
- Evidence and rollback entrypoint: `.runtime/prod/deploy-backups/20261017T052050Z/`; production database is never automatically restored during image rollback.
- All owned verification containers/network/browser and throwaway scripts were removed; unrelated user tasks remain untouched.
