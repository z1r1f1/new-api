# Integrate upstream 1c63cc8c9

## Goal

Integrate the reviewed upstream target into local main, preserve native relay/custom administration contracts, fix the identified integration defects, and verify in isolation without changing production.

## Frozen inputs

- Local HEAD: 416f7da0d9ef0f4f8824f053906fbfcdacf40a09
- Upstream target: 1c63cc8c903630cf16611515d81b9785134daf19
- Upstream base: c046559bbd8b154e88e69905f60e71a5f4f1e5aa
- Reviewed synthetic tree: e05a63cc8832d33c767124cc26d401301eca589f
- User authorizes integration and requires disk awareness. Deployment, push, production writes and deletion of existing data/resources are out of scope.

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
- No Docker/system prune, old cache/backup deletion, running-container/image/volume removal, or production restarts.
- Temporary database containers, volumes, binaries and fixtures must be uniquely named and cleaned only if created by this task.
- Preserve the three pre-existing untracked task directories.

## Acceptance criteria

- Conflict-free source and compatible exported contracts, with regressions covering the identified consumer-visible defects.
- Root verification and GOWORK=off build/test of affected independent modules; focused frontend tests, typecheck/lint/format and production build.
- Real SQLite/MySQL/PostgreSQL fresh and representative-upgrade migration verification, repeated startup, data preservation and rollback evidence; actual Redis integrity/quota tests.
- Isolated gateway HTTP/WS checks for native protocols, raw JSON/normalization, large disk bodies, retries/cancellation, plugin execution and billing. No paid or production upstream inference.
- Browser proof of the changed administration surfaces using isolated fixtures.
- Report exact commands/results, remaining scope, disk before/after and unchanged production baseline. Do not commit or push without the required commit confirmation.

## Known activation boundaries

- Production has no request-plugin instances; identified public-value/ordering/scope defects are conditional new-feature defects, not observed production leaks.
- SensitiveWords is populated but CheckSensitiveEnabled=false; migration must create a disabled filter and rollback must restore retired options, not just the old image.
- Existing provider-table enabled rows are legacy storage, not proof of runtime-active new providers. New provider selection is type61; native Codex remains type57.

## Verification references

Reuse the existing isolated verification fixtures under .runtime/prod/deploy-backups/20261102T072639Z as appropriate, never their private production data. Authentication work follows OWASP Authentication, Session Management, MFA and OAuth2 guidance and ASVS 5.0.0; record the applicable controls and observed validation without claiming blanket compliance.
