---
name: loi-generate
description: >
  Generate, update, validate, and implement LOI (Library of Intent) indices. Handles full index generation, incremental updates, codetect-based scaffolding, validation, and bi-directional intent sync (code ↔ index). Uses parallel Agent workers for generation.
triggers:
  - /loi-generate
  - /loi generate
  - /loi update
  - /loi validate
  - /loi implement
  - "generate loi"
  - "full loi"
  - "rebuild index"
  - "update loi"
  - "refresh loi"
  - "refresh index"
  - "incremental update"
  - "full codebase index"
  - "implement loi changes"
  - "sync intent to code"
---

## Mode Dispatch

| Trigger / Keyword | Mode |
|-------------------|------|
| `/loi-generate`, `/loi generate`, "generate loi", "full loi", "full codebase index", "rebuild index" | **Full-Generate** |
| `/loi update`, "update loi", "refresh loi", "refresh index", "incremental update" | **Incremental-Generate** |
| `/loi implement`, "implement loi changes", "sync intent to code" | **Implement** |
| `/loi validate` | **Validate** |

"Rebuild" without further context means **Full-Generate**.

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
- **Never create a building with only one room.** If a subdomain folder would contain only `_root.md` + one `.md` room file, the folder adds navigation overhead with no benefit. Instead, promote the room to a flat top-level file (e.g., `docs/index/config.md` rather than `docs/index/config/_root.md` + `docs/index/config/general.md`). A building needs at least 2 room files to justify its own folder.
- **Shared utilities go in `shared/` or `infra/`, not in their primary consumer's room.** If `readlimit/` is used by mirrors, scanning, and API handlers, it belongs in `infra/` or `shared/` — not in `proxy/`. Place a file where its *responsibility* lives, not where its biggest caller lives.

**4.5. Scaffold mechanical fields** — If the project has a codetect index (`.codetect/symbols.db`), run the scaffold script to pre-populate SYMBOLS, TYPE, and DEPENDS from AST data:

```bash
python3 <skill-base-dir>/../loi/scripts/generate_loi.py <project-root> --scaffold
```

This produces room files with `<!-- LLM-FILL -->` markers where the LLM must fill in DOES, PATTERNS, USE WHEN, EMITS, and CONSUMERS. If no codetect index exists, skip this step and generate entries manually in step 5.

**5. Generate (via RLM Skill with Consensus Loop)** — Use RLM's parallel processing with a 3-step pipeline: Map -> Critique -> Reduce.

**IMPORTANT — Scaffolded rooms:** If step 4.5 produced room files with `<!-- LLM-FILL -->` markers, the Map phase MUST preserve existing SYMBOLS, DEPENDS, and TYPE fields and ONLY fill in the markers. Do NOT regenerate or rewrite fields that the scaffold already populated. Read each room file first and replace only the `<!-- LLM-FILL: ... -->` comments with the appropriate content.

- **Phase A (Map):** Spawn one worker agent per room. Prompt: "Read all files in [file list]. If the room file already exists with `<!-- LLM-FILL -->` markers, preserve all existing fields (SYMBOLS, DEPENDS, TYPE) and only fill in the marked fields. Otherwise, produce a LOI entry for each file following `reference/FORMAT_REFERENCE.md`.

  **CRITICAL — use this exact entry format (no bullets, no bold, no markdown formatting on field names):**

  ```
  # filename.go

  DOES: One or two sentences describing what this file does specifically.
  SYMBOLS:
  - ExportedFunc(ctx context.Context, id string) (Result, error)
  - unexportedFunc(input Input) error
  - Types: StructA, StructB (+4 more)
  TYPE: MainStruct { field1, field2, field3 }
  DEPENDS: internal/audit, internal/config (+2 more)
  PATTERNS: middleware-chain, token-bucket
  USE WHEN: Choosing between this and alternative.go for X.
  ```

  **Format rules — violations will be rejected by the Committee:**
  - Fields are plain `FIELDNAME:` — never `- **FIELDNAME**:` or `**FIELDNAME**:`
  - SYMBOLS are list items (`- `) with full signatures including params and return types — never comma-separated names on one line
  - Entries separated by `---`
  - Entries alphabetized by filename within each room
  - No `PATH:` field — the `# filename.ext` heading is the path

  **Content rules:**
  - **DEPENDS**: Trace `import`/`require` statements for cross-package internal imports. Omit stdlib and same-package. Cap at 4 items.
  - **EMITS**: If the file publishes events, emits to channels, calls webhook delivery, or invokes callback functions that notify other subsystems, add EMITS.
  - **PROPS/HOOKS**: For React/Vue components, extract component props into PROPS and custom hooks/composables into HOOKS.
  - **PATTERNS**: Name only, no parenthetical explanations.
  - **SYMBOLS**: Functions (exported and unexported) with full signatures. Group types — never list each one. Use shortest same-repo paths for DEPENDS; keep full paths for external repos.
  - **TYPE**: Only for actual struct/class definitions, not file categories.
  - **DOES**: 1-2 sentences max."
