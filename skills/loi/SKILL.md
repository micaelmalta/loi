---
name: loi
description: >
  Codebase "Library of Intent" for LLM navigation. Replaces probabilistic global searching with a deterministic, plain-English navigation hierarchy. Uses Recursive Language Model (RLM) logic to scale indefinitely by nesting indices (Campus → Building → Room). Eliminates file searching by providing a structured, machine-readable index of every file, symbol, and dependency chain.
triggers:
  - /loi
  - /loi generate
  - /loi update
  - "generate loi"
  - "full loi"
  - "rebuild index"
  - "update loi"
  - "where is"
  - "find where"
  - "navigate codebase"
  - "full codebase index"
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

**1. Discover ALL source directories** — Full census before scoping anything:
```bash
find . -maxdepth 1 -type d | sort
find . -type f \( -name "*.rb" -o -name "*.ts" -o -name "*.go" -o -name "*.py" \) \
  ! -path "*/node_modules/*" ! -path "*/.git/*" ! -path "*/vendor/*" \
  | sed 's|/[^/]*$||' | sort | uniq -c | sort -rn | head -40
```
**Every top-level directory with source files MUST be included.** Non-standard directories (`packages/`, `engines/`, `gems/`) often contain more code than `app/` or `src/`. Omitting them produces a partial index.

**2. Filter source files** — Use Glob tool by language:
- `**/*.go`, `**/*.ts`, `**/*.tsx`, `**/*.py`, `**/*.js`, `**/*.rb` (adapt per project)
- Exclude: `vendor/`, `node_modules/`, `__pycache__/`, `.git/`, generated files

**3. Group into Subdomains and Domains** — Organize by **responsibility**, not language or directory. The hierarchy is:

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

**4. Generate via RLM** 
— Invoke `/rlm` (via the Skill tool with `skill: "rlm"`) to parallelize:
- **Map prompt per domain:** "Read all files in [file list]. For each file, produce a LOI entry following `reference/FORMAT_REFERENCE.md`. 
- **At the top of the output, generate a YAML metadata header including `room`, `see_also` (predict related rooms), and `hot_paths` (common edit sequences).** Return one markdown code block."
- **Reduce prompt**: "Merge these domain LOI outputs into the subdomain structure and build `docs/index/_root.md` with a task→load mapping table AND a pattern→load table for cross-cutting behavioral patterns (retry, backoff, caching, transactions, etc.)."
- If `/rlm` is unavailable, fall back to spawning one background Agent per subdomain

**5. Write index files:**
- `docs/index/_root.md` — Campus map (see format below)
- `docs/index/<subdomain>/_root.md` — Building router per subdomain
- `docs/index/<subdomain>/<room>.md` — Domain files with flat entry lists

**6. Verify coverage** — Check that every source file from step 1 appears in at least one domain file. List any uncovered directories explicitly.

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
see_also: ["../<subdomain>/<related_room>.md"] # Conceptually linked rooms to pre-fetch
hot_paths: "If editing X -> remember to update Y"
---
```


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
