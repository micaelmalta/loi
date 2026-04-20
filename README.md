# LOI — Library of Intent

> *In French, **loi** means Law. In this framework, the specification is the Law — not the code.*

AI coding assistants are fast. But without a map, they search probabilistically, duplicate abstractions, and drift from the architecture you intended. LOI gives them — and you — a deterministic, plain-English map of any codebase.

Instead of embedding, re-ranking, or hoping the right file surfaces, the model reads a three-level index (**Campus → Building → Room**) and reaches the right context in three reads. Navigation cost becomes constant regardless of codebase size.

For the full argument — why this matters, how it compares to RAG and knowledge graphs, and where the approach is heading — read the **[MANIFESTO](MANIFESTO.md)**.

---

## How it works

The index has three levels:

| Level | File | Purpose |
|-------|------|---------|
| **Campus** | `docs/index/_root.md` | Routes tasks and patterns to the right subdomain |
| **Building** | `docs/index/<subdomain>/_root.md` | Routes within a subdomain to the right room |
| **Room** | `docs/index/<subdomain>/<room>.md` | Flat list of files with structured intent metadata |

Each entry answers *what does this code do?* via `DOES`, `SYMBOLS`, `PATTERNS`, and other fields. Navigation is always three reads regardless of codebase size.

The generated index lives in your target repo under `docs/index/`. This repo contains the skill definitions, format reference, and the `loi` CLI binary.

---

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/micaelmalta/loi/main/install.sh | bash
```

The script detects Claude Code, prompts for global or per-project install, and handles updates if LOI is already present.

**Manual alternative (global):**

```bash
git clone https://github.com/micaelmalta/loi.git ~/.claude/skills/loi
```

After installation, say `"generate loi"`, `"update loi"`, or `"navigate codebase"` to trigger the skill.

---

## The `loi` binary

All operations are available as a single self-contained binary — no Python, no runtime dependency.

**Build from source:**

```bash
go build -o loi .
# Cross-compile for Linux arm64 (Raspberry Pi, etc.)
GOOS=linux GOARCH=arm64 go build -o loi-arm64 .
```

**Install to PATH:**

```bash
go install github.com/micaelmalta/loi@latest
```

```
loi validate              Validate index structure and source coverage
loi validate-patterns     Check PATTERN table entries are grounded in target rooms
loi generate              Scaffold room files from codetect symbols.db
loi governance            Aggregate GOVERNANCE WATCHLIST across repos
loi proposals             Query and validate AI-generated proposal metadata
loi diff-tables           Row-level diff for TASK/PATTERN/GOVERNANCE tables
loi claim                 Claim a room with an intent before editing
loi heartbeat             Extend an active claim's TTL
loi release               Release a room claim
loi status                Show active claims and summaries for a room
loi summary               Publish a work summary for a room
loi claims                List all active claims
loi setup-hook            Install git hooks into a target repo
loi check-stale           Pre-commit: warn when staged source has stale LOI coverage
loi watch                 Background daemon: watch for changes and react
```

---

## Quick start

In your AI agent (Claude Code, Cursor):

```
/loi generate      # build full index for a project
/loi update        # refresh stale rooms after code changes
```

In your shell:

```bash
# Validate the index
loi validate

# Validate only rooms changed in the current git diff
loi validate --changed-rooms

# Install git hooks into the current repo
loi setup-hook
```

---

## Contents

| Path | Purpose |
|------|---------|
| [`skills/loi/SKILL.md`](skills/loi/SKILL.md) | Navigation skill: Campus → Building → Room, symbol disambiguation |
| [`skills/loi-generate/SKILL.md`](skills/loi-generate/SKILL.md) | Generation skill: full index, incremental update, implement, validate |
| [`skills/loi/reference/FORMAT_REFERENCE.md`](skills/loi/reference/FORMAT_REFERENCE.md) | Field guide and schema extensions |
| [`main.go`](main.go) + [`cmd/`](cmd/) | CLI entry point and cobra subcommands |
| [`internal/`](internal/) | Core packages: git, index parsing, claims, codetect, notify, fswatch, testrun |
| [`MANIFESTO.md`](MANIFESTO.md) | Architecture rationale and the 10-level autonomy taxonomy |

---

## Level 7: watcher daemon

`loi watch` monitors `docs/index/` and fires on changes. Default mode is `notify` — validates, creates a draft PR, sends a notification. Use `--mode auto` with a `--policy` to opt into code generation.

It also watches source files in the other direction (**Code-to-Intent**): when a `.go`, `.py`, `.ts`, or other source file changes, it finds the covering LOI room and proposes an index update via draft PR.

```bash
# Notify only — Slack
loi watch --notify-backend slack --notify-url https://hooks.slack.com/...

# Auto: branch + PR, no code generation (safest auto option)
loi watch --mode auto --policy draft-only --notify-backend webhook --notify-url https://...

# Auto: implement within scopes, block on security-sensitive rooms
loi watch --mode auto --policy scoped-code-safe \
  --allowed-scopes "docs/**,tests/**" --notify-backend slack --notify-url https://...

# Auto: full autonomy with webhook notification
loi watch --mode auto --policy full-auto --notify-backend webhook --notify-url https://...
```

| Policy | Behaviour |
|--------|-----------|
| `notify-only` | Validate + notify; worker never invoked |
| `draft-only` | Branch + draft PR created; worker not invoked |
| `docs-safe` | Implement only if all source files are under `docs/` |
| `tests-safe` | Implement only if all source files are test files |
| `scoped-code-safe` | Implement only within `--allowed-scopes` globs |
| `full-auto` | No scope restriction |

After auto-mode implementation, the watcher runs the test suite automatically (`pytest`, `go test`, or `npm test` — auto-detected). On failure it marks affected rooms `architectural_health: conflicted`, commits to a `loi/conflict-<timestamp>` branch, and notifies.

See [`skills/loi-generate/SKILL.md`](skills/loi-generate/SKILL.md) for the full command reference.

---

## License

MIT
