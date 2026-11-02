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
- [ ] Root build and complete root tests pass; independent relaykit and tokenkit builds and relevant relaykit tests pass.
- [ ] Frontend frozen install, bun check -b, lint, scoped formatting, affected contracts and production build pass; real browser surface verified.
- [ ] Real SQLite/MySQL/PostgreSQL fresh and rc.47 upgrade startup matrix, twice-start idempotence, rows/index/uniqueness and separate log DB verification pass.
- [ ] Isolated actual gateway smoke proves native DeepSeek JSON/SSE, raw schema/fast/cache normalization, stream DONE/cancellation/retry and plugin accounting; real three-engine charge/refund verification passes.
- [ ] Exact final source revision binds candidate image and all image-bound gates.
- [ ] Validated PostgreSQL dump and executable application-only rollback exist before cutover.
- [ ] Production exact image/version/health, direct+proxy status/assets, environment/mounts/command and existing options are preserved; PostgreSQL/Redis identities and start times unchanged.

## Technical Notes
Deployment evidence and private baseline: /home/ubuntu/git-project/new-api/.runtime/prod/deploy-backups/20261102T072639Z. Simulated merge tree 9e744967f4beaba10d7306ba8311d30ccf77c327. Read-only findings in session local://upstream-triage-result.json. GitNexus index is current for primary HEAD; context/impact work, query FTS is unavailable. No language server is configured. Manual RemoveDisabledFields impact depth3 is CRITICAL (5 direct,15 affected,2 flows); fast/cache helpers HIGH. Upstream moego migration requires replacing seven remaining gjson test references in the local DeepSeek tests.
