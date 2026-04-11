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

The generated index lives in your target repo under `docs/index/`. This repo contains only the skill definition, format reference, and tooling.

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

## Quick start

In your AI agent (Claude Code, Cursor):

```
/loi generate      # build full index for a project
/loi update        # refresh stale rooms after code changes
```

In your shell (`<skill-root>` = `~/.claude/skills/loi` for Claude Code):

```bash
# Validate the index
python3 <skill-root>/scripts/validate_loi.py .

# Install the pre-push validation hook
python3 <skill-root>/scripts/setup_hook.py .
```

---

## Contents

| Path | Purpose |
|------|---------|
| [`skills/loi/SKILL.md`](skills/loi/SKILL.md) | Full skill: all modes, commands, and format rules |
| [`skills/loi/reference/FORMAT_REFERENCE.md`](skills/loi/reference/FORMAT_REFERENCE.md) | Field guide and schema extensions |
| [`skills/loi/scripts/validate_loi.py`](skills/loi/scripts/validate_loi.py) | Validates index structure and source coverage (`--changed-rooms`, `--ci`) |
| [`skills/loi/scripts/setup_hook.py`](skills/loi/scripts/setup_hook.py) | Installs pre-push validation hook into a target repo |
| [`skills/loi/scripts/validate_patterns.py`](skills/loi/scripts/validate_patterns.py) | Checks PATTERN table entries are semantically grounded in target rooms |
| [`skills/loi/scripts/diff_tables.py`](skills/loi/scripts/diff_tables.py) | Row-level diff for TASK / PATTERN / GOVERNANCE tables |
| [`skills/loi/scripts/governance.py`](skills/loi/scripts/governance.py) | Aggregates GOVERNANCE WATCHLIST entries across repos, sorted by severity |
| [`skills/loi/scripts/runtime.py`](skills/loi/scripts/runtime.py) | Advisory room claims (`claim`, `heartbeat`, `release`, `status`, `summary`) |
| [`skills/loi/scripts/proposals.py`](skills/loi/scripts/proposals.py) | Queries and validates AI-generated proposal provenance metadata |
| [`skills/loi/scripts/backends/`](skills/loi/scripts/backends/) | Pluggable notify backends: `stdout`, `file`, `webhook`, `slack` |
| [`skills/loi/scripts/watcher.py`](skills/loi/scripts/watcher.py) | Background daemon — watches `docs/index/` and triggers implementation |
| [`skills/loi/scripts/pre-commit-loi.sh`](skills/loi/scripts/pre-commit-loi.sh) | Pre-commit hook — intercepts index commits and triggers implementation |
| [`skills/loi/hooks/pre-push.sample`](skills/loi/hooks/pre-push.sample) | Pre-push hook source (installed via `setup_hook.py`) |
| [`.github/workflows/loi-committee.yml`](.github/workflows/loi-committee.yml) | CI — runs Architect + Security committee on every PR |
| [`skills/loi/tests/`](skills/loi/tests/) | Test suite (pytest) |

---

## Level 7: watcher daemon

The watcher watches `docs/index/` and fires on changes. Default mode is `notify` — validates, creates a draft PR, sends a notification. Use `--mode auto` with a `--policy` to opt into code generation.

```bash
# Notify only
uv run --with watchdog watcher.py --notify-backend slack --notify-url https://hooks.slack.com/...

# Auto: branch + PR, no code generation
uv run --with watchdog watcher.py --mode auto --policy draft-only --notify-backend webhook --notify-url https://...

# Auto: implement within scopes, block on security-sensitive rooms
uv run --with watchdog watcher.py --mode auto --policy scoped-code-safe \
  --allowed-scopes "docs/**,tests/**" --notify-backend slack --notify-url https://...
```

See [`SKILL.md`](skills/loi/SKILL.md) for the full policy tier reference and all commands.

---

## License

MIT
