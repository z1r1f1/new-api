# Integrate upstream 1c63cc8c9

## Goal

Integrate the reviewed upstream target into local main, preserve native relay/custom administration contracts, fix the identified integration defects, verify in isolation, then commit, push and deploy the verified immutable candidate under explicit release authorization.

## Frozen inputs

- Local HEAD: 416f7da0d9ef0f4f8824f053906fbfcdacf40a09
- Upstream target: 1c63cc8c903630cf16611515d81b9785134daf19
- Upstream base: c046559bbd8b154e88e69905f60e71a5f4f1e5aa
- Reviewed synthetic tree: e05a63cc8832d33c767124cc26d401301eca589f
- Initial authorization covered integration and disk-aware isolated verification only. The user's subsequent explicit instruction, “提交推送并部署”, authorizes the two-parent merge commit, non-force origin/main push, private production backup and application-only deployment described below; unrelated data and resources remain protected.

## Requirements

- Resolve all merge conflicts by combining contracts, not choosing whole sides.
- Preserve native DeepSeek Claude/Responses, upstream IDs/continuation/tools, independent Search/Fetch, raw schema order/large integers/explicit zero and false, fast-to-priority/cache-key normalization, replay cursor and closer ownership, affinity/request-policy/task diagnostics, credential-file import, precise prices and all local locale changes.
- Preserve compress 1.19.0; integrate the complete upstream MoeForge runtime/import/PGO and frontend dependency set.
- Public request-plugin records must not persist body scalar values or credentials. Keep useful operation/path/type/size metadata; remove obsolete value consumers/contracts consistently.
- Normalize gateway-owned scalars after transformations and before final Egress inspection on filtered and passthrough HTTP/WS paths. Avoid full disk-body reads when normalization is unnecessary.
- Scoped request-plugin instances cannot be offered or accepted as channel bindings; keep existing invalid bindings removable and explain their ineffective status.
- Failed price saves must retain the confirmation/comparison view and draft, without duplicate error toasts.
- Verify new plugin-login expiry, replay, concurrency and actor-revocation behavior; complete credential persistence under the existing authoritative server-side authorization/transaction patterns.
- Preserve exact stored TypeSafe/Alibaba overrides, native Codex57 behavior, disabled legacy sensitive-word state and reversible configuration migration.

## Disk and production safety

- Initial root available: 66618261504 bytes (approximately 62 GiB); reserve at least 20 GiB throughout.
- Reuse installed dependencies and existing database images; use one worktree and one candidate binary, bounded compiler/test parallelism, and sequential database engine scenarios.
- No Docker/system prune, old cache/backup deletion, or unrelated container/image/volume removal. After private backup and rollback preparation, recreate only the production application; never restart PostgreSQL, Redis or other dependencies.
- Temporary database containers, volumes, binaries and fixtures must be uniquely named and cleaned only if created by this task.
- Preserve the three pre-existing untracked task directories.

## Acceptance criteria

- Conflict-free source and compatible exported contracts, with regressions covering the identified consumer-visible defects.
- Root verification and GOWORK=off build/test of affected independent modules; focused frontend tests, typecheck/lint/format and production build.
- Real SQLite/MySQL/PostgreSQL fresh and representative-upgrade migration verification, repeated startup, data preservation and rollback evidence; actual Redis integrity/quota tests.
- Isolated gateway HTTP/WS checks for native protocols, raw JSON/normalization, large disk bodies, retries/cancellation, plugin execution and billing. No paid or production upstream inference.
- Browser proof of the changed administration surfaces using isolated fixtures.
- Report exact commands/results, remaining scope and disk before/after. Preserve the frozen merge parents, push non-force to the existing origin/main, and bind the committed source to the already-verified image without rebuilding it or mislabelling the source-tree digest as a Git SHA.
- Before cutover, capture exact production configuration and stored overrides, validate a fresh PostgreSQL custom dump and retain an executable application/configuration-boundary rollback. Restore only the five retired sensitive options and original request-plugin instances on rollback; never restore the whole database automatically.
- After cutover, prove the exact image/version/health, unchanged environment/mounts/command/restart/network and dependency identities/lifecycles, preserved settings and stored plugins, disabled global sensitive filter with real keywords retained, direct/NGINX public pages and linked assets, and the public browser sign-in surface without logging in.

## Known activation boundaries

- Production has no request-plugin instances; identified public-value/ordering/scope defects are conditional new-feature defects, not observed production leaks.
- SensitiveWords is populated but CheckSensitiveEnabled=false; migration must create a disabled filter and rollback must restore retired options, not just the old image.
- Existing provider-table enabled rows are legacy storage, not proof of runtime-active new providers. New provider selection is type61; native Codex remains type57.

## Verification references

Reuse the existing isolated verification fixtures under .runtime/prod/deploy-backups/20261102T072639Z as appropriate, never their private production data. Authentication work follows OWASP Authentication, Session Management, MFA and OAuth2 guidance and ASVS 5.0.0; record the applicable controls and observed validation without claiming blanket compliance.