- **Phase B (Critique / The Committee):** Pass the mapped drafts to specialized personas before finalizing:
  - *Architect Persona:* "Does this room mix concerns? (e.g., HTTP parsing next to database logic). If yes, set `architectural_health: warning` or `critical` and write a `committee_notes` explanation."
  - *Security Persona:* "Does this room handle raw SQL, PII, or auth tokens? If yes, set `security_tier: sensitive` or `high`."
  - *Completeness Persona:* "For each entry: (1) Are DEPENDS fields present for all cross-domain imports? (2) Are EMITS fields present for event/callback publishers? (3) For frontend components, are PROPS and HOOKS populated? (4) Are all behavioral PATTERNS named? Flag omissions."
  - *Brevity Persona:* "For each entry: (1) Are fields plain `FIELDNAME:` format — not `- **FIELDNAME**:` bullets? (2) Are SYMBOLS list items (`- `) with full signatures — not comma-separated names on one line? (3) Are entries separated by `---` and alphabetized by filename? (4) Is there a `PATH:` field? (reject — heading is the path). (5) Do SYMBOLS group types instead of listing individually? (6) Are same-repo DEPENDS using shortest paths? Max 4 items. (7) Are PATTERNS name-only? (8) Is DOES 1-2 sentences max? Flag violations."
- **Phase C (Reduce):** "Merge the critiqued LOI outputs into the subdomain structure. At the top of each room file, generate the YAML metadata header including `see_also`, `hot_paths`, and the exact Governance Flags determined in Phase B. All rooms MUST have `architectural_health` and `security_tier` fields — including test and utility rooms (use `normal` when no issues are flagged). Build `docs/index/_root.md` with task/pattern mapping tables."
- **Agent count**: spawn one worker agent per room, plus the Committee personas for the Critique phase. If `/rlm` is unavailable, fall back to spawning one background Agent per subdomain to simulate the Committee.

**5.5. Cross-room symbol collision pass** — After Phase C produces all room content but before writing any files, scan for symbol name collisions across rooms and annotate each conflicting entry.

**How:**
1. For each room, extract the bare function/method name from every SYMBOLS line by stripping receiver, params, and return types. Examples: `(p *Persist) CreateMessageLog(ctx context.Context, input CreateMessageLogInput) (MessageLog, error)` → `CreateMessageLog`; `CreateMessageLog(ctx context.Context, p Persister, input persist.CreateMessageLogInput) (persist.MessageLog, error)` → `CreateMessageLog`.
2. Build an inverted map: `bare_name → [(room_file, full_signature, source_file_heading)]`.
3. For every bare name that maps to 2+ rooms: append a `DISAMBIGUATION:` line to the relevant entry in **each** room. The note must:
   - Name the other source file and room
   - State the key behavioral difference in one sentence
   - Give a concrete routing hint for when to choose the other room

**Example:**
```
DISAMBIGUATION: There is also a package-level `CreateMessageLog` in `internal/temporal/shared/activities.go` (`workers/temporal_core.md`) that wraps this DB method and classifies errors for Temporal retry (non-1062 → retryable wrapped error; 1062 → non-retryable). If the question is about error behaviour, retryability, or Temporal, load `workers/temporal_core.md` instead.
```

In **Incremental-Generate**, run this pass only for regenerated rooms plus any rooms whose SYMBOLS now collide with them.

**6. Write index files:**
- `docs/index/_root.md` — Campus map (see format below)
- `docs/index/<subdomain>/_root.md` — Building router per subdomain
- `docs/index/<subdomain>/<room>.md` — Domain files with flat entry lists
- **If scaffolded rooms exist:** Update them in-place (fill markers, update metadata header). Do NOT overwrite the entire file — preserve scaffold-generated SYMBOLS, DEPENDS, and TYPE.

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

## GOVERNANCE WATCHLIST

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

**When to use:** After code changes, user says "update loi", "refresh loi", or "refresh index", or during iterative development.

