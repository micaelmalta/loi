---
room: LOI Automation Tooling & CI/CD
see_also: ["skill.md"]
hot_paths: "If adding a notify backend -> update backends/__init__.py load_backend() + tests. If changing _check_policy() -> update INTENT_CONFLICT_MATRIX in runtime.py. If changing validate_loi.py schema -> check --changed-rooms and --ci modes. If changing intent field names -> update extract_changed_entries() in watcher.py + pre-commit-loi.sh grep."
security_tier: "sensitive"
architectural_health: "warning"
committee_notes: "Fixed: pre-commit-loi.sh variables fully quoted; watcher.py validates --worker-cmd via shutil.which() at startup; loi-committee.yml file paths validated against GITHUB_WORKSPACE before readFileSync. Remaining: --worker-cmd is operator-controlled and intentionally unrestricted beyond existence check."
---

# .github/workflows/loi-committee.yml
DOES: GitHub Actions CI — runs Architect and Security personas as a matrix job on every PR with source changes; supports three providers via workflow_dispatch input or LOI_AI_PROVIDER repo variable: Gemini (google-github-actions/run-gemini-cli@v0.1.21 official action), Anthropic (direct API), OpenAI (direct API); builds persona prompt with file contents, posts idempotent review comments with critical-flag badges; uses FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true; gate job fails if personas fail
CONFIG: GEMINI_API_KEY, ANTHROPIC_API_KEY, OPENAI_API_KEY, LOI_AI_PROVIDER (repo var), LOI_AI_MODEL (repo var), LOI_FAIL_ON_CRITICAL
PATTERNS: ci-matrix, idempotent-comment, provider-dispatch

# .github/workflows/tests.yml
DOES: GitHub Actions CI — runs pytest on Python 3.11 and 3.12 matrix when scripts/ or tests/ change on push/PR; installs only pytest; uses FORCE_JAVASCRIPT_ACTIONS_TO_NODE24=true

