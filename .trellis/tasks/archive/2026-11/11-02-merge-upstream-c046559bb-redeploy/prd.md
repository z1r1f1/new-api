# Merge upstream c046559bb and redeploy

## Goal
Merge the reviewed upstream commit c046559bbd8b154e88e69905f60e71a5f4f1e5aa into local 367009f9ba0a69f223ea1c6877415255d092540e, preserve local behavior, and redeploy only the existing production application service after verification. The user explicitly authorized merge and redeployment after the read-only triage.

## Requirements
- Use the isolated integrate/upstream-20261102-c046559bb worktree. Preserve unrelated primary-worktree tasks and all production configuration/data.
- Resolve 11 conflicted files semantically. Keep local DeepSeek native Claude /anthropic/v1/messages and Responses /responses, raw schema order, prompt-cache normalization, fast-to-priority mapping, affinity key normalization and local submission diagnostics. Integrate upstream native plugin routing/SSE/settlement and priority fallback correction completely.
- Upgrade host Bun from 1.4.2 to pinned 1.4.4; retain Go 1.26.10 and locally newer dependencies.
- Keep existing production environment unchanged, adopting the upstream 72-hour default task timeout because TASK_TIMEOUT_MINUTES is unset; no nonterminal tasks existed at baseline.
- Preserve stored TypeSafe 1.0.0 and Alibaba 1.4.1 plugin overrides and existing pricing/options.
- No push, production paid requests, database restoration, dependency-container restart, engine upgrade, or unrelated cleanup.

## Acceptance Criteria
- [x] Root build and complete root tests pass; independent relaykit and tokenkit builds and relevant relaykit tests pass.
- [x] Frontend frozen install, bun check -b, lint, scoped formatting, affected contracts and production build pass; real browser surface verified.
- [x] Real SQLite/MySQL/PostgreSQL fresh and rc.47 upgrade startup matrix, twice-start idempotence, rows/index/uniqueness and separate log DB verification pass.
- [x] Isolated actual gateway smoke proves native DeepSeek JSON/SSE, raw schema/fast/cache normalization, stream DONE/cancellation/retry and plugin accounting; real three-engine charge/refund verification passes.
- [x] Exact final source revision binds candidate image and all image-bound gates.
- [x] Validated PostgreSQL dump and executable application-only rollback exist before cutover.
- [x] Production exact image/version/health, direct+proxy status/assets, environment/mounts/command and existing options are preserved; PostgreSQL/Redis identities and start times unchanged.

## Technical Notes
Deployment evidence and private baseline: `/home/ubuntu/git-project/new-api/.runtime/prod/deploy-backups/20261102T072639Z`. Simulated merge tree `9e744967f4beaba10d7306ba8311d30ccf77c327`. Manual impact analysis used the graph matching pre-merge source `367009f9b`; the integration refresh timed out after 900s. Actual staged integration and correction diffs were detected against that base graph, with new-symbol/moved-line coverage explicitly limited. No language server is configured. Manual `RemoveDisabledFields` depth3 impact was CRITICAL (5 direct, 15 affected, 2 flows); the raw Chat/Responses correction was HIGH and includes HTTP, WebSocket and Playground callers.

## Completed Deployment

- Merge commit: `ae5d2476043073b8a1e4a1f15351590e09c6cfb1`; final source correction/deployed revision: `9b6fcd223b79a1aa8f954a2c7ebb1e78f1b3c4d9`.
- Running version: `v1.0.0-rc.47-local.20261102`; arm64 image: `sha256:28475f5627b3462e5a2d1363e977b14ca217a817c66eca4a689687498b21760e`; application container `131357df99c566483f2fef34fef33b57901afb840dddc10aadedf98bd6c97782` is healthy.
- Passed full root build/tests, standalone module gates, frontend typecheck/lint/scoped formatting/build and 227 affected frontend tests. The frontend subtree is identical between the verified merge and final deployed revision.
- Final-image startup matrix: six fresh/rc.47-upgrade scenarios, two starts each, with real SQLite 3.50.4, MySQL 8.4.11 and PostgreSQL 16.13; rows, schema/index/uniqueness idempotence and separate log databases verified. Real exported billing lifecycle: 17 cases per engine, 51 total.
- Final-image HTTP smoke: 43 checks, 44 controlled upstream requests, eight reconciled wallet/token ledgers. Covers native DeepSeek protocols/IDs/continuation, Search/Fetch charges/refunds, raw/filtered/channel/global normalization, cache creation accounting, stream terminal/retry/cancel, native plugin SSE/polling/failure/settlement/auth and prefix isolation.
- Actual candidate desktop/mobile plugin/pricing/sidebar and synthetic sign-in/sign-out surfaces passed. Production public sign-in rendered read-only with the observed public logo host; no production login or paid upstream request was performed by the assistant.
- Application-only Compose cutover preserved all 107 existing option hashes, two stored task-plugin rows, five active provider-plugin rows, seven provider-plugin versions, complete environment/mounts/command/restart/network and PostgreSQL/Redis IDs/start times. Existing TypeSafe 1.0.0 and Alibaba 1.4.1 overrides were not overwritten.
- Fresh cutover backup: `database-before-cutover.dump`, 2,289,778,413 bytes, SHA256 `a7caeafe60b6f891b1d77289c4e8bd3c8eba9c07a885fa4a8c04458771559291`; database/TOC validated. `rollback.sh` retains the previous image and recreates only the application, never the database.
- Evidence: `verification.json`, `deployment.json`, `production-acceptance.json`, real-database reports, HTTP reports and screenshots in the evidence directory. No remote push or rollback execution was performed.

## Regression Analysis and Prevention

- Category C/D: upstream raw-body replay optimization bypassed the local scalar normalizer even though `RemoveDisabledFields` unit tests passed. Actual candidate HTTP smoke caught unchanged `fast` and oversized cache keys before production cutover.
- The correction routes raw Chat and Responses through `NewPassthroughJSONBody`, retaining raw bytes and independent replay while borrowing unchanged incoming storage. The obsolete scalar-only reader was removed. The existing regression file now protects memory/disk normalization, schema order/precise integers, replay cursors, source immutability and closer ownership.
- The executable contract in `.trellis/spec/backend/error-handling.md` requires testing actual filtered, channel-passthrough and global-passthrough wire paths; direct storage forwarding is not evidence that scalar normalization still runs.
- Known nonfunctional check limitations: the complete GitNexus refresh timed out; the global copyright checker reports 24 existing noncanonical/missing headers. Scoped formatting passed and all 35 checked existing protected headers stayed byte-identical. No attribution was rewritten to satisfy the cosmetic checker.