**Process:**

1. **Detect stale rooms** — For each domain file across all subdomains:
   ```bash
   git diff --name-only $(git log -1 --format=%H -- docs/index/<subdomain>/<room>.md) HEAD -- <source-paths>
   ```
   Mark rooms where source files changed.

2. **Regenerate only stale rooms** — If the project has a codetect index, run `generate_loi.py <project-root> --scaffold --room <room>` for each stale room to refresh SYMBOLS/DEPENDS from AST. Then invoke `/rlm` (via the Skill tool with `skill: "rlm"`) with only the stale rooms as map inputs to fill semantic fields; if `/rlm` is unavailable, spawn agents manually per room.

3. **Merge results** — Update only affected domain files; leave others unchanged.

4. **Update routers** — Refresh `_root.md` entries (both campus and building level) for any changed rooms.

---

## Implement Mode (Bi-Directional Sync)

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

  Notifications use the pluggable `--notify-backend` flag. Legacy `--slack-webhook` / `--slack-channel` flags still work (deprecated). Changes within the batch window (`--debounce`, default 5s) are grouped into a single PR. In `auto` mode, a `--policy` flag gates what the worker is allowed to implement.

  ```bash
  # Notify mode — Slack webhook (preferred)
  uv run --with watchdog watcher.py --notify-backend slack --notify-url https://hooks.slack.com/services/...

  # Notify mode — write events to a JSONL file
  uv run --with watchdog watcher.py --notify-backend file --notify-file loi-events.jsonl

  # Notify mode — POST to a custom HTTP endpoint
  uv run --with watchdog watcher.py --notify-backend webhook --notify-url https://example.com/loi-hook

  # Auto mode — create PR but never invoke implement worker (safest auto option)
  uv run --with watchdog watcher.py --mode auto --policy draft-only --notify-backend slack --notify-url https://...

  # Auto mode — implement only test files; block on sensitive rooms
  uv run --with watchdog watcher.py --mode auto --policy tests-safe --notify-backend webhook --notify-url https://...

  # Auto mode — implement within explicit scopes only
  uv run --with watchdog watcher.py --mode auto --policy scoped-code-safe \
    --allowed-scopes "docs/**,tests/**" --notify-backend slack --notify-url https://...

  # Auto mode — full autonomy, disable governance blocking
  uv run --with watchdog watcher.py --mode auto --policy full-auto \
    --block-governance-security none --notify-backend slack --notify-url https://...
  ```

  **Policy tiers** (controls what `--mode auto` is allowed to implement):

  | Policy | Behaviour |
  |--------|-----------|
  | `notify-only` | Validate + notify; worker never invoked |
  | `draft-only` | Branch + draft PR created; worker not invoked |
  | `docs-safe` | Implement only if all source files are under `docs/` |
  | `tests-safe` | Implement only if all source files are test files |
  | `scoped-code-safe` | Implement only within `--allowed-scopes` globs |
  | `full-auto` | No scope restriction (default) |

  Regardless of policy, auto-implement is always blocked if a changed room has `security_tier` matching `--block-governance-security` (default: `sensitive`) or `architectural_health: critical`. A conflicting room claim from another agent also blocks the worker.

See the README for setup instructions.

---

## Validate Mode

Verify the structural integrity and coverage of a generated LOI index.

**When to use:** After generating or updating an index, or when `/loi validate` is triggered.

**Process:**

Run the validation script using the skill's base directory (provided at the top of the skill invocation as `Base directory for this skill:`):

```bash
python3 <skill-base-dir>/../loi/scripts/validate_loi.py <project-root>
```

The script checks:
- `docs/index/_root.md` exists and has TASK → LOAD table
- All subdomain `_root.md` routers exist and reference valid room files
- All room `.md` files have YAML frontmatter with required fields (`room`, `see_also`)
- Every room file referenced in a router actually exists on disk
- Every source directory with code files is covered by at least one room
- No room exceeds ~150 entries

**Changed-rooms mode** — validate only rooms touched in the current git diff (useful during development):

```bash
python3 <skill-base-dir>/../loi/scripts/validate_loi.py <project-root> --changed-rooms
```

**CI mode** — treat warnings as errors (exits 1 if any warnings exist):

```bash
python3 <skill-base-dir>/../loi/scripts/validate_loi.py <project-root> --ci
```

Both flags can be combined. The pre-push hook runs `--changed-rooms` automatically before every push.

**Install the pre-push hook** with `setup_hook.py`:

