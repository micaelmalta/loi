---
name: loi
description: >
  Codebase "Library of Intent" for LLM navigation. Replaces probabilistic global searching with a deterministic, plain-English navigation hierarchy. Uses Campus → Building → Room nested indices to scale indefinitely. Use as the default codebase navigation method when docs/index/_root.md exists. Use for generating or updating the full index when requested. Depends on /rlm skill for full-generate mode (falls back to manual agents if unavailable).
triggers:
  - /loi
  - /loi generate
  - /loi update
  - /loi validate
  - /loi implement
  - "generate loi"
  - "full loi"
  - "rebuild index"
  - "update loi"
  - "navigate codebase using loi"
  - "full codebase index"
  - "implement loi changes"
  - "sync intent to code"
---

## Navigate (Default Mode)

Fast three-read navigation: Campus → Building → Room. Zero greps.

1. **Read Campus Map:** Load `docs/index/_root.md`.
2. **Pick a Building:** Use the `TASK → LOAD` table for domain tasks, or `PATTERN → LOAD` table for cross-cutting behavioral patterns (retry, backoff, caching, etc.). Select a subdomain (`docs/index/<subdomain>/_root.md`) or a top-level domain file.
3. **Pick a Room:** The building router points to a domain file. Load it. Check `see_also` in the room's YAML header for related rooms to pre-fetch.
4. **Staleness Check:** For each index file loaded, run:
   ```bash
   git diff --name-only $(git log -1 --format=%H -- <index-file>) HEAD -- <source-paths>
   ```
   Flag if files changed: `Index may be stale — [files] changed after <domain>.md.`
5. **Surgical Entry:** Use DOES (intent) and SYMBOLS (signatures) to jump directly to the target.

---

## Full-Generate Mode

Build a complete index from scratch. Use for first-time setup or major architectural shifts.

**When to use:** First-time setup, user requests "generate full loi", "rebuild entire index", or for large codebases where incremental updates miss structural changes.

### Process

**Invoke the RLM skill** (via `Skill tool` with `skill: "rlm"`) for all discovery and generation steps. Do NOT run bash file discovery commands directly — delegate to RLM.

**1. Discover ALL source directories via RLM Index & Filter phase:**
- Use RLM's Phase 1 (Native Mode) to census the repo:
  - Glob `**/*.go`, `**/*.ts`, `**/*.tsx`, `**/*.py`, `**/*.js`, `**/*.rb` (adapt per project)
  - Exclude: `vendor/`, `node_modules/`, `__pycache__/`, `.git/`, generated files
- Use RLM's `fd`/`rg` tools (falling back to `find`/`grep`) to count files per directory and identify top-level subdomain boundaries

**Every top-level directory with source files MUST be included.** Non-standard directories (`packages/`, `engines/`, `gems/`) often contain more code than `app/` or `src/`. Omitting them produces a partial index.

**NO SAMPLING.** Do not pick "representative" directories. Every source directory discovered in step 1 must be assigned to a room. If the census finds N directories, all N must appear in the index.

**2. Build a coverage checklist before writing any files** — After the census, produce a flat list of every source directory with its file count. This is your ground truth. Assign each directory to a room before proceeding. Do not write any index files until every directory has a room assignment.

```
[ ] <dir>/   <N> files  → (unassigned)
[x] <dir>/   <N> files  → <subdomain>/<room>.md
```

**3. Filter source files** — via RLM's Glob tool by language (RLM handles exclusions automatically via `.gitignore`).

**4. Group into Subdomains and Domains** — Organize by **responsibility**, not language or directory. The hierarchy is:

```
docs/index/
├── _root.md                 # CAMPUS: Routes to Buildings
├── infra/                   # BUILDING: System-wide wiring
│   ├── _root.md             # Building Router
│   ├── startup.md           # Room: Boot, DI, config loading
│   └── external_apis.md     # Room: Stripe, AWS, SendGrid
├── identity/                # BUILDING: User management
│   ├── _root.md             # Building Router
│   ├── auth_flow.md         # Room: JWT, OAuth, login/logout
│   └── permissions.md       # Room: RBAC, scopes, roles
└── <business_domain>/       # BUILDING: The core business logic
    ├── _root.md
    ├── core.md              # Room: Most frequent entry points
    └── legacy.md            # Room: Older code, adapters, bridges
```

