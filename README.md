# LOI — Library of Intent

An agent skill that gives AI models a deterministic, plain-English map of a codebase. Instead of guessing with broad search, the model navigates **Campus → Building → Room** and reaches the right file in three reads.

The generated index lives in your target repo under `docs/index/`. This repository contains only the skill definition and format reference.

---

## The problem with search-based navigation

When a model doesn't know a codebase, it explores by searching — running `grep`, `glob`, `fd`, and `rg` commands, opening files to understand their purpose, and repeating until it finds the right entry point. On a large repo, this burns hundreds of tool calls and thousands of tokens *before any real work starts*.

Worse, every file loaded to answer "where is this?" stays in the context window even after it's no longer relevant. A few rounds of exploratory search can fill half the context with code that has nothing to do with the current task. Response quality degrades, the model loses track of earlier context, and eventually a reset is forced — discarding all accumulated understanding.

LOI eliminates this loop entirely.

---

## How it works

LOI is a structured index organized by **intent**, not by folder or file name. Each entry answers: *what does this code do?* — not just what it's called.

The index has three levels:

| Level | File | Purpose |
|-------|------|---------|
| **Campus** | `docs/index/_root.md` | Routes tasks and patterns to the right subdomain |
| **Building** | `docs/index/<subdomain>/_root.md` | Routes within a subdomain to the right room |
| **Room** | `docs/index/<subdomain>/<room>.md` | Flat list of files with structured intent metadata |

Navigation is always three reads: campus → building → room. The model arrives at the correct source file with no grep, no file scanning, and a minimal context footprint. Structured `DOES` and `SYMBOLS` fields often mean the source file never needs to be opened at all just to understand a component.

Grouping is by **responsibility**, not by directory or language. An `infra/` building holds boot, config, and DI regardless of where those files live on disk.

---

## How it differs from RAG and graph tools

Several tools already exist for AI codebase navigation. LOI occupies a different position.

**RAG (vector search)** embeds source files and retrieves the most semantically similar chunks for a given query. It works well for fuzzy, open-ended questions but has three weaknesses for structured navigation: retrieval is probabilistic (the right chunk may not rank first), the model has no map of the codebase (it gets fragments, not structure), and every query re-embeds and re-retrieves — there is no persistent, human-readable representation you can inspect or correct.

**Graph tools** (call graphs, symbol graphs, dependency maps) are precise about *relationships* — what calls what, what imports what. They are weak on *intent* — they tell you that `A` calls `B` but not what `A` is trying to accomplish. Navigating a graph still requires the model to reason about structure rather than purpose, and graphs are expensive to build and keep current.

**LOI** trades probabilistic retrieval for deterministic lookup. The index is built once by a model that reads the code and writes plain-English intent summaries; future models navigate by reading those summaries, not by searching. The index is a small set of markdown files — inspectable, editable, and version-controlled alongside the code. Navigation cost is fixed at three reads regardless of codebase size.

| | RAG | Graph | LOI |
|---|---|---|---|
| Navigation style | Probabilistic retrieval | Structural traversal | Deterministic lookup |
| Answers "what does this do?" | Partially | No | Yes |
| Human-readable / editable | No | No | Yes |
| Cost scales with codebase size | Yes (embedding) | Yes (graph build) | No (fixed 3 reads) |
| Stays in context window | Chunks pile up | Subgraphs pile up | 3 small index files |

---

## Token savings in practice

| Without LOI | With LOI |
|-------------|----------|
| 5–20 file reads per navigation | 3 index reads per navigation |
| Source files pile up in context | Only the relevant room stays in context |
| Context fills with irrelevant code | Source files loaded only when editing |
| Exploratory search before every task | Zero search — intent-first lookup |

---

## Current status

LOI implements **Levels 4 through 7** of the [10-level autonomy roadmap](LEVELS.md):

| Level | Status | What it does |
|-------|--------|--------------|
| 4 — The Cartographer | Shipped | Deterministic Campus/Building/Room navigation in 3 reads |
| 5 — Predictive Context | Shipped | `see_also` and `hot_paths` metadata for cross-domain pre-fetching |
| 6 — Multi-Agent Governance | Shipped | Architect + Security personas flag drift and risk in YAML headers |
| 7 — Intent-Driven Autonomy | Shipped | `/loi implement` — edit markdown, AI writes code, opens PR |

Levels 1-3 describe the industry baseline (autocomplete, chat, grep-based agents) that LOI replaces. Levels 8-10 are the forward roadmap.

## Roadmap

| Level | Target | Description |
|-------|--------|-------------|
| **8 — Telemetry-Driven Autonomy** | Next | AI connects to APM (Datadog, Sentry), traces production issues back to LOI rooms, and opens fix PRs with benchmarks |
| **9 — Metric-Driven Development** | Future | AI connects to analytics (PostHog, Mixpanel), writes features to optimize business KPIs, deploys via A/B tests |
| **10 — Dynamic Synthesis** | Vision | No static repo — AI provisions infrastructure and writes services on demand based on real-time traffic |