```bash
python3 <skill-base-dir>/../loi/scripts/setup_hook.py <project-root>
python3 <skill-base-dir>/../loi/scripts/setup_hook.py <project-root> --force   # overwrite existing
```

Fix any reported errors before considering the index complete.

---

## Pattern Validation

Verify that every PATTERN entry in `_root.md` files is semantically grounded in the room it points to.

```bash
python3 <skill-base-dir>/../loi/scripts/validate_patterns.py <project-root>
python3 <skill-base-dir>/../loi/scripts/validate_patterns.py <project-root> --level 2
```

**Levels:**
- `1` (default) — Pattern label (normalized) must appear in the target room body.
- `2` — Also checks `pattern_aliases` frontmatter and `pattern_metadata` freshness (`last_validated` within 14 days).

**Output categories:**
- `ERRORS` — Target room file missing entirely.
- `ORPHANED PATTERNS` — Label not found in room body and no aliases match.
- `WARNINGS` — Alias-only support, or `last_validated` is stale (>14 days).

---

## Table Diff

Compute row-level deltas for TASK, PATTERN, and GOVERNANCE tables between two git revisions.

```bash
python3 <skill-base-dir>/../loi/scripts/diff_tables.py <project-root> docs/index/auth/_root.md
python3 <skill-base-dir>/../loi/scripts/diff_tables.py <project-root> docs/index/_root.md --from HEAD~3 --to HEAD
```

The watcher daemon calls this automatically and attaches the table diff to Slack/webhook notifications when a `_root.md` file changes.

---

## Governance Aggregation

Aggregate GOVERNANCE WATCHLIST entries from all `_root.md` files, sorted by combined severity (security + health).

```bash
python3 <skill-base-dir>/../loi/scripts/governance.py <project-root>
python3 <skill-base-dir>/../loi/scripts/governance.py <project-root> --security sensitive
python3 <skill-base-dir>/../loi/scripts/governance.py <project-root> --health warning
python3 <skill-base-dir>/../loi/scripts/governance.py <project-root> --format json

# Multi-repo fleet view
python3 <skill-base-dir>/../loi/scripts/governance.py /repos/alpha /repos/beta /repos/gamma
```

Also surfaces rooms with non-normal `architectural_health` or `security_tier` frontmatter flags that aren't already covered by a watchlist entry.

---

## Runtime Coordination (Room Claims)

Advisory-first coordination so multiple agents avoid editing the same room simultaneously. Claims are stored in `.loi-claims.json` at the project root (gitignored).

```bash
# Claim a room before editing it
python3 <skill-base-dir>/../loi/scripts/runtime.py claim auth/ucan.md --intent edit --ttl 15m

# Extend your claim while working
python3 <skill-base-dir>/../loi/scripts/runtime.py heartbeat auth/ucan.md

# Release when done
python3 <skill-base-dir>/../loi/scripts/runtime.py release auth/ucan.md

# Check who holds a claim
python3 <skill-base-dir>/../loi/scripts/runtime.py status auth/ucan.md --include-freshness

# Record a handoff summary
python3 <skill-base-dir>/../loi/scripts/runtime.py summary auth/ucan.md "Working on TTL path in MintToken"

# List all active claims
python3 <skill-base-dir>/../loi/scripts/runtime.py claims
python3 <skill-base-dir>/../loi/scripts/runtime.py claims --repo my-repo
```

**Intent options:** `read` | `edit` | `review` | `security-sweep`

**Conflict matrix:** edit + edit = conflict (blocked); security-sweep + edit = governance-sensitive (warning). All other combinations allow with optional visibility notice.

---

## Proposal Provenance

Query and validate AI-generated improvement proposals stored under `docs/index/proposals/`.

```bash
# List all proposals
python3 <skill-base-dir>/../loi/scripts/proposals.py <project-root>

# Filter by room, grader version, or failure reason
python3 <skill-base-dir>/../loi/scripts/proposals.py <project-root> --target-room auth/ucan.md
python3 <skill-base-dir>/../loi/scripts/proposals.py <project-root> --grader-version v2.3
python3 <skill-base-dir>/../loi/scripts/proposals.py <project-root> --failure-reason REFUSAL_CONTEXT

# Validate all proposals for required metadata fields
python3 <skill-base-dir>/../loi/scripts/proposals.py <project-root> --validate
```

Each proposal file must contain a `proposal_metadata:` block. See `reference/FORMAT_REFERENCE.md` → Schema Extensions for the full field reference.
