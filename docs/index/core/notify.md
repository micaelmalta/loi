---
room: core/notify
see_also:
  - core/claims.md
  - core/index.md
architectural_health: normal
security_tier: normal
hot_paths: backend.go, slack.go
---

# LOI Room: core/notify

Source paths: internal/notify/

## Entries

# backend.go

DOES: Defines the `NotifyBackend` interface and `NotifyEvent` struct; `LoadBackend` factory dispatches to stdout, file, webhook, or slack backends based on a config map.
SYMBOLS:
- LoadBackend(config map[string]string) (NotifyBackend, error)
- Types: NotifyBackend, NotifyEvent
TYPE: NotifyEvent { Type string; Timestamp time.Time; Repo, Path, Summary, PRURL, TableDiff string; Governance map[string]string; Rooms, TestOutput []string }
PATTERNS: strategy-pattern, factory
USE WHEN: Constructing the backend at watcher startup; any caller that needs to dispatch a structured LOI event.

---

# file.go

DOES: Appends JSON-encoded `NotifyEvent` lines to a log file; thread-safe via `sync.Mutex`; validates path writability on construction.
SYMBOLS:
- newFileBackend(path string) (*fileBackend, error)
PATTERNS: jsonl-append
USE WHEN: Persisting LOI events to disk for audit or replay.

---

# slack.go

DOES: Posts `NotifyEvent` to a Slack incoming webhook as a Block Kit message; builds a header block (event type + repo), fields section (path, governance health/security, summary), optional table-diff code block (truncated at 2000 chars), PR action button, and context timestamp.
SYMBOLS:
- newSlackBackend(url string) *slackBackend
- buildSlackBlocks(e NotifyEvent) []slackBlock
PATTERNS: slack-block-kit
USE WHEN: Routing LOI change events to a Slack channel with rich formatting.

---

# stdout.go

DOES: Prints each `NotifyEvent` as a single JSON line to stdout; used as the default backend when no external sink is configured.
SYMBOLS:
- stdoutBackend.Send(e NotifyEvent) error
USE WHEN: Local development or piped output where no external sink is needed.

---

# webhook.go

DOES: POSTs JSON-encoded `NotifyEvent` to an arbitrary HTTP endpoint; reads an optional bearer token at send time from a named environment variable.
SYMBOLS:
- newWebhookBackend(url, tokenEnv string) *webhookBackend
USE WHEN: Sending LOI events to any HTTP receiver; the tokenEnv param avoids hardcoding secrets.

---
