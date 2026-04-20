---
room: core
architectural_health: normal
security_tier: sensitive
---

# Core

Subdomain: core/
Source paths: internal/index/, internal/claims/, internal/notify/, internal/datadog/

## TASK → LOAD

| Task | Load |
|------|------|
| Parse or update room frontmatter | index.md |
| Find which rooms cover a source file | index.md |
| Parse or diff TASK/PATTERN/GOVERNANCE tables | index.md |
| Extract source paths or markdown links from rooms | index.md |
| Claim, heartbeat, or release a room | claims.md |
| Check for agent intent conflicts before editing | claims.md |
| Parse TTL or derive agent/session IDs | claims.md |
| Send a notification event (Slack, webhook, file, stdout) | notify.md |
| Add or change a notification backend | notify.md |
| Format Slack Block Kit message from a NotifyEvent | notify.md |
| Poll a Datadog metric and fire alert callbacks | datadog.md |
| Map a Datadog scope tag to LOI rooms | datadog.md |
| Modify alert threshold evaluation or retry/backoff logic | datadog.md |

## Rooms

| Room | Source paths | Files |
|------|-------------|-------|
| index.md | internal/index/ | 6 |
| claims.md | internal/claims/ | 6 |
| notify.md | internal/notify/ | 5 |
| datadog.md | internal/datadog/ | 2 |

## See Also (flat rooms)

- [../cli.md](../cli.md) — CLI layer (cmd/, main.go)
- [../runtime.md](../runtime.md) — File watcher, git ops, test detection, codetect symbols
