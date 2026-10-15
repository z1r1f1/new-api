# Merge upstream 6549a17ed and redeploy

## Goal
Merge the 13 upstream commits through 6549a17eda83eaa270fd8cbb0c03623836b351d6 into the local main line and redeploy only the existing production application container.

## Requirements
- Preserve all local relay retries, accounting, diagnostics, request timing, frontend features, translations, deployment configuration and unrelated untracked task directories.
- Resolve the 13 known conflicted files using a clean cutover to upstream error masking and console-search APIs; remove obsolete helpers rather than compatibility shims.
- Update local masking regressions for the new contextual masking contract without losing protection of secrets or Responses event names.
- Keep the inspected upstream target pinned; no unrelated provider fixes or new features.
- Create a verified PostgreSQL backup, source rollback ref, and rollback image tag before deployment.
- Preserve Compose environment, command, networks and mounts. Do not restart/recreate PostgreSQL or Redis.

## Acceptance Criteria
- [x] Both local pre-merge HEAD d72d5367f and upstream target are ancestors of the resulting merge commit.
- [x] All conflicts resolved and affected backend/frontend checks pass.
- [x] relaykit and tokenkit independently build with GOWORK=off.
- [x] Real SQLite/MySQL/PostgreSQL fresh/release-upgrade startup verification is recorded where affected by the serialized channel-settings contract.
- [x] Synthetic runtime requests exercise contextual error masking and Claude cached usage without production credentials or upstream calls.
- [x] New production image is healthy; direct and Nginx status/home/login/assets work and browser login surface is verified.
- [x] Production environment/mounts and PostgreSQL/Redis container identities/start times remain unchanged.
- [x] Deployment evidence identifies source commit, image IDs, backups and rollback command.

## Technical Approach
Use an integration branch and --no-commit merge; resolve backend/frontend slices independently. Build and exercise a uniquely tagged arm64 candidate from the merged workspace, commit the verified source, then recreate only the Compose application service. Retain rollback artifacts; do not push.

## Out of Scope
Provider behavior not changed by these commits, production account/configuration changes, PostgreSQL/Redis upgrades, and unrelated task directories.

## Verification Evidence
- Merge/source commit: bf4b1a3870d04e93baeea3350c041591a3dc0e0a.
- Production version: v1.0.0-rc.44-local.20261015; app container 65bdef2a0d9f is healthy.
- Backend affected packages passed; relaykit/tokenkit independent builds and tests passed.
- Frontend typecheck passed; lint passed with 47 warnings; 194 files / 2306 tests passed.
- SQLite 3.50.4 (Go driver), MySQL 8.4.11, PostgreSQL 16.13: fresh and rc.44-upgrade fixtures, two startups, schema/index/row preservation and token uniqueness verified. Separate MySQL/PostgreSQL log databases covered.
- Runtime requests verified contextual masking, exact cache-only charge of 25 quota, and Vertex project URL on all three databases.
- Isolated real-browser regular-user search hides admin pages and has one dialog; pricing input-price sort returns Gemini, Claude, GPT-4 in ascending order.
- Direct 3001 and Nginx 8999 status, home, login and JS/CSS assets passed; production login controls and screenshot verified without using production credentials.
- Environment/mounts unchanged; PostgreSQL/Redis container IDs and start times unchanged. Business row counts remain users=8, tokens=42, channels=16, options=106.
- Runtime evidence and verified 2,441,430,593-byte database backup: .runtime/prod/deploy-backups/20261015T125138Z/.
- Rollback image: new-api-prod-local:rollback-20261015T125138Z. No push.
