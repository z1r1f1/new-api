# Merge upstream and deploy current service

## Goal
Fetch current upstream/main, merge it while preserving local behavior, verify the exact candidate, and apply it to the existing Docker Compose application. The user explicitly authorized merge and deployment.

## Requirements
- Preserve all existing uncommitted files, production configuration, data, mounts, PostgreSQL and Redis containers.
- Merge upstream/main on a new integration branch. Retain the newer local compress dependency.
- Preserve local DeepSeek Claude/Responses-to-Chat bridges, request timing, Responses output normalization, accepted-error handling, and existing pricing precision.
- Integrate checked Responses delivery, slow-client streaming, task endpoint selection/header/multipart support, Claude cache billing and exchange-rate pricing changes.
- Verify builds, affected contracts, independent relaykit/tokenkit, real database matrix, isolated gateway scenarios and actual frontend surface.
- Back up production PostgreSQL, retain old image and source ref, deploy only the application and verify configuration fingerprints, dependency identity and public HTTP/assets.

## Acceptance Criteria
- [x] upstream/main is an ancestor of the deployed source commit; no conflict markers remain.
- [x] Focused verification and isolated runtime proof pass on the final image.
- [x] Real SQLite/MySQL/PostgreSQL fresh and release-upgrade startup checks pass with repeated startup and separate log databases.
- [x] Production runs the candidate image healthy; existing options/environment/mounts are retained; PG/Redis identity and StartedAt remain unchanged.
- [x] Verified dump, evidence and executable image-only rollback entry remain available.

## Out of Scope
No remote push, production provider requests, unrelated plugin/config changes, host engine upgrade or cleanup of existing backups/resources.

## Decision
Use the established isolated integration and application-only rollout. Text merges are not sufficient for shared relay/task semantics; validate those on synthetic upstreams. Do not downgrade compress or rewrite unrelated work.

## Completion Evidence
- Merge `b2756cb0e`; final source `3cf656554`; upstream `d8e65613c` (13 commits).
- Deployed `v1.0.0-rc.46-local.20261022`, image `e5927be9d3b0`, healthy app `ee8dddf9c2dc`.
- Root build/full tests, independent relaykit build/tests and tokenkit build; frontend typecheck/build/lint and 23 pricing tests pass. Protected-header formatting passes all 30 changed frontend source/JSON files; 37 pre-existing unchanged formatting violations remain outside this update.
- SQLite 3.50.4 / MySQL 8.4.11 / PostgreSQL 16.13: fresh + rc.46-upgrade, two candidate startups each, separate MySQL/PostgreSQL logs, business rows/indexes/constraints retained. Four pricing/routing regression tests also run on actual three-engine fixtures.
- 32 isolated HTTP/runtime checks pass: 11 preserved gateway contracts, 6 new task/cache contracts, 15 stream contracts. A 16 MiB event survives 36 seconds of real receive-side backpressure, completes and bills once.
- Candidate desktop/mobile browser proof covers exact model-name copying and actual exchange-rate preview/save. Production login, direct/Nginx status/home/assets pass; all 107 options, full environment/mounts and PostgreSQL/Redis identity/start time retained.
- Evidence and two verified PostgreSQL dumps: `.runtime/prod/deploy-backups/20261022T114823Z/`; application-only rollback is `rollback.sh`, with no automatic DB restore.
- Existing gap observed outside the changed scanner path: closing an ordinary Claude client before upstream headers did not promptly close that pending upstream connection. Ordinary `http.NewRequest` construction is unchanged in this merge; this update does not claim to fix pre-header cancellation. Scanner-stage client cancellation passed.
- No remote push, production paid provider request or unrelated WIP changes.