See [`LEVELS.md`](LEVELS.md) for the full breakdown of all 10 levels.

---

## Contents

| Path | Purpose |
|------|---------|
| [`skills/loi/SKILL.md`](skills/loi/SKILL.md) | Full skill: navigation, generate, update, implement (Level 7), staleness checks |
| [`skills/loi/reference/FORMAT_REFERENCE.md`](skills/loi/reference/FORMAT_REFERENCE.md) | Field guide and examples for LOI entries (`DOES`, `SYMBOLS`, `ROUTES`, etc.) |
| [`skills/loi/scripts/validate_loi.py`](skills/loi/scripts/validate_loi.py) | Validates index structure, cross-references, frontmatter, and source coverage |
| [`skills/loi/scripts/watcher.py`](skills/loi/scripts/watcher.py) | Level 7 background daemon — watches `docs/index/` and triggers AI implementation |
| [`skills/loi/scripts/pre-commit-loi.sh`](skills/loi/scripts/pre-commit-loi.sh) | Level 7 pre-commit hook — intercepts index commits and triggers implementation |
| [`.github/workflows/loi-committee.yml`](.github/workflows/loi-committee.yml) | CI/CD pipeline — runs RLM Committee (Architect + Security) on every PR |

---

## Installation

Copy or symlink the `skills/loi` folder so `SKILL.md` is at `<skill-root>/loi/SKILL.md`:

```text
<skill-root>/loi/SKILL.md
<skill-root>/loi/reference/FORMAT_REFERENCE.md
```

**Claude Code:** `~/.claude/skills/loi` (global) or `<project>/.claude/skills/loi` (per-project).

**Cursor:** place the `loi` folder in your agent skills directory per your version's docs.

After installation, phrases like `"generate loi"`, `"update loi"`, or `"navigate codebase using loi"` trigger the skill automatically.

---

## Indexing a codebase

**First time or after major structural changes — full generate:**
Discover all source trees, group into subdomains and rooms, generate entries per `FORMAT_REFERENCE.md`, write `docs/index/**`, and verify every source file is covered.

**Day-to-day — incremental update:**
Detect stale rooms via git, regenerate only those rooms, then refresh the campus and building routers.

Both modes use **git** for staleness checks. File discovery uses `fd` and `rg` for fast directory census and content filtering. For large codebases, generation is parallelized via the [RLM pattern](https://github.com/Bowtiedswan/rlm-skill) — the skill falls back to per-subdomain agents if RLM is unavailable.

**Validation:**
After generating or updating an index, run `python3 skills/loi/scripts/validate_loi.py <project-root>` to verify structural integrity and source coverage.

---

## Level 7: Full Autonomy

Level 7 transforms LOI from a read-only index into an executable contract. Edit the markdown, and the AI updates the code to match.

### `/loi implement` — Bi-Directional Sync

The primary IDE-native workflow. After editing intent fields (`DOES`, `SYMBOLS`, etc.) in a room file:

1. Run `/loi implement` (or say "implement loi changes")
2. The skill diffs the markdown against the last commit
3. For each changed entry, the AI navigates to the exact source file, implements the new intent, and runs the test suite
4. Changes are committed on a dedicated branch and a PR is opened for review

### Automation Options

Three ways to trigger implementation automatically:

| Option | Mechanism | Setup |
|--------|-----------|-------|
| **A — IDE Native** | `/loi implement` in Claude Code or Cursor | Built into SKILL.md — no extra setup |
| **B — Pre-Commit Hook** | Triggers on `git commit` when `docs/index/` files are staged | `cp skills/loi/scripts/pre-commit-loi.sh .git/hooks/pre-commit && chmod +x .git/hooks/pre-commit` |
| **C — Background Daemon** | Watches `docs/index/` for saves, batches changes | `uv run --with watchdog skills/loi/scripts/watcher.py --watch-path .` |

The daemon defaults to **notify mode** — validates changes, creates a draft PR, and sends a Slack notification. You approve the PR, then run `/loi implement` to generate code.

```bash
# Notify mode with Slack webhook (preferred)
uv run --with watchdog watcher.py --slack-webhook https://hooks.slack.com/services/...

# Notify mode with Slack MCP fallback (uses claude to send via MCP)
uv run --with watchdog watcher.py --slack-channel "#loi-approvals"

# Full auto mode (validates, implements, PRs — no human gate)
uv run --with watchdog watcher.py --mode auto
```

### CI/CD — LOI Committee on Pull Requests

The `.github/workflows/loi-committee.yml` action runs the RLM Committee on every PR:

- **Architect persona**: Flags mixed concerns, coupling, missing abstractions
- **Security persona**: Flags raw SQL, PII exposure, hardcoded secrets

Setup: Add an `ANTHROPIC_API_KEY` secret to your repository. The action posts committee findings as PR comments and can block merges on critical flags.

---

## License

MIT
