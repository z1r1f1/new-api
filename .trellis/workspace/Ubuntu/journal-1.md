# Journal - Ubuntu (Part 1)

> AI development session journal
> Started: 2026-05-11

---



## Session 1: Merge upstream rc.44 and deploy current application

**Date**: 2026-10-14
**Task**: Merge upstream rc.44 and deploy current application
**Branch**: `main`

### Summary

Merged upstream 7de10f2c6; preserved local relay, billing, auth and deployment behavior. Backend suite, 2273 frontend tests, standalone modules and six real SQLite/MySQL/PostgreSQL migration and quota API scenarios passed. Recreated only new-api-prod-app with healthy rc.44-local image; environment/mounts and PostgreSQL/Redis containers retained; verified direct/Nginx assets and actual login UI. Rollback image and verified PostgreSQL backup recorded in .runtime/prod/deploy-backups/20261014T073240Z/deployment.json.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `f2b321509` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 2: Merge upstream 6549a17ed and redeploy

**Date**: 2026-10-15
**Task**: Merge upstream 6549a17ed and redeploy
**Branch**: `main`

### Summary

Merged 13 upstream commits with local behavior preserved; deployed v1.0.0-rc.44-local.20261015. Backend/module checks and 2306 frontend tests passed; SQLite/MySQL/PostgreSQL fresh/upgrade matrix and isolated relay/UI smoke passed. App healthy; production environment, mounts, PostgreSQL and Redis preserved. Backup/evidence: .runtime/prod/deploy-backups/20261015T125138Z/.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `bf4b1a387` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 3: Merge upstream 09109ec31 and redeploy production

**Date**: 2026-10-17
**Task**: Merge upstream 09109ec31 and redeploy production
**Branch**: `main`

### Summary

Merged four reviewed upstream commits; production v1.0.0-rc.44-local.20261017 healthy. Root and standalone module builds/tests, frontend96 affected tests/typecheck/build, real SQLite3.50.4/MySQL9.7.2/PostgreSQL18.6 fresh+rc44-upgrade matrices and isolated protocol/UI smoke verified. Preserved PostgreSQL16.13, Redis, environment/mounts and operational thresholds; validated2,447,439,938-byte backup plus image/source rollback. Evidence .runtime/prod/deploy-backups/20261017T052050Z/. Initial frontend contention timeout and unchanged Redis post-response accounting window retained in evidence; no unrelated fixes. No push.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `fa8f7f89f` | (see git log) |
| `475aab9fb` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 4: Merge upstream 9a42033a6 and deploy rc.45

**Date**: 2026-10-21
**Task**: Merge upstream 9a42033a6 and deploy rc.45
**Branch**: `main`

### Summary

