---
room: core/datadog.md
see_also:
  - core/notify.md
  - ../cli.md
architectural_health: normal
security_tier: sensitive
---

# LOI Room: core/datadog

Source paths: internal/datadog/

## Entries

# client.go

DOES: Wraps the Datadog Metrics Query API (`GET /api/v1/query`). Authenticates via `DD-API-KEY` and `DD-APPLICATION-KEY` headers. Returns the last data point per series. Retries on 429 with exponential backoff (1s→2s→4s, cap 60s, max 5 attempts). Returns `ErrAuthFailure` on 403 to stop the poll loop.
SYMBOLS:
- NewClient(apiKey, appKey string) *Client
- (c *Client) QueryLastValues(ctx context.Context, query string, window time.Duration) ([]Series, error)
- ErrAuthFailure
- Types: Client, Series
DEPENDS: internal/index
PATTERNS: exponential-backoff, retry

---

# poller.go

DOES: Runs a blocking poll loop at a configurable interval, calling `client.QueryLastValues` each tick, evaluating a threshold condition (`>`, `>=`, `<`, `<=`), and invoking `AlertCallback` for each breaching series. Maps metric scope tags (e.g. `service:auth`) to LOI room paths via `index.FindCoveringRooms` by stripping the `key:` prefix and treating the value as a source path component.
SYMBOLS:
- Poll(ctx context.Context, cfg PollConfig, client *Client) error
- breaches(value, threshold float64, op string) bool
- mapScopeToRooms(projectRoot, scope string) []string
- Types: PollConfig, AlertCallback
DEPENDS: internal/index
PATTERNS: poll-loop, strategy-pattern
USE WHEN: Modifying alert evaluation logic, scope-to-room mapping heuristic, or poll interval/backoff behaviour.