# pre-commit-loi.sh
DOES: Level 7 pre-commit hook (Option B) — intercepts git commits staging docs/index/*.md files; checks if intent fields (DOES, SYMBOLS, TYPE, INTERFACE, PATTERNS) changed in cached diff; invokes AI worker with diff context; auto-stages modified source files into the commit; skippable via LOI_SKIP=1
CONFIG: LOI_WORKER_CMD (default: claude), LOI_INDEX_PATH, LOI_AUTO_STAGE, LOI_SKIP

# hooks/pre-push.sample
DOES: Pre-push hook template — detects changed docs/index/ files in push range via git diff, runs validate_loi.py --changed-rooms, blocks push (exit 1) on errors; searches for validate script in project root, ~/.claude/skills/loi/, and relative paths; install via setup_hook.py

# validate_loi.py
DOES: Validates LOI index structural integrity — checks campus _root.md, building routers, room frontmatter (room, see_also required), cross-references, file/glob references in TASK rows, source directory coverage, room size limits; --changed-rooms mode restricts to uncommitted index changes (git diff HEAD); --ci mode treats warnings as errors; exits 0/1
SYMBOLS:
- validate(project_root: Path, changed_rooms_only: bool = False) → ValidationResult
- get_changed_index_files(project_root: Path) → list[Path]
- check_file_refs(room_file: Path, project_root: Path, result: ValidationResult) → None
- find_source_dirs(root: Path, excluded_dirs: set[str]) → set[str]
USE WHEN: After generating or updating the LOI index; in CI (--ci flag); in pre-push hook (--changed-rooms flag)

# validate_patterns.py
DOES: Semantic validator for PATTERN → LOAD table entries — Level 1: normalized label must appear in target room body; Level 2: also checks pattern_aliases frontmatter and pattern_metadata.last_validated freshness (warns if > 14 days); deduplicates _root.md files to avoid double-reporting; reports errors (missing target), orphans (no semantic support), warnings (alias-only, stale)
SYMBOLS:
- validate_patterns(project_root: Path, level: int = 1) → PatternValidationResult
- extract_pattern_rows(root_file: Path) → list[dict]
- parse_pattern_metadata_block(room_file: Path) → dict[str, dict]
- normalize(text: str) → str
USE WHEN: Verifying PATTERN table rows are semantically grounded; catching fabricated or stale pattern labels

# diff_tables.py
DOES: Row-level semantic diff over TASK, PATTERN, and GOVERNANCE markdown tables between two file versions; supports git ref-to-ref comparison and HEAD-vs-working-tree mode (used by watcher); warns on duplicate first-column keys; called by watcher to attach structured change summaries to notifications
SYMBOLS:
- diff_tables(old_text: str, new_text: str) → dict[str, dict]
- diff_file_between_refs(project_root: Path, filepath: str, from_ref: str, to_ref: str) → str | None
- diff_file_against_head(project_root: Path, filepath: str) → str | None
- parse_tables(text: str) → dict[str, list[tuple]]
- format_diff(diff: dict) → str

# governance.py
DOES: Aggregates GOVERNANCE WATCHLIST entries from all _root.md files across one or more repos, sorted by combined health+security severity; surfaces rooms with non-normal frontmatter flags not already covered by a watchlist entry (deduplication via endswith path matching); supports --security / --health filters and --format json
SYMBOLS:
- aggregate_governance(project_roots: list[Path]) → list[dict]
- parse_governance_table(text: str) → list[dict]
- parse_room_frontmatter_flags(room_file: Path) → dict
USE WHEN: Getting cross-repo governance risk view; finding all sensitive/warning rooms without opening every _root.md

# runtime.py
DOES: Advisory room claim coordination — stores claims in .loi-claims.json with exclusive (_exclusive) and shared (_shared) fcntl locking to prevent concurrent write corruption; INTENT_CONFLICT_MATRIX defines per-intent-pair actions (edit+edit=conflict, security-sweep+edit=governance_sensitive); TTL with heartbeat extension; handoff summaries; --include-freshness on status
SYMBOLS:
- ClaimsStore(project_root: Path)
- ClaimsStore.add_claim(claim: dict) → None
- ClaimsStore.remove_claim(scope_id: str, agent_id: str) → bool
- ClaimsStore.update_expiry(scope_id: str, agent_id: str, extra_seconds: int) → bool
- ClaimsStore.add_summary(scope_id: str, agent_id: str, summary: str) → None
- check_conflict(existing_claims: list[dict], incoming_intent: str) → tuple[str, str]
PATTERNS: file-locking, advisory-coordination

# proposals.py
DOES: Queries and validates AI-generated proposal provenance metadata — discovers files under docs/index/proposals/ and any *proposal* named files; parses proposal_metadata YAML blocks; list command filters by --target-room, --grader-version, --failure-reason; --validate checks required fields (proposal_id, generated_at, source_run_id, target_room) and warns on missing optional fields
SYMBOLS:
- parse_proposal_metadata(filepath: Path) → dict | None
- find_proposal_files(project_root: Path) → list[Path]
USE WHEN: Auditing eval-generated proposals; querying proposals by grader version, failure reason, or target room

# setup_hook.py
DOES: Installs LOI git hooks into a target repo — copies hooks/<mode>.sample to .git/hooks/<mode>, makes it executable; also writes .loi-claims.json and .loi-claims.json.lock to .gitignore if absent; --force to overwrite; --mode pre-push (only supported mode)
SYMBOLS:
- install_hook(project_root: Path, mode: str, force: bool) → int
- _ensure_gitignore(project_root: Path) → None

# backends/__init__.py
DOES: Defines NotifyBackend Protocol (send(event_type, payload)) and load_backend() factory — dispatches to stdout, file, webhook, or slack backend based on config dict
SYMBOLS:
- NotifyBackend.send(event_type: str, payload: dict) → None
- load_backend(config: dict) → NotifyBackend
PATTERNS: strategy-pattern, factory

# backends/stdout.py
DOES: Stdout notify backend — prints event as a JSON line to stdout; default when no notify-url is configured

# backends/file.py
DOES: File notify backend — appends each LOI event as a JSONL record to a configurable file path; auto-creates parent directories

# backends/webhook.py
DOES: Webhook notify backend — POSTs LOI events as JSON to any HTTP endpoint; optional Bearer token from env var via token_env; propagates HTTP errors to caller (single error boundary in watcher.py)
CONFIG: LOI_NOTIFY_TOKEN (via token_env param)

# backends/slack.py
DOES: Slack notify backend — sends LOI events as Block Kit messages via incoming webhook URL; includes repo, event type, governance flags, table diff (truncated at 2000 chars with notice), and PR review button

# watcher.py
DOES: Level 7 background daemon — monitors docs/index/ for markdown saves, batches within --debounce window (default 5s), dispatches to pluggable notify backends (--notify-backend stdout/file/webhook/slack) or AI implement worker; in auto mode applies --policy tiers (notify-only/draft-only/docs-safe/tests-safe/scoped-code-safe/full-auto) with governance-aware blocking (--block-governance-security, default: sensitive) and room-claim conflict check; attaches TASK/PATTERN/GOVERNANCE table diff and worst governance flags to notifications
SYMBOLS:
- LOIHandler(project_root, mode, debounce, worker_cmd, notify_backend, policy, allowed_scopes, block_governance_security)
- LOIHandler._check_policy(entries, governance, changed_room_files) → tuple[bool, str]
- LOIHandler._extract_batch_governance(changed_room_files) → dict
- compute_table_diff(project_root: Path, filepath: str) → str | None
- create_draft_pr(project_root, changed_files, all_entries) → str | None
CONFIG: --watch-path, --mode (notify/auto/dry-run), --notify-backend, --notify-url, --notify-file, --notify-token-env, --debounce, --policy, --allowed-scopes, --block-governance-security, --worker-cmd
PATTERNS: file-watcher, debounce, strategy-pattern, policy-tiers
DEPENDS: backends/__init__.py, diff_tables.py, validate_loi.py, runtime.py

# check_stale.py
DOES: Pre-commit stale index detector — finds staged source files covered by LOI Source paths: fields that have no corresponding staged index file; blocks by default (LOI_STALE_BLOCK=0 to warn only); git commit --no-verify to skip; resolves coverage via extract_source_paths() scanning all docs/index/ .md files
SYMBOLS:
- main() → int
- find_covering_rooms(project_root: Path, source_file: str) → list[Path]
- extract_source_paths(room_file: Path) → list[str]
- get_staged_files(project_root: Path) → list[str]
USE WHEN: Detecting that source code changed without a corresponding LOI index update

# hooks/pre-commit-stale.sample
DOES: Pre-commit hook template — locates check_stale.py (repo-local or ~/.claude/skills/loi/), executes it against the repo root; skips silently if script not found; install via setup_hook.py --mode pre-commit-stale

# tests/conftest.py
DOES: Adds scripts/ directory to sys.path for all pytest sessions

# tests/test_diff_tables.py + test_governance.py + test_runtime.py + test_validate_patterns.py + test_setup_hook.py + test_check_stale.py
DOES: Pytest test suite — 99 tests covering diff_tables, governance, runtime (concurrent locking), validate_patterns, setup_hook, check_stale (coverage detection, LOI_STALE_BLOCK blocking, LOI_SKIP bypass)
