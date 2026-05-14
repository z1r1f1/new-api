---
title: "Channel Priority Selection and Affinity Diagnostics"
tags: ["channel", "priority", "affinity", "relay", "debugging"]
created: 2026-05-13T05:56:01.570Z
updated: 2026-05-13T05:56:01.570Z
sources: []
links: []
category: debugging
confidence: medium
schemaVersion: 1
---

# Channel Priority Selection and Affinity Diagnostics

# Channel Priority Selection and Affinity Diagnostics

## Context
When investigating why a model call did not appear to use a higher-priority channel, distinguish three separate mechanisms: channel priority selection, channel affinity, and upstream prompt/cache behavior.

## Actual channel priority contract
Runtime channel distribution does not only look at `channels.priority`. The effective selection priority is scoped by the ability row:

```text
abilities.group + abilities.model + abilities.enabled + abilities.priority
```

For a normal request, `middleware.Distribute()` selects an initial channel. If no token-specific channel and no accepted affinity hit is used, it calls `service.CacheGetRandomSatisfiedChannel()`, which calls `model.GetRandomSatisfiedChannel(group, model, retry)`.

## Priority behavior
- `retry = 0` selects only from the highest available priority tier for that group/model.
- Channels inside the same priority tier are selected by weight. If all weights are zero, they are effectively equal.
- On retry, lower priority tiers can be selected. Therefore a final consume/error log may show a lower-priority channel if higher-priority attempts failed first. Check `admin_info.use_channel` to see the retry chain.
- Background model/channel tests may specify channels directly and should not be treated as production distributor priority evidence. These logs often have `content = 模型测试`, `token_id = 0`, and `admin_info.use_channel = null`.

## Evidence query pattern
Use log rows plus `abilities` to compare selected vs max priority:

```sql
WITH latest AS (
  SELECT id, created_at, model_name, "group", channel_id, ip, request_id, other
  FROM logs
  WHERE type IN (2,5)
), maxp AS (
  SELECT "group", model, max(priority) AS max_priority
  FROM abilities
  WHERE enabled = true
  GROUP BY "group", model
)
SELECT l.id, to_timestamp(l.created_at) AS ts, l.model_name, l."group",
       l.channel_id, a.priority AS selected_priority, maxp.max_priority,
       l.ip, l.request_id, l.other::jsonb #> '{admin_info,use_channel}' AS use_channel
FROM latest l
LEFT JOIN abilities a
  ON a.channel_id = l.channel_id
 AND a."group" = l."group"
 AND a.model = l.model_name
LEFT JOIN maxp
  ON maxp."group" = l."group"
 AND maxp.model = l.model_name
ORDER BY l.id DESC;
```

## Example finding from 2026-05-13
For IP `60.216.102.18`, recent production calls used `model=gpt-5.4`, `group=vip`, and selected channel `938`. In the database, `vip + gpt-5.4` highest priority was `1`, with channels `935`, `936`, `937`, and `938`; therefore channel `938` was a highest-priority channel, not a lower-priority miss.

## Related files
- `middleware/distributor.go` — initial distribution and affinity-before-random selection.
- `service/channel_select.go` — group/auto-group selection and retry priority progression.
- `model/channel_cache.go` — memory-cache priority tier and weighted selection.
- `model/ability.go` — DB fallback using max/distinct ability priority.
- `service/channel_affinity.go` — affinity cache diagnostics and request-prefix debug.

## Diagnostic rule of thumb
If a request appears to violate priority, first check whether it is a production relay request, whether `use_channel` contains retries, and whether selected channel priority is being compared against the matching `abilities` row rather than only the channel table.

