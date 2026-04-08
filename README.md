# LOI — Library of Intent

An agent skill that gives AI models a deterministic, plain-English map of a codebase. Instead of guessing with broad search, the model navigates **Campus → Building → Room** and reaches the right file in three reads.

The generated index lives in your target repo under `docs/index/`. This repository contains only the skill definition and format reference.

---

## The problem with search-based navigation

When a model doesn't know a codebase, it explores by searching — running `grep` and `glob` commands, opening files to understand their purpose, and repeating until it finds the right entry point. On a large repo, this burns hundreds of tool calls and thousands of tokens *before any real work starts*.

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

## Contents

| Path | Purpose |
|------|---------|
| [`skills/loi/SKILL.md`](skills/loi/SKILL.md) | Full skill: navigation, full generate, incremental update, layouts, staleness checks |
| [`skills/loi/reference/FORMAT_REFERENCE.md`](skills/loi/reference/FORMAT_REFERENCE.md) | Field guide and examples for LOI entries (`DOES`, `SYMBOLS`, `ROUTES`, etc.) |

---

## Installation

Copy or symlink the `skills/loi` folder so `SKILL.md` is at `<skill-root>/loi/SKILL.md`:

```text
<skill-root>/loi/SKILL.md
<skill-root>/loi/reference/FORMAT_REFERENCE.md
```

**Claude Code:** `~/.claude/skills/loi` (global) or `<project>/.claude/skills/loi` (per-project).

**Cursor:** place the `loi` folder in your agent skills directory per your version's docs.

After installation, phrases like `"generate loi"`, `"update loi"`, or `"where is …"` trigger the skill automatically.

---

## Indexing a codebase

**First time or after major structural changes — full generate:**
Discover all source trees, group into subdomains and rooms, generate entries per `FORMAT_REFERENCE.md`, write `docs/index/**`, and verify every source file is covered.

**Day-to-day — incremental update:**
Detect stale rooms via git, regenerate only those rooms, then refresh the campus and building routers.

Both modes use **git** for staleness checks. For large codebases, generation is parallelized via the [RLM pattern](https://github.com/Bowtiedswan/rlm-skill) — the skill falls back to per-subdomain agents if RLM is unavailable.

---

## License

MIT
