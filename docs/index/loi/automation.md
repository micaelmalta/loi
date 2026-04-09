---
room: LOI Automation Tooling & CI/CD
see_also: ["skill.md"]
hot_paths: "If changing intent field names (DOES, SYMBOLS, etc.) -> update extract_changed_entries() in watcher.py and the grep pattern in pre-commit-loi.sh. If changing index structure -> update validate_loi.py find_source_dirs() and extract_source_paths_from_rooms()."
security_tier: "sensitive"
architectural_health: "healthy"
committee_notes: "pre-commit-loi.sh passes raw git diff content via shell variable into a heredoc — potential injection if docs/index/ content is attacker-controlled. watcher.py passes user-supplied --worker-cmd to subprocess.run() without sanitization. Both are acceptable risks for local tooling but worth noting."
---

# loi-committee.yml
DOES: GitHub Actions CI pipeline that runs Architect and Security personas as a matrix job on every PR with source file changes; reads changed files (up to 200), loads LOI _root.md context, posts per-persona review comments (create or update idempotently); gates PR merge via committee-gate job when LOI_FAIL_ON_CRITICAL=true and ANTHROPIC_API_KEY is connected
CONFIG: ANTHROPIC_API_KEY, LOI_FAIL_ON_CRITICAL
PATTERNS: ci-matrix, idempotent-comment

# pre-commit-loi.sh
DOES: Level 7 pre-commit hook (Option B) — intercepts git commits that stage docs/index/*.md files; checks if any intent fields (DOES, SYMBOLS, TYPE, INTERFACE, PATTERNS) changed in the cached diff; invokes AI worker with diff context and list of changed files; auto-stages any source files modified by the worker into the in-progress commit; skippable via LOI_SKIP=1
CONFIG: LOI_WORKER_CMD (default: claude), LOI_INDEX_PATH (default: docs/index), LOI_AUTO_STAGE (default: true), LOI_SKIP

# validate_loi.py
DOES: Validates LOI index structural integrity — checks campus _root.md exists and has TASK→LOAD table, building routers exist and reference valid room files, room files have YAML frontmatter with required fields (room, see_also), all markdown cross-references resolve to existing files, every source directory with code files is covered by at least one room, no room exceeds 150 entries; prints summary count of total rooms validated and total entries found across all rooms; exits 0 on success, 1 on errors
SYMBOLS:
- validate(project_root: Path) → ValidationResult
- find_source_dirs(root: Path) → set[str]
- parse_frontmatter(filepath: Path) → dict[str, str] | None
- count_entries(filepath: Path) → int
- extract_md_links(filepath: Path) → list[str]
- extract_source_paths_from_rooms(index_dir: Path) → set[str]
USE WHEN: After generating or updating the LOI index to verify integrity before declaring it complete

# watcher.py
DOES: Level 7 background daemon (Option C) — monitors docs/index/ with watchdog for markdown file saves; diffs modified files via git diff HEAD to detect uncommitted changes; parses diff for changed intent fields (DOES, SYMBOLS, TYPE, INTERFACE, PATTERNS) using regex; constructs prompt for AI worker with filepath, diff, and affected source files; invokes worker command (default: claude) with debounce to avoid duplicate triggers; supports --dry-run mode
SYMBOLS:
- LOIHandler(debounce: float, worker_cmd: str, dry_run: bool)
- LOIHandler.on_modified(event: FileModifiedEvent) → None
- get_intent_diff(filepath: str) → str | None
- extract_changed_entries(diff: str) → list[dict]
- build_worker_prompt(filepath: str, diff: str, entries: list[dict]) → str
- find_project_root() → Path
CONFIG: --watch-path (default: docs/index), --debounce (default: 2.0s), --worker-cmd (default: claude), --dry-run
PATTERNS: file-watcher, debounce
