#!/usr/bin/env -S python3 -u
"""LOI Level 7 — Background daemon that watches docs/index/ for markdown changes
and triggers validation, draft PRs, notifications, or AI implementation.

Usage:
    uv run --with watchdog watcher.py [options]
    uv run --with watchdog watcher.py --mode notify --notify-backend webhook --notify-url http://peer-broker.local/loi/events
    uv run --with watchdog watcher.py --mode notify --notify-backend slack --notify-url https://hooks.slack.com/...
    uv run --with watchdog watcher.py --mode auto --watch-path /path/to/project
    uv run --with watchdog watcher.py --mode dry-run

Modes:
    notify    (default) Validate → create draft PR → notification. No code changes.
    auto      Validate → implement via AI worker → commit → PR. Full autonomy.
    dry-run   Log what would happen without taking any action.

Notify backends:
    stdout   Print events to stdout (default)
    file     Append JSON events to a file (--notify-file)
    webhook  POST JSON to an HTTP endpoint (--notify-url)
    slack    Post to Slack incoming webhook (--notify-url)

Requires: uv (https://docs.astral.sh/uv/)
"""

import argparse
import fnmatch
import json
import re
import shutil
import subprocess
import sys
import threading
import time
from datetime import datetime, timezone
from pathlib import Path

try:
    from watchdog.observers import Observer
    from watchdog.events import FileSystemEventHandler, FileModifiedEvent
except ImportError:
    print("Error: watchdog not installed. Run with: uv run --with watchdog watcher.py", file=sys.stderr)
    sys.exit(1)

# Make sibling scripts importable — set once at module load, not inside each call
_SCRIPTS_DIR = str(Path(__file__).resolve().parent)
if _SCRIPTS_DIR not in sys.path:
    sys.path.insert(0, _SCRIPTS_DIR)

# ---------------------------------------------------------------------------
# Git / path helpers
# ---------------------------------------------------------------------------

def find_git_root(start: Path) -> Path:
    """Walk up from start to find the git root."""
    p = start.resolve()
    while p != p.parent:
        if (p / ".git").exists():
            return p
        p = p.parent
    return start


def resolve_watch_dir(watch_path: str) -> tuple[Path, Path]:
    """Resolve watch_path to (project_root, watch_dir)."""
    p = Path(watch_path).resolve()

    if (p / "docs" / "index").is_dir():
        return find_git_root(p), p / "docs" / "index"

    if p.is_dir():
        return find_git_root(p), p

    cwd = Path.cwd()
    rel = cwd / watch_path
    if (rel / "docs" / "index").is_dir():
        return find_git_root(rel), rel / "docs" / "index"
    if rel.is_dir():
        return find_git_root(rel), rel

    return find_git_root(cwd), rel


def get_repo_name(project_root: Path) -> str:
    """Get the repository name from git remote or folder name."""
    result = subprocess.run(
        ["git", "remote", "get-url", "origin"],
        capture_output=True, text=True, cwd=project_root,
    )
    if result.returncode == 0:
        url = result.stdout.strip()
        name = url.rstrip("/").rsplit("/", 1)[-1].removesuffix(".git")
        owner = url.rstrip("/").rsplit("/", 2)[-2].rsplit("/", 1)[-1].removesuffix(":")
        return f"{owner}/{name}"
    return project_root.name


# ---------------------------------------------------------------------------
# Diff parsing
# ---------------------------------------------------------------------------

def get_intent_diff(filepath: str, project_root: Path) -> str | None:
    """Run git diff on the file and return the diff output, or None if clean."""
    result = subprocess.run(
        ["git", "diff", "HEAD", "--", filepath],
        capture_output=True, text=True, cwd=project_root,
    )
    diff = result.stdout.strip()
    return diff if diff else None


def extract_changed_entries(diff: str) -> list[dict]:
    """Parse a git diff to find changed intent fields in heading and table formats."""
    entries = []
    current_file = None
    intent_fields = ("DOES:", "SYMBOLS:", "TYPE:", "INTERFACE:", "PATTERNS:")

    for line in diff.splitlines():
        if not line.startswith("+") or line.startswith("+++"):
            heading_match = re.match(r"[+ ]# (\S+\.\w+)", line)
            if heading_match:
                current_file = heading_match.group(1)
            continue

        added = line[1:]

        for field in intent_fields:
            if field in added and current_file:
                entries.append({
                    "source_file": current_file,
                    "changed_line": added.strip(),
                })
                break
        else:
            if added.strip().startswith("|") and "|" in added:
                cells = [c.strip() for c in added.split("|")]
                cells = [c for c in cells if c]
                if len(cells) >= 2:
                    filepath_cell = cells[0]
                    content = " | ".join(cells[1:])
                    if re.match(r".*\.\w+$", filepath_cell) and filepath_cell != "FILE":
                        entries.append({
                            "source_file": filepath_cell,
                            "changed_line": content[:120],
                        })

    return entries


