# Merge upstream and deploy the current application

## Goal
Merge upstream/main at 7de10f2c602b50122017556e8a09cb7c488c273c into local main, preserve local gateway customizations, and deploy the merged workspace to the existing new-api-prod-app container.

## Requirements
- Preserve unrelated untracked Trellis tasks.
- Preserve local relay retry, billing, diagnostics, authentication, payment and UI contracts when resolving conflicts.
- Preserve production Compose environment, host network, port 3001, bind mounts and dependent PostgreSQL/Redis containers.
- Save a source rollback ref, old-image rollback tag and verified PostgreSQL backup before deployment.
- Run frontend verification, backend checks, standalone relaykit/tokenkit builds and real SQLite/MySQL/PostgreSQL migration checks including repeat startup and representative upgrade.
- Commit the completed upstream merge without pushing.
- Recreate only the application service after verification, then exercise direct and Nginx entrypoints and inspect the actual login UI.

## Acceptance Criteria
- No unresolved merge conflicts; the merge retains both histories.
- Required builds/checks and database matrix succeed.
- The production application runs the new image and is healthy.
- Production environment/mounts are unchanged; PostgreSQL/Redis IDs and startup timestamps are unchanged.
- Rollback image, source ref and database backup are recorded with deployment evidence.

## Out of Scope
- No upstream push, unrelated feature work, production account changes, upstream inference calls, database restore, volume deletion or dependent service upgrades.

## Technical Notes
- Pre-merge HEAD: f08f8910cbaa719330319aefcc643825a7c1fb33.
- Common ancestor: 7aa3531ef4c247ad4891c06cc8ed9d0ffb73fecc.
- Dry merge: 23 conflicting files.
- Relay impact: CRITICAL, 8 direct callers, 2 affected flows.
- LSP unavailable in this project; GitNexus CLI available.
