# Merge upstream 9a42033a6 and redeploy production

## Goal
Merge the reviewed fifteen upstream commits through 9a42033a6 into the local application and deploy to the existing production Compose application, preserving local behavior, configuration, data and rollback capability. The user explicitly approved merge and deployment after receiving the read-only risk assessment.

## Requirements
- Integrate on merge/upstream-20261020-9a42033a6 from local 8c343708c; preserve unrelated user task directories and do not push.
- Resolve all twelve conflicts. Preserve DeepSeek Claude/Responses-to-Chat conversion, local performance bucket series, stream status semantics, request timing/replay/redirect behavior and default ID channel sorting. Retain all upstream features and all locale keys.
- Verify root backend and independent relaykit builds; frontend typecheck/lint/build; affected contracts and real SQLite/MySQL/PostgreSQL fresh and rc.45-upgrade databases with two candidate startups, separate log databases, data/index/constraint preservation and new cache/audit fields.
- Exercise changed streaming/header/task/pricing/priority/cache paths against an isolated actual application with synthetic credentials and upstream; inspect real frontend surfaces.
- Snapshot actual production environment/mounts/commands/dependency IDs, tag old image, verify a fresh PostgreSQL dump before application cutover.
- Deploy only the application using existing Compose project, env-file and configuration. Preserve PostgreSQL, Redis and all operator options, including SMTP, ping settings and administrator pricing overrides.

## Acceptance Criteria
- No unresolved conflicts; integrated source committed and retained on local main without pushing.
- Required builds, behavioral smoke and three-database migration matrix pass with exact versions and evidence recorded.
- Existing production container replaced by the verified candidate image; healthy status, correct version and actual frontend assets at direct and Nginx entrypoints.
- Production environment/mounts and dependency IDs/start times unchanged; schema additions present and business records preserved.
- Verified database backup and executable image-only rollback entrypoint retained in the ignored runtime backup directory.

## Out of Scope
No production stress/fault injection, notification/email sends, real upstream inference charges, dependency upgrades, unrelated repairs, production DB restores, PostgreSQL/Redis restart, remote push or changes to existing user task directories.

## Technical Notes
Reviewed baseline: upstream 09109ec31; target 9a42033a6; rc.45=6cf0d90b5. Delta 163 files +10004/-1208. Conflict groups: metrics.go, DeepSeek adaptor.go/tests, stream_scanner.go, channels-provider.tsx, seven locales. GitNexus identified StreamScannerHandler and shared doRequest as CRITICAL. Production before work: v1.0.0-rc.44-local.20261017, image revision fa8f7f89f, host port 3001, PostgreSQL 16.13/new-api, no separate production log DSN; ping true/60 seconds. See the preceding upstream review and applicable .trellis/spec/backend contracts.

## Verified Delivery
- Integrated commit `ce48d92a4b088754c7372498a9e230191610100e` is on local main; no push.
- Deployed `new-api-prod-local:upstream-9a42033a6-20261020T132843Z`, image `sha256:25e7c225a57ca941a92cd7aacb6f032525685c07acf85f7a6c6197c2840fb447`; container `bfc232898e8e` is healthy and reports `v1.0.0-rc.45-local.20261020`.
- Root build and full tests passed with actual external MySQL/PostgreSQL fixtures; independent relaykit build/all tests and tokenkit build passed. Frontend typecheck/lint/targeted format passed. Affected frontend suite exercised 44 files/568 cases: an asynchronous readiness assertion was corrected, then its full 34-case file passed; all other 567 cases had passed the broad run.
- Real SQLite 3.50.4, MySQL 8.4.11 and PostgreSQL 16.13 fresh/rc.45-upgrade matrix passed all six scenarios, two candidate startups each; separate MySQL/PostgreSQL log stores, rows/indexes/constraints/uniqueness, cache aggregates and optimistic priority updates verified.
- PostgreSQL audit-index test now inspects ordered physical catalog positions; driver GetIndexes returned table-column order although the actual index was correct. No production index reorder or dependency change was needed.
- Actual isolated gateway verified Claude preparation/retry heartbeat and SSE errors, both local DeepSeek Chat bridges and modifier precedence, Search/Fetch exact 375-quota charges and empty-result refunds, task submit/poll/content header precedence and one terminal charge. Existing hosted-tool filtering remains unchanged.
- Real HTTP summary fixture verified 100% normal-window versus 75% requested history and retained sorted bucket series; real gateway cache persistence recorded 50 read and 20 creation tokens after graceful shutdown.
- Browser verified fresh default ID sorting, priority selection/drag onboarding and root notification defaults. Production login rendered without application errors. Both direct 3001 and Nginx 8999 status/home/login and new JS/CSS assets passed.
- Environment hash, command, mounts, restart/network, all 107 existing persisted options and PostgreSQL/Redis IDs/start times remained unchanged. Production users=8, tokens=42, channels=10; quota_data rows grew naturally to 10925, with both new cache columns and the ordered audit index present.
- Fresh pre-cutover PostgreSQL dump: 2459311339 bytes, SHA-256 `418dc2c9c28a58cce324fb21807eba9b327824619cdc56f5c986523c3c1db89b`; archive TOC verified for core tables.
- Evidence and image-only rollback command are retained under `.runtime/prod/deploy-backups/20261020T132843Z/`; rollback image `new-api-prod-local:rollback-20261020T132843Z`, source ref `rollback/pre-upstream-20261020T132843Z`. Never restore production DB automatically.
- Owned verification containers/network/browser and temporary runners are removed after verification; user task directories remain untouched.
