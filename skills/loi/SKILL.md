---
name: loi
description: >
  Codebase "Library of Intent" for LLM navigation. Replaces probabilistic global searching with a deterministic, plain-English navigation hierarchy. Uses Campus → Building → Room nested indices to scale indefinitely. Use as the default codebase navigation method when a _root.md exists under docs/index/.
triggers:
  - /loi
  - "navigate codebase using loi"
---

## Navigate

Fast three-read navigation: Campus → Building → Room. Zero greps.

1. **Read Campus Map:** Load `docs/index/_root.md`. Always follow path
2. **Pick a Building:** Use the `TASK → LOAD` table for domain tasks, or `PATTERN → LOAD` table for cross-cutting behavioral patterns (retry, backoff, caching, etc.). Select a subdomain (`docs/index/<subdomain>/_root.md`) or a top-level domain file.
3. **Pick a Room:** The building router points to a domain file. Load it. Check `see_also` in the room's YAML header for related rooms to pre-fetch.
4. **Staleness Check:** For each index file loaded, run:
   ```bash
   git diff --name-only $(git log -1 --format=%H -- <index-file>) HEAD -- <source-paths>
   ```
   Flag if files changed: `Index may be stale — [files] changed after <domain>.md.`
5. **Surgical Entry:** Use DOES (intent) and SYMBOLS (signatures) to jump directly to the target. If multiple SYMBOLS match, defer to Step 6 before selecting.
6. **Branch Resolution for Named Symbols:**
   If the query names a function, method, or error case:
   - locate the defining file via LOI
   - read the exact function body
   - identify the branch named in the question
   - trace to the terminal return / throw / emit
   - answer from code, not index summary
   - preserve exact semantics: wrapped, joined, translated, retried, ignored, or propagated

7. **Resolve Symbol Ambiguity:**
   If multiple matching symbols exist (same name at different layers):
   - **Default to the highest-level symbol** — the public API, exported function, or entry-point that callers invoke directly.
   - Only descend to a lower-level implementation if the query explicitly names an internal helper, a specific package path, or asks about the implementation detail itself.
   - To confirm which is highest-level: check which symbol is exported/public and which is called *by* the other — the caller is higher level.
   - State which symbol you are answering about and why (e.g. "answering about `Foo` in `pkg/api` — it is the exported entry-point; `Foo` in `internal/core` is the implementation it delegates to").