# ---------------------------------------------------------------------------
# Table diff (Gap 8)
# ---------------------------------------------------------------------------

def compute_table_diff(project_root: Path, filepath: str) -> str | None:
    """Compute a semantic diff over TASK / PATTERN / GOVERNANCE table rows.

    Returns a human-readable summary string, or None if no table changes.
    """
    try:
        from diff_tables import diff_file_against_head
        return diff_file_against_head(project_root, filepath)
    except Exception as exc:
        print(f"[LOI Watcher] table-diff unavailable: {exc}")
        return None


# ---------------------------------------------------------------------------
# Governance helper
# ---------------------------------------------------------------------------

def extract_governance_flags(project_root: Path, filepath: str) -> dict:
    """Extract health/security governance flags from a room file."""
    path = project_root / filepath if not Path(filepath).is_absolute() else Path(filepath)
    if not path.is_file():
        return {}
    try:
        text = path.read_text(encoding="utf-8")
        health_m = re.search(r"architectural_health\s*:\s*['\"]?(\w+)", text)
        security_m = re.search(r"security_tier\s*:\s*['\"]?(\w+)", text)
        return {
            "health": health_m.group(1) if health_m else "normal",
            "security": security_m.group(1) if security_m else "normal",
        }
    except Exception:
        return {}


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

def validate_project(project_root: Path) -> tuple[bool, str]:
    """Run LOI validation. Returns (passed, message)."""
    try:
        from validate_loi import validate

        result = validate(project_root)
        if not result.ok:
            return False, f"Validation failed with {len(result.errors)} errors:\n" + "\n".join(
                f"  - {e}" for e in result.errors
            )
        msg = "Validation passed"
        if result.warnings:
            msg += f" ({len(result.warnings)} warnings)"
        return True, msg
    except Exception as e:
        return False, f"Validation error: {e}"


# ---------------------------------------------------------------------------
# Draft PR creation
# ---------------------------------------------------------------------------

def create_draft_pr(
    project_root: Path,
    changed_files: list[str],
    all_entries: list[dict],
) -> str | None:
    """Create a branch with markdown changes, push, and open a draft PR.

    Returns the PR URL or None on failure.
    """
    timestamp = datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S")
    room_names = []
    for f in changed_files:
        name = Path(f).stem
        if name not in room_names:
            room_names.append(name)
    room_label = "-".join(room_names[:3])
    branch = f"loi/intent-{room_label}-{timestamp}"

    def run(cmd: list[str]) -> subprocess.CompletedProcess:
        return subprocess.run(cmd, capture_output=True, text=True, cwd=project_root)

    current = run(["git", "rev-parse", "--abbrev-ref", "HEAD"]).stdout.strip()

    r = run(["git", "checkout", "-b", branch])
    if r.returncode != 0:
        print(f"[LOI Watcher] Failed to create branch: {r.stderr.strip()}")
        return None

    run(["git", "add"] + changed_files)
    r = run(["git", "commit", "-m", f"loi: intent update — {', '.join(room_names)}"])
    if r.returncode != 0:
        print(f"[LOI Watcher] Nothing to commit: {r.stderr.strip()}")
        run(["git", "checkout", current])
        run(["git", "branch", "-D", branch])
        return None

    r = run(["git", "push", "-u", "origin", branch])
    if r.returncode != 0:
        print(f"[LOI Watcher] Failed to push: {r.stderr.strip()}")
        run(["git", "checkout", current])
        run(["git", "branch", "-D", branch])
        return None

    source_files = sorted(set(e["source_file"] for e in all_entries))
    body_lines = [
        "## Intent Delta",
        "",
        "| Source File | Changed Field |",
        "|-------------|---------------|",
    ]
    for e in all_entries:
        field_val = e["changed_line"][:80]
        body_lines.append(f"| `{e['source_file']}` | {field_val} |")

    body_lines += [
        "",
        "## Affected Source Files",
        "",
    ]
    for sf in source_files:
        body_lines.append(f"- `{sf}`")

    body_lines += [
        "",
        "---",
        "Approve this PR then run `/loi implement` to generate the code changes.",
        "",
        "*Created by [LOI Watcher](skills/loi/scripts/watcher.py)*",
    ]

    title = f"LOI Intent: {', '.join(room_names)}"
    body = "\n".join(body_lines)

    r = run(["gh", "pr", "create", "--draft", "--title", title, "--body", body])
    pr_url = None
    if r.returncode == 0:
        pr_url = r.stdout.strip()
        print(f"[LOI Watcher] Draft PR created: {pr_url}")
    else:
        print(f"[LOI Watcher] Failed to create PR: {r.stderr.strip()}")

    run(["git", "checkout", current])

    return pr_url