Merged 15 upstream commits with local relay, metrics and sorting behavior preserved. Root and independent module builds/tests, affected frontend regressions, six real SQLite/MySQL/PostgreSQL fresh/rc.45-upgrade scenarios and actual isolated relay/UI smoke passed. Application-only production cutover healthy; 107 options, environment/mounts and PostgreSQL/Redis identities unchanged. Verified backup, rollback and evidence: .runtime/prod/deploy-backups/20261020T132843Z/. No push; unrelated task directories untouched.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ce48d92a4b088754c7372498a9e230191610100e` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 5: Repair Docker crash and complete upstream deployment

**Date**: 2026-10-21
**Task**: Repair Docker crash and complete upstream deployment
**Branch**: `integration/upstream-206e0fa5c-20261021`

### Summary

Fixed the Docker build-history crash and completed the previously interrupted exact-source production deployment; preserved configuration and data-service uptime.

### Main Changes

- Replaced vulnerable distro Docker 29.1.3 with official signed Docker CE 29.9.2, containerd 2.4.2 and BuildKit 0.34.0 after explicit maintenance approval. Enabled live restore with a reload before migration; reinitialized docker.socket after package replacement. The formerly crashing build-history query returned zero with 50 records; production container IDs/start times were preserved.
- Rebuilt and deployed source a411c2d592d751f382df060778d327a1e3ea844f as arm64 image sha256:abb76ccadfe138dbaf168e291c08b0c8c63c16524ae81f46a1ec06dfb1d39138, version v1.0.0-rc.45-local.20261021. Only the app was recreated; new container 4c1bd90cd749 is healthy. Local main now references the verified source; no push or new commit was performed.
- Exact-image checks: six fresh/rc.45-upgrade SQLite/MySQL/PostgreSQL scenarios, each with two candidate startups and preserved rows/schema/indexes; eleven isolated HTTP scenarios / 33 controlled upstream requests; candidate and production browser sign-in surfaces. Root builds/tests, independent-module checks and 18 billing cases from the same final source were reused with explicit hash provenance, not rerun or claimed as new results. Engine versions: SQLite 3.50.4, MySQL 8.4.11, PostgreSQL 16.13.
- All 107 persisted option fingerprints, application environment/mounts and PostgreSQL/Redis identities/start times remained unchanged. Public status/home/sign-in/assets passed on ports 3001 and 8999. Browser harness initially blocked the configured public logo; allowing exactly pazom.xyz resolved it without application changes. No real login or paid inference was performed.
- Evidence and executable app-only rollback: .runtime/prod/deploy-backups/20261021T151118Z/. Fresh pre-cutover dump: 2460113383 bytes, SHA-256 c7be4c72f75481671ce58c48427dcaac80f143ca5591f2af60a1c1a0d1a5fe3f; pg_restore listing validated. Old/new engine packages and a package rollback command are retained; engine downgrade was not tested and older containerd metadata compatibility is not guaranteed.
- Owned fixture containers/networks, browser service and temporary runner copies were removed. Existing unrelated task directories and stopped services were untouched. Deployment recovery contracts were added to the existing backend quality spec. Disk has about 15 GiB free; previous backups were retained.


### Git Commits

| Hash | Message |
|------|---------|
| `51f8717757c6200298e3abef45171857b090a460` | (see git log) |
| `a411c2d592d751f382df060778d327a1e3ea844f` | (see git log) |

### Testing

- [OK] Docker 29.9.2 / BuildKit 0.34.0 history regression; running production containers preserved across engine migration.
- [OK] Exact-source arm64 image build, six real-database startup scenarios and eleven isolated HTTP smoke scenarios.
- [OK] Public candidate/production browser surfaces and direct/proxy HTTP/assets; all 107 option fingerprints and PostgreSQL/Redis identities preserved.
- [OK] Same-source historical build/test/billing evidence verified by provenance; no claim that those checks were rerun.

### Status

[OK] **Completed**

### Next Steps

- Deployment complete. Specification/journal/task bookkeeping remains uncommitted; no remote push. Retain rollback backups and monitor the remaining approximately 15 GiB of disk space.


## Session 6: Merge upstream d8e65613c and deploy rc.46

**Date**: 2026-10-22
**Task**: Merge upstream d8e65613c and deploy rc.46
**Branch**: `integration/upstream-d8e65613c-20261022`

### Summary

Merged 13 upstream commits; deployed image e5927be9d3b0 as rc.46-local.20261022. Root and independent module gates, 23 pricing tests, six real SQLite/MySQL/PostgreSQL startup scenarios, 32 isolated runtime checks and actual desktop/mobile UI passed. Preserved 107 options, env/mounts and PG/Redis IDs/start times. Verified dumps and image-only rollback: .runtime/prod/deploy-backups/20261022T114823Z. Existing pre-header cancellation gap and 37 unrelated formatting issues recorded; original WIP preserved and no push.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `b2756cb0e` | (see git log) |
| `3cf656554` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 7: Merge upstream 33c61a242 and deploy native DeepSeek protocols

**Date**: 2026-10-24
**Task**: Merge upstream 33c61a242 and deploy native DeepSeek protocols
**Branch**: `integrate/upstream-20261024-native-deepseek`

### Summary

Deployed v1.0.0-rc.46-local.20261024, image 065208c74a9d, healthy app 89c1aa394500. Native DeepSeek Claude/Responses replace the local Chat bridge by explicit operator decision. Root build/full tests, independent relaykit/tokenkit gates, 346 frontend tests, 24 final-image HTTP checks, real SQLite 3.50.4/MySQL 8.4.11/PostgreSQL 16.13 fresh+rc46 upgrades with repeated startup, 84 exact cache-charge cases and 12 no-credit denials, desktop/mobile pricing, expiry persistence, 12h log details and read-only production login passed. All 107 option fingerprints, environment, mounts and PostgreSQL/Redis identity/start times preserved. Original main worktree WIP untouched; no paid inference and no rollback drill. Evidence and executable image-only rollback: /home/ubuntu/git-project/new-api/.runtime/prod/deploy-backups/20261024T022638Z/.

### Main Changes

(Add details)

### Git Commits

| Hash | Message |
|------|---------|
| `ea4b3d6759055365f13285b0ee82ad36ce70296a` | (see git log) |

### Testing

- [OK] (Add test results)

### Status

[OK] **Completed**

### Next Steps

- None - task complete


## Session 8: Merge upstream c046559bb and redeploy rc.47

**Date**: 2026-11-02
**Task**: Merge upstream c046559bb and redeploy rc.47
**Branch**: `integrate/upstream-20261102-c046559bb`

### Summary

Merged the reviewed50 upstream commits, corrected replayable raw normalization, and deployed the exact validated arm64 image with application-only cutover; all preservation and synthetic DB/HTTP/browser gates passed. No push.

### Main Changes

- Merged the frozen c046559bb upstream target with native DeepSeek, raw schema, cache/tier normalization and local request-policy diagnostics preserved. Fixed the real passthrough regression caught by candidate HTTP smoke; the normalized replay helper preserves incoming body ownership and memory/disk replay.
- Deployed v1.0.0-rc.47-local.20261102 from 9b6fcd223b79a1aa8f954a2c7ebb1e78f1b3c4d9, image sha256:28475f5627b3462e5a2d1363e977b14ca217a817c66eca4a689687498b21760e. Only the application was recreated; PostgreSQL/Redis IDs/start times, complete environment/mounts/command/restart/network, all107 options and all stored plugin rows remained unchanged.
- Root build/full tests; standalone relaykit/tokenkit; frontend typecheck/lint/scoped format/build and227 affected frontend tests passed. Exact final image passed six real fresh+rc47 upgrade/twice-start DB scenarios (SQLite3.50.4/MySQL8.4.11/PostgreSQL16.13),51 real billing cases and43 controlled HTTP checks with44 upstream requests/eight reconciled ledgers. Candidate desktop/mobile/plugin/pricing/sidebar and synthetic login/logout, plus read-only production sign-in, were observed.
- Evidence and image-only rollback: /home/ubuntu/git-project/new-api/.runtime/prod/deploy-backups/20261102T072639Z. The fresh cutover PostgreSQL dump is2,289,778,413 bytes, SHA256 a7caeafe60b6f891b1d77289c4e8bd3c8eba9c07a885fa4a8c04458771559291. No database restoration, paid inference, notification, engine/dependency restart or remote push was performed.
- Limits: integration GitNexus refresh timed out; actual staged changes were mapped through the baseline graph, not claimed as full new-symbol coverage. Global copyright diagnostics were pre-existing;35 protected headers remained byte-identical and scoped formatting passed. Unrelated primary task WIP is preserved; deployment source remains9b6fcd223 independently of metadata-only bookkeeping commits.


### Git Commits

| Hash | Message |
|------|---------|
| `ae5d2476043073b8a1e4a1f15351590e09c6cfb1` | (see git log) |
| `9b6fcd223b79a1aa8f954a2c7ebb1e78f1b3c4d9` | (see git log) |

### Testing

- [OK] Go 1.26.10 root build/full suite; standalone `GOWORK=off` relaykit build/tests and tokenkit build.
- [OK] Bun 1.4.4 frontend typecheck/lint/scoped formatting/production build and 227 affected contract tests; protected attribution unchanged.
- [OK] Final-image real three-DB startup/upgrade/idempotence matrix, 51 billing cases and 43 HTTP checks; candidate desktop/mobile/login/logout and production public sign-in screenshots retained.
- [OK] Application-only cutover, exact image/version/health, direct/proxy assets, 107 options, stored-plugin fingerprints and unchanged PostgreSQL/Redis identities. Full commands and results: `.runtime/prod/deploy-backups/20261102T072639Z/verification.json` and `deployment.json`.

### Status

[OK] **Completed**

### Next Steps

- None - task complete
