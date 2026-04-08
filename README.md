# LOI

**Library of Intent** — an agent skill for building and using a deterministic, plain-English index of a codebase so models can navigate **Campus → Building → Room** instead of guessing with broad search.

This repository contains the skill definition and the canonical format for LOI entries. The **generated** index lives in a target repo under `docs/index/` (not in this package).

## Contents

| Path | Purpose |
|------|---------|
| [`skills/loi/SKILL.md`](skills/loi/SKILL.md) | Full skill: navigation mode, full generate, incremental update, `_root.md` layouts, staleness checks |
| [`skills/loi/reference/FORMAT_REFERENCE.md`](skills/loi/reference/FORMAT_REFERENCE.md) | Field guide and examples for each LOI entry (`DOES`, `SYMBOLS`, `ROUTES`, etc.) |

## Concepts

- **Campus** — `docs/index/_root.md`: task → load and pattern → load tables, plus the list of subdomain “buildings.”
- **Building** — `docs/index/<subdomain>/_root.md`: router for that area (e.g. `infra/`, `identity/`, `api/`).
- **Room** — `docs/index/<subdomain>/<room>.md`: flat lists of files with structured intent metadata (see the format reference).

Grouping is by **responsibility**, not by folder or language. The skill document describes subdomain templates, splitting rules, and how to verify coverage.

## Using the skill

Install the skill where your agent loads skills from, keeping this layout:

```text
<skill-root>/loi/SKILL.md
<skill-root>/loi/reference/FORMAT_REFERENCE.md
```

Examples:

- **Claude Code:** copy or symlink `skills/loi` to `~/.claude/skills/loi` (or your project’s skills path) so `SKILL.md` is at `.../loi/SKILL.md`.
- **Cursor:** follow [Agent Skills](https://cursor.com/docs) for your version — typically a project or user skills directory containing the `loi` folder as above.

After installation, natural phrases like “generate loi,” “update loi,” “navigate codebase,” or “where is …” should match the skill’s triggers (see the YAML front matter in `SKILL.md`).

## Applying it to another codebase

1. Run **full generate** when first setting up or after large structural changes: discover all source trees, group into subdomains/rooms, generate entries per `FORMAT_REFERENCE.md`, write `docs/index/**` and verify every source file appears somewhere.
2. Use **incremental generate** for day-to-day edits: detect stale rooms with git, regenerate only those rooms, then refresh the campus and building routers.

The skill assumes **git** for staleness and history commands. For workflows that parallelize generation, it references the **Recursive Language Model (RLM)** pattern (`/rlm`). A concrete installable skill for that workflow is [BowTiedSwan/rlm-skill](https://github.com/Bowtiedswan/rlm-skill); you can fall back to manual or per-subdomain agents if needed.

## License

Add a license file if you intend to publish this repo; none is bundled here.