#### Subdomain Categories

Use these as starting points — rename to match the codebase's actual language:

| Subdomain | What lives here | Example rooms |
|-----------|----------------|---------------|
| **infra/** | Boot, config, DI, migrations, server wiring | `startup.md`, `config.md`, `migrations.md` |
| **identity/** | Auth, sessions, users, roles, permissions | `auth_flow.md`, `permissions.md`, `users.md` |
| **api/** | HTTP/gRPC/GraphQL handlers, middleware, routing | `routes.md`, `middleware.md`, `serializers.md` |
| **data/** | Repositories, queries, ORM models, caching | `repositories.md`, `models.md`, `cache.md` |
| **integrations/** | External API clients, webhooks, SDKs | `stripe.md`, `aws.md`, `email.md` |
| **workers/** | Background jobs, queues, cron, event consumers | `jobs.md`, `consumers.md`, `schedulers.md` |
| **shared/** | Logging, errors, formatters, test helpers | `errors.md`, `logging.md`, `utilities.md` |
| **`<business>/`** | Core business logic (name after the domain) | `catalog/`, `orders/`, `fulfillment/` |

#### Splitting Rules

- **Subdomains are the default.** Every codebase gets at least `infra/` and one business domain.
- **Flat top-level domain files are the exception** — only for tiny groups (<5 files) that don't warrant a folder. Example: a single `config.md` at the top level is fine if there are only 2 config files.
- **Never split by alphabet.** `A-M.md` / `N-Z.md` is meaningless. Always split by functional concern.
- **Room file size limit: ~150 entries.** If a room would exceed this, split it into multiple rooms within the same building.
- **Business logic gets its own building.** If the codebase serves multiple business domains (catalog, orders, fulfillment), each gets a subdomain folder — not one giant `business.md`.
- Merge tiny rooms (<3 files) into the nearest neighbor. Don't create a room for 1 file.

**5. Generate (via RLM Skill with Consensus Loop)** — Use RLM's parallel processing with a 3-step pipeline: Map -> Critique -> Reduce.
- **Phase A (Map):** Spawn one worker agent per room. "Read all files in [file list]. Produce a LOI entry following `reference/FORMAT_REFERENCE.md`."
- **Phase B (Critique / The Committee):** Pass the mapped drafts to specialized personas before finalizing:
  - *Architect Persona:* "Does this room mix concerns? (e.g., HTTP parsing next to database logic). If yes, set `architectural_health: warning` or `critical` and write a `committee_notes` explanation."
  - *Security Persona:* "Does this room handle raw SQL, PII, or auth tokens? If yes, set `security_tier: sensitive` or `high`."
- **Phase C (Reduce):** "Merge the critiqued LOI outputs into the subdomain structure. At the top of each room file, generate the YAML metadata header including `see_also`, `hot_paths`, and the exact Governance Flags determined in Phase B. Build `docs/index/_root.md` with task/pattern mapping tables."
- **Agent count**: spawn one worker agent per room, plus the Committee personas for the Critique phase. If `/rlm` is unavailable, fall back to spawning one background Agent per subdomain to simulate the Committee.

**6. Write index files:**
- `docs/index/_root.md` — Campus map (see format below)
- `docs/index/<subdomain>/_root.md` — Building router per subdomain
- `docs/index/<subdomain>/<room>.md` — Domain files with flat entry lists

**7. Verify coverage** — Cross-check the coverage checklist from step 2 against the written index files. Every directory on the checklist must appear in at least one room file. If any remain uncovered, generate those rooms before declaring the index complete.

### _root.md Format (Campus Map)

```markdown
# LOI Index

Generated: 2026-04-07
Source paths: internal/, cmd/, pkg/

## TASK → LOAD

| Task | Load |
|------|------|
| <task description> | <subdomain>/_root.md |
| <task description> | <subdomain>/<room>.md |
| ... | ... |

## PATTERN → LOAD

Cross-cutting behavioral patterns that span multiple rooms.

| Pattern | Load |
|---------|------|
| Exponential backoff / retry | <all rooms whose PATTERNS field includes it> |
| Circuit breaker / fault tolerance | ... |
| Middleware chain | ... |
| Event publishing / async messaging | ... |

## 🚨 GOVERNANCE WATCHLIST

Rooms flagged by the RLM Committee for architectural drift or security audits.

| Room | Health | Security | Committee Note |
|------|--------|----------|----------------|
| `identity/legacy_auth.md` | `critical` | `high` | "Mixing JWT generation with direct DB queries. Needs extraction." |
| `api/payments.md` | `warning` | `sensitive` | "Stripe keys accessed directly in handler instead of config struct." |

## Buildings

| Subdomain | Description | Rooms |
|-----------|-------------|-------|
| infra/ | Boot, config, DI, migrations | <room>.md, ... |
| identity/ | Auth, sessions, users, RBAC | <room>.md, ... |
| api/ | HTTP handlers, middleware | <room>.md, ... |
| ... | ... | ... |
```

### Subdomain _root.md Format (Building Router)

```markdown
# <Subdomain Name>

Subdomain: <subdomain>/
Source paths: <paths covered by this subdomain>

## TASK → LOAD

| Task | Load |
|------|------|
| <task description> | <room>.md |
| ... | ... |

## Rooms

| Room | Source paths | Files |
|------|-------------|-------|
| <room>.md | <source paths> | <count> |
| ... | ... | ... |
```

---

## Incremental-Generate Mode

Regenerate only stale domains (files changed since last index).

**When to use:** After code changes, user says "update loi" or "rebuild index" (not "full"), or during iterative development.

**Process:**

1. **Detect stale rooms** — For each domain file across all subdomains:
   ```bash
   git diff --name-only $(git log -1 --format=%H -- docs/index/<subdomain>/<room>.md) HEAD -- <source-paths>
   ```
   Mark rooms where source files changed.

2. **Regenerate only stale rooms** — Invoke `/rlm` (via the Skill tool with `skill: "rlm"`) with only the stale rooms as map inputs; if `/rlm` is unavailable, spawn agents manually per room.

3. **Merge results** — Update only affected domain files; leave others unchanged.

4. **Update routers** — Refresh `_root.md` entries (both campus and building level) for any changed rooms.

---

## Implement Mode (Level 7 — Bi-Directional Sync)

Reverse the flow: edits to the markdown index drive changes in source code. The LOI becomes an executable contract.

**When to use:** User edits a `DOES`, `SYMBOLS`, or other intent field in a `docs/index/` room file and wants the source code updated to match the new intent. Triggered by `/loi implement`, `"implement loi changes"`, or `"sync intent to code"`.

### Process

**1. Detect intent delta** — Diff the modified room file against its last committed version:

```bash
git diff HEAD -- docs/index/<subdomain>/<room>.md
```

Parse the diff to extract changed entries. For each changed entry, identify:
- **File path**: The `# filename.ext` heading (deterministic — the LOI maps every entry to an exact source file)
- **Old intent**: Previous `DOES`, `SYMBOLS`, `TYPE`, `INTERFACE`, `PATTERNS` values
- **New intent**: Updated values from the markdown

If no intent fields changed (e.g., only whitespace or `see_also`), skip that entry.

**2. Validate the delta** — Before touching source code:
- Confirm every referenced source file exists on disk. If a file was deleted or renamed, flag it and halt for that entry.
- Check that the new intent is actionable (not empty, not identical to old).

**3. Create an implementation branch:**

```bash
git checkout -b loi/implement-<room>-<timestamp>
```

Never work directly on the current branch. All changes happen on a dedicated branch.

**4. Implement changes** — For each changed entry, construct a worker prompt:

```
The Architect has updated the Contract for <source-file>.

Old Intent: <old DOES value>
New Intent: <new DOES value>

Old Symbols: <old SYMBOLS, if changed>
New Symbols: <new SYMBOLS, if changed>

Task: Read <source-file>. Refactor the code to fulfill the New Intent.
- Preserve existing function signatures unless the New Intent explicitly demands changes.
- Add new functions/types if the New Symbols field includes them.
- Do not remove existing exports unless the New Intent removes their purpose.
- Follow existing code style and patterns in the file.
```

For large rooms with many changed entries, use the RLM skill (`skill: "rlm"`) to parallelize — one worker agent per changed entry. If RLM is unavailable, process entries sequentially.

**5. Run the test suite** — After all source changes are written:

```bash
# Detect project type and run appropriate tests
# Go: go test ./...
# Node: npm test
# Python: pytest
```

If tests fail:
- Read the failure output. Attempt a fix (max 2 retries).
- If tests still fail after retries, **do not commit**. Report the failures and leave the branch for human review.

**6. Update the LOI index** — Regenerate the room file for any source files that were modified (the code may have gained new symbols, types, etc. beyond what the user specified). This keeps the index in sync with reality.

**7. Commit and push:**

```bash
git add <modified-source-files> docs/index/<affected-rooms>
git commit -m "loi/implement: <summary of intent changes>"
git push -u origin loi/implement-<room>-<timestamp>
```

**8. Open a Pull Request** — Create a PR with:
- Title: `LOI Implement: <room> intent sync`
- Body: Table of old → new intents, list of modified source files, test results
- Label: `loi-implement`

The PR requires human approval before merge. The AI never merges its own work.

### Safety Rails

- **Branch isolation**: All changes happen on a fresh branch. The working branch is never modified.
- **Test gate**: Code is only committed if the test suite passes.
- **No force-push**: Never use `--force`. If the branch exists, create a new one with an incremented timestamp.
- **Human approval**: PRs are opened for review, never auto-merged.
- **Atomic entries**: Each entry (source file) is implemented independently. A failure in one entry does not block others.

### Automation Options

The `/loi implement` command is the IDE-native way (Option A). Two additional automation hooks are provided:

- **Option B — Pre-Commit Hook**: `skills/loi/scripts/pre-commit-loi.sh` intercepts commits that modify `docs/index/` and triggers implementation before the commit completes.
- **Option C — Background Daemon**: `skills/loi/scripts/watcher.py` monitors `docs/index/` for changes with three modes:
  - `--mode notify` (default): validate → create draft PR → Slack notification. No code changes until you approve.
  - `--mode auto`: validate → implement via AI → commit → PR. Full autonomy, opt-in.
  - `--mode dry-run`: log only.

  Slack notifications use `--slack-webhook` (incoming webhook URL, preferred) or fall back to `--slack-channel` (sends via Slack MCP tool if available). Changes within the batch window (`--debounce`, default 5s) are grouped into a single PR.

See the README for setup instructions.

---

## Format Rules

All LOI entry format details, field guide, examples, and anti-patterns are in `reference/FORMAT_REFERENCE.md`. Load it when generating or reviewing entries.

Key principles:
- **DOES**: Always required. Specific action/outcome, never generic.
- **SYMBOLS**: Full signatures with params and return types.
- **No prose, no empty keys.** Omit fields that don't apply.

---

### Mandatory Metadata Header (Predictive Hooks)
Every domain/room file MUST begin with a YAML frontmatter block. This enables predictive context loading (Level 5) for cross-domain navigation.

```yaml
---
room: [Room Name]
see_also: ["../infra/database.md"]
hot_paths: "If editing X -> remember to update Y"
# --- LEVEL 6 GOVERNANCE FLAGS ---
security_tier: "high" # Flagged by Security Agent
architectural_health: "warning" # Flagged by Architect
committee_notes: "This room is handling both HTTP parsing and business logic. Consider splitting in the next refactor."
---
```

---

## Validate Mode

Verify the structural integrity and coverage of a generated LOI index.

**When to use:** After generating or updating an index, or when `/loi validate` is triggered.

**Process:**

Run the validation script:

```bash
python3 skills/loi/scripts/validate_loi.py <project-root>
```

The script checks:
- `docs/index/_root.md` exists and has TASK → LOAD table
- All subdomain `_root.md` routers exist and reference valid room files
- All room `.md` files have YAML frontmatter with required fields (`room`, `see_also`)
- Every room file referenced in a router actually exists on disk
- Every source directory with code files is covered by at least one room
- No room exceeds ~150 entries

Fix any reported errors before considering the index complete.

---

## Staleness Policy

Index files become stale when source files change:
- **For navigation:** Always check staleness before providing code locations
- **For regeneration:** If >30% of files in a room changed, regenerate entire room

```bash
# Last update to domain file
git log -1 --format=%ai -- docs/index/<subdomain>/<room>.md

# Latest change in source paths
git log -1 --format=%ai -- <source-paths>

# Diff to see what changed
git diff --name-only $(git log -1 --format=%H -- docs/index/<subdomain>/<room>.md) HEAD -- <source-paths>
```