# ---------------------------------------------------------------------------
# Worker prompt (auto mode)
# ---------------------------------------------------------------------------

def build_worker_prompt(filepaths: list[str], diffs: dict[str, str], entries: list[dict]) -> str:
    """Construct the prompt to send to the AI worker."""
    file_list = ", ".join(sorted(set(e["source_file"] for e in entries)))
    room_list = ", ".join(filepaths)

    diff_block = "\n\n".join(
        f"--- {fp} ---\n{diff}" for fp, diff in diffs.items()
    )

    return (
        f"The Architect has updated LOI Contracts in: {room_list}\n\n"
        f"Affected source files: {file_list}\n\n"
        f"Diffs:\n```\n{diff_block}\n```\n\n"
        f"Task: Run /loi implement to sync the source code with the updated intent. "
        f"Create a branch, implement the changes, run tests, and open a PR."
    )


# ---------------------------------------------------------------------------
# Event handler with batch support
# ---------------------------------------------------------------------------

POLICY_TIERS = [
    "notify-only",      # validate + notify; never invoke the implement worker
    "draft-only",       # validate + branch + draft PR; worker not invoked
    "docs-safe",        # implement only if all source files are under docs/
    "tests-safe",       # implement only if all source files are test files
    "scoped-code-safe", # implement only if all source files match --allowed-scopes
    "full-auto",        # implement unconditionally (opt-in)
]

HEALTH_SEVERITY = {"normal": 0, "warning": 1, "critical": 2}
SECURITY_SEVERITY = {"normal": 0, "high": 1, "sensitive": 2}


