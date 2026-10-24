# Merge upstream 33c61a242 and deploy native DeepSeek protocols

## Goal
Merge the five reviewed upstream commits through 33c61a24234aa20cb10e8a644304322c476361de, explicitly remove the local DeepSeek Claude/Responses-to-Chat bridge as chosen by the user, verify the candidate, and redeploy only the production application.

## Requirements
- Restore upstream DeepSeek native Claude `/anthropic/v1/messages` and Responses `/responses` request/response behavior; retain independent Search/Fetch and upstream thinking/modifier semantics. Remove obsolete bridge helpers, session storage, tests and comments/callers.
- Preserve unrelated local behavior and all original worktree WIP. In particular retain rounded consume-log timing, request diagnostics, pricing precision, prompt-cache-key normalization, and fast-to-priority semantics.
- Resolve the upstream raw-JSON DTO/filter cutover without rebuilding nested schema maps. Preserve schema property order, including passthrough with fast service tier.
- Integrate 12-hour cache billing and align frontend fallback estimates with backend normalization. Preserve explicitly saved administrator prices; do not silently rewrite production settings.
- Keep project branding/attribution unchanged. No unrelated authentication, engine maintenance, dependency downgrade, remote push, or paid production inference.
- Back up the production database, retain old image/source, and provide an executable image-only rollback script. Never restore the database automatically.

## Acceptance Criteria
- Root build and tests pass; relaykit independently builds/tests with GOWORK=off. Frontend affected checks and production build pass.
- Actual isolated gateway proves native DeepSeek Claude/Responses nonstream+stream routing (not Chat), thinking behavior, schema order, and 12h cache settlement.
- Actual SQLite/MySQL/PostgreSQL verify changed cache charges and startup compatibility, fresh and released-image upgrade, repeated candidate startup and separate log DB where applicable.
- Browser verifies candidate changed pricing and drawer surfaces, plus read-only production login after deployment.
- Production exact image/revision/version healthy; original environment, mounts, command, options and PG/Redis container identity/start time preserved.

## Decision
User explicitly chose not to preserve the Claude/Responses-to-Chat bridge after being told native endpoints are required and existing local response IDs cannot be carried over. No type43 channel was present in the production baseline. Native protocol selection is the requested cutover, not a conflict-resolution shortcut.

## Out of Scope
Engine upgrades, cache cleanup, unrelated pre-existing format failures, production model requests, automatic price edits, git remote push.

## Technical Notes
- Original HEAD b24dcd716; deployed source 3cf656554; base d8e65613c; upstream target 33c61a242.
- Text conflicts: relay/channel/deepseek/adaptor.go and service/text_quota.go. Additional raw-JSON compatibility in relay/common/relay_info.go.
- Original WIP fingerprinted outside this isolated worktree.
- Evidence/rollback: /home/ubuntu/git-project/new-api/.runtime/prod/deploy-backups/20261024T022638Z/.