class LOIHandler(FileSystemEventHandler):
    def __init__(
        self,
        project_root: Path,
        mode: str,
        debounce: float,
        worker_cmd: str,
        notify_backend,
        policy: str = "full-auto",
        allowed_scopes: list[str] | None = None,
        block_governance_security: set[str] | None = None,
    ):
        self.project_root = project_root
        self.mode = mode
        self.debounce = debounce
        self.worker_cmd = worker_cmd
        self.notify_backend = notify_backend
        self.policy = policy
        self.allowed_scopes = allowed_scopes or []
        self.block_governance_security = block_governance_security or {"sensitive"}
        self._pending_files: set[str] = set()
        self._batch_timer = None
        self._lock = threading.Lock()

    # -----------------------------------------------------------------------
    # Governance helper
    # -----------------------------------------------------------------------

    def _extract_batch_governance(self, changed_room_files: list[str]) -> dict:
        """Return the worst health/security flags across all changed rooms."""
        all_gov: list[dict] = []
        for filepath in changed_room_files:
            rel = str(Path(filepath).relative_to(self.project_root))
            flags = extract_governance_flags(self.project_root, rel)
            if flags:
                all_gov.append(flags)
        if not all_gov:
            return {}
        severity_h = {"normal": 0, "warning": 1, "critical": 2}
        severity_s = {"normal": 0, "high": 1, "sensitive": 2}
        return {
            "health": max(
                (g.get("health", "normal") for g in all_gov),
                key=lambda h: severity_h.get(h, 0),
            ),
            "security": max(
                (g.get("security", "normal") for g in all_gov),
                key=lambda s: severity_s.get(s, 0),
            ),
        }

    # -----------------------------------------------------------------------
    # Policy gate (Gap 4)
    # -----------------------------------------------------------------------

    def _check_policy(
        self,
        entries: list[dict],
        governance: dict,
        changed_room_files: list[str],
    ) -> tuple[bool, str]:
        """Return (allowed, reason) for the current implement policy.

        Called only in auto mode. Handles governance-aware blocking,
        scope filtering, and optional room-claim conflict checks.
        """
        policy = self.policy

        # notify-only: never implement
        if policy == "notify-only":
            return False, "policy=notify-only — auto-implement disabled"

        # Governance blocking — configurable set of security tiers to refuse
        room_security = governance.get("security", "normal")
        if room_security in self.block_governance_security:
            return False, (
                f"governance block: changed room has security={room_security}. "
                f"To override, remove '{room_security}' from --block-governance-security or set it to 'none'."
            )

        # Always block on critical health
        room_health = governance.get("health", "normal")
        if HEALTH_SEVERITY.get(room_health, 0) >= HEALTH_SEVERITY["critical"]:
            return False, (
                f"governance block: changed room has health={room_health}. "
                f"Resolve the architectural issue before auto-implementing."
            )

        # draft-only: PR created, worker not invoked
        if policy == "draft-only":
            return False, "policy=draft-only — branch/PR created, worker not invoked"

        # Scope checks
        source_files = [e["source_file"] for e in entries]

        if policy == "docs-safe":
            blocked = [
                f for f in source_files
                if not (f.startswith("docs/") or fnmatch.fnmatch(f, "docs/**"))
            ]
            if blocked:
                return False, f"docs-safe: source files outside docs/: {', '.join(blocked)}"

        elif policy == "tests-safe":
            def _is_test(f: str) -> bool:
                return (
                    f.startswith("tests/") or
                    fnmatch.fnmatch(f, "tests/**") or
                    "_test." in f or
                    f.endswith(".test.ts") or
                    f.endswith(".spec.ts") or
                    f.endswith("_spec.rb")
                )
            blocked = [f for f in source_files if not _is_test(f)]
            if blocked:
                return False, f"tests-safe: source files outside tests/: {', '.join(blocked)}"

        elif policy == "scoped-code-safe":
            if not self.allowed_scopes:
                return False, "scoped-code-safe requires --allowed-scopes"
            blocked = [
                f for f in source_files
                if not any(fnmatch.fnmatch(f, scope) for scope in self.allowed_scopes)
            ]
            if blocked:
                return False, (
                    f"scoped-code-safe: files outside allowed scopes "
                    f"({', '.join(self.allowed_scopes)}): {', '.join(blocked)}"
                )

        # Advisory room-claim check (non-fatal warning unless edit conflict)
        try:
            from runtime import ClaimsStore, check_conflict

            store = ClaimsStore(self.project_root)
            for filepath in changed_room_files:
                rel = str(Path(filepath).relative_to(self.project_root))
                existing = store.get_claims_for(rel)
                action, msg = check_conflict(existing, "edit")
                if action == "conflict":
                    return False, f"room claim conflict: {msg}"
                if action in ("allow_with_warning", "governance_sensitive"):
                    print(f"[LOI Watcher] Claim notice: {msg}")
        except Exception:
            pass  # runtime coordination is optional

        return True, "allowed"

    def on_modified(self, event):
        if not isinstance(event, FileModifiedEvent):
            return
        if not event.src_path.endswith(".md"):
            return

        with self._lock:
            self._pending_files.add(event.src_path)
            if self._batch_timer:
                self._batch_timer.cancel()
            self._batch_timer = threading.Timer(self.debounce, self._process_batch)
            self._batch_timer.start()

    def _process_batch(self):
        """Process all accumulated file changes as a single batch."""
        with self._lock:
            files = list(self._pending_files)
            self._pending_files.clear()
            self._batch_timer = None

        if not files:
            return

        print(f"\n[LOI Watcher] Changes detected in {len(files)} file(s):")
        for f in files:
            print(f"  - {f}")

        all_entries: list[dict] = []
        diffs: dict[str, str] = {}
        changed_room_files: list[str] = []

        for filepath in files:
            diff = get_intent_diff(filepath, self.project_root)
            if not diff:
                print(f"[LOI Watcher] {Path(filepath).name}: no uncommitted diff — skipping.")
                continue

            entries = extract_changed_entries(diff)
            if not entries:
                print(f"[LOI Watcher] {Path(filepath).name}: no intent fields changed — skipping.")
                continue

            diffs[filepath] = diff
            all_entries.extend(entries)
            changed_room_files.append(filepath)

        if not all_entries:
            print("[LOI Watcher] No actionable intent changes found.")
            return

        print(f"[LOI Watcher] {len(all_entries)} intent changes across {len(changed_room_files)} room(s):")
        for e in all_entries:
            print(f"  - {e['source_file']}: {e['changed_line'][:80]}")

        if self.mode == "dry-run":
            print("[LOI Watcher] DRY RUN — no action taken.")
            return

        passed, msg = validate_project(self.project_root)
        print(f"[LOI Watcher] {msg}")
        if not passed:
            print("[LOI Watcher] Skipping — fix validation errors first.")
            return

        repo_name = get_repo_name(self.project_root)
        room_names = [Path(f).stem for f in changed_room_files]

        if self.mode == "notify":
            pr_url = create_draft_pr(self.project_root, changed_room_files, all_entries)

            # Build structured table diff
            table_diff_parts = []
            for filepath in changed_room_files:
                rel = str(Path(filepath).relative_to(self.project_root))
                td = compute_table_diff(self.project_root, rel)
                if td:
                    table_diff_parts.append(td)
            table_diff = "\n\n".join(table_diff_parts) or None

            governance = self._extract_batch_governance(changed_room_files)

            payload = {
                "repo": repo_name,
                "path": ", ".join(str(Path(f).relative_to(self.project_root)) for f in changed_room_files),
                "summary": f"Intent change in {', '.join(room_names)}",
                "pr_url": pr_url,
                "table_diff": table_diff,
                "governance": governance,
                "entries": [e for e in all_entries[:20]],
            }

            if self.notify_backend is not None:
                try:
                    self.notify_backend.send("room.changed", payload)
                except Exception as exc:
                    # Single error boundary — backends propagate, watcher absorbs
                    print(f"[LOI Watcher] Notification failed ({type(exc).__name__}): {exc}")

            return

        if self.mode == "auto":
            auto_governance = self._extract_batch_governance(changed_room_files)
            allowed, reason = self._check_policy(all_entries, auto_governance, changed_room_files)

            if not allowed:
                print(f"[LOI Watcher] Auto-implement blocked: {reason}")
                # For draft-only, still create a branch/PR and notify
                if self.policy == "draft-only":
                    pr_url = create_draft_pr(self.project_root, changed_room_files, all_entries)
                    payload = {
                        "repo": get_repo_name(self.project_root),
                        "path": ", ".join(
                            str(Path(f).relative_to(self.project_root)) for f in changed_room_files
                        ),
                        "summary": f"Draft PR (policy=draft-only): {', '.join(Path(f).stem for f in changed_room_files)}",
                        "pr_url": pr_url,
                        "governance": auto_governance,
                        "entries": all_entries[:20],
                    }
                    if self.notify_backend is not None:
                        try:
                            self.notify_backend.send("room.changed", payload)
                        except Exception as exc:
                            print(f"[LOI Watcher] Notification failed ({type(exc).__name__}): {exc}")
                return

            prompt = build_worker_prompt(changed_room_files, diffs, all_entries)
            print(f"[LOI Watcher] Policy={self.policy} — triggering worker: {self.worker_cmd}")
            subprocess.run(
                [
                    self.worker_cmd, "-p", prompt,
                    "--allowedTools", "Edit,Write,Read,Glob,Grep,Bash",
                ],
                cwd=self.project_root,
            )


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="LOI Level 7 — Watch docs/index/ and trigger on intent changes",
    )
    parser.add_argument(
        "--watch-path", default="docs/index",
        help="Project root or docs/index/ directory to watch (default: ./docs/index)",
    )
    parser.add_argument(
        "--mode", choices=["notify", "auto", "dry-run"], default="notify",
        help="notify (default): validate + draft PR + notification. auto: validate + implement. dry-run: log only.",
    )
    parser.add_argument(
        "--debounce", type=float, default=5.0,
        help="Seconds to wait for additional changes before processing batch (default: 5.0)",
    )
    parser.add_argument(
        "--worker-cmd", default="claude",
        help="Command to invoke the AI worker in auto mode (default: claude)",
    )

    # Notifier backend options
    notify_group = parser.add_argument_group("Notification")
    notify_group.add_argument(
        "--notify-backend", default="stdout",
        choices=["stdout", "file", "webhook", "slack"],
        help="Notification backend (default: stdout)",
    )
    notify_group.add_argument(
        "--notify-url",
        help="Webhook or Slack incoming webhook URL",
    )
    notify_group.add_argument(
        "--notify-file", default="loi-events.jsonl",
        help="File path for file backend (default: loi-events.jsonl)",
    )
    notify_group.add_argument(
        "--notify-token-env",
        help="Env var name holding bearer token for webhook backend",
    )

    # Legacy Slack flags (kept for backward compatibility)
    legacy = parser.add_argument_group("Legacy Slack (deprecated, use --notify-backend slack)")
    legacy.add_argument("--slack-webhook", help="Slack incoming webhook URL (use --notify-url instead)")
    legacy.add_argument("--slack-channel", help="Slack channel (deprecated)")

    # Policy tiers (Gap 4)
    policy_group = parser.add_argument_group("Implementation Policy (auto mode)")
    policy_group.add_argument(
        "--policy",
        choices=POLICY_TIERS,
        default="full-auto",
        help=(
            "Controls what auto mode is allowed to implement. "
            "notify-only: never implement; "
            "draft-only: branch+PR but no worker; "
            "docs-safe: only docs/ source files; "
            "tests-safe: only test files; "
            "scoped-code-safe: only files matching --allowed-scopes; "
            "full-auto: no restriction (default)"
        ),
    )
    policy_group.add_argument(
        "--allowed-scopes",
        help="Comma-separated glob patterns for scoped-code-safe policy (e.g. 'docs/**,tests/**')",
    )
    policy_group.add_argument(
        "--block-governance-security",
        default="sensitive",
        help=(
            "Comma-separated security tiers that block auto-implement "
            "(default: sensitive). Use 'none' to disable governance blocking."
        ),
    )

    parser.add_argument("--dry-run", action="store_true", help="Shorthand for --mode dry-run")
    args = parser.parse_args()

    if args.dry_run:
        args.mode = "dry-run"

    # Handle legacy Slack args
    if args.slack_webhook and not args.notify_url:
        args.notify_backend = "slack"
        args.notify_url = args.slack_webhook
        print("[LOI Watcher] Note: --slack-webhook is deprecated; use --notify-backend slack --notify-url <url>")

    project_root, watch_dir = resolve_watch_dir(args.watch_path)

    if not watch_dir.is_dir():
        print(f"Error: {watch_dir} does not exist. Generate an LOI index first.", file=sys.stderr)
        sys.exit(1)

    # Build the notify backend
    notify_backend = None
    if args.mode != "dry-run":
        scripts_dir = Path(__file__).resolve().parent
        sys.path.insert(0, str(scripts_dir))
        from backends import load_backend

        backend_config: dict = {"backend": args.notify_backend}
        if args.notify_url:
            backend_config["notify_url"] = args.notify_url
        if args.notify_file:
            backend_config["file_path"] = args.notify_file
        if args.notify_token_env:
            backend_config["auth_token_env"] = args.notify_token_env

        try:
            notify_backend = load_backend(backend_config)
        except ValueError as exc:
            print(f"[LOI Watcher] Error loading notify backend: {exc}", file=sys.stderr)
            sys.exit(1)

    # Parse policy options
    allowed_scopes = (
        [s.strip() for s in args.allowed_scopes.split(",") if s.strip()]
        if args.allowed_scopes else []
    )
    block_governance_security: set[str] = set()
    if args.block_governance_security.lower() != "none":
        block_governance_security = {
            t.strip() for t in args.block_governance_security.split(",") if t.strip()
        }

    print(f"[LOI Watcher] Project root: {project_root}")
    print(f"[LOI Watcher] Monitoring: {watch_dir}")
    print(f"[LOI Watcher] Mode: {args.mode}")
    print(f"[LOI Watcher] Batch window: {args.debounce}s")
    if args.mode != "dry-run":
        print(f"[LOI Watcher] Notify backend: {args.notify_backend}")
    if args.mode == "auto":
        if not shutil.which(args.worker_cmd):
            print(
                f"[LOI Watcher] Error: --worker-cmd '{args.worker_cmd}' not found in PATH.",
                file=sys.stderr,
            )
            sys.exit(1)
        print(f"[LOI Watcher] Worker command: {args.worker_cmd}")
        print(f"[LOI Watcher] Policy: {args.policy}")
        if block_governance_security:
            print(f"[LOI Watcher] Governance block: security tiers {block_governance_security}")
        if allowed_scopes:
            print(f"[LOI Watcher] Allowed scopes: {allowed_scopes}")
    print("[LOI Watcher] Waiting for changes... (Ctrl+C to stop)\n")

    handler = LOIHandler(
        project_root=project_root,
        mode=args.mode,
        debounce=args.debounce,
        worker_cmd=args.worker_cmd,
        notify_backend=notify_backend,
        policy=args.policy,
        allowed_scopes=allowed_scopes,
        block_governance_security=block_governance_security,
    )
    observer = Observer()
    observer.schedule(handler, str(watch_dir), recursive=True)
    observer.start()

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        print("\n[LOI Watcher] Shutting down.")
        observer.stop()
    observer.join()


if __name__ == "__main__":
    main()
