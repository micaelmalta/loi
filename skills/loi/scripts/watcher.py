#!/usr/bin/env -S python3 -u
"""LOI Level 7 — Background daemon that watches docs/index/ for markdown changes
and triggers validation, draft PRs, Slack notifications, or AI implementation.

Usage:
    uv run --with watchdog watcher.py [options]
    uv run --with watchdog watcher.py --mode notify --slack-webhook https://hooks.slack.com/...
    uv run --with watchdog watcher.py --mode auto --watch-path /path/to/project
    uv run --with watchdog watcher.py --mode dry-run

Modes:
    notify    (default) Validate → create draft PR → Slack notification. No code changes.
    auto      Validate → implement via AI worker → commit → PR. Full autonomy.
    dry-run   Log what would happen without taking any action.

Requires: uv (https://docs.astral.sh/uv/)
"""

import argparse
import json
import re
import subprocess
import sys
import threading
import time
import urllib.request
from datetime import datetime
from pathlib import Path

try:
    from watchdog.observers import Observer
    from watchdog.events import FileSystemEventHandler, FileModifiedEvent
except ImportError:
    print("Error: watchdog not installed. Run with: uv run --with watchdog watcher.py", file=sys.stderr)
    sys.exit(1)


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
    """Resolve watch_path to (project_root, watch_dir).

    Accepts:
      - A project root (has docs/index/ inside it)
      - A direct docs/index path
      - A relative path from cwd
    """
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
        # Extract owner/repo from URL
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
    """Parse a git diff to find changed DOES/SYMBOLS fields and their file headings."""
    entries = []
    current_file = None

    for line in diff.splitlines():
        heading_match = re.match(r"[+ ]# (\S+\.\w+)", line)
        if heading_match:
            current_file = heading_match.group(1)

        if line.startswith("+") and not line.startswith("+++"):
            for field in ("DOES:", "SYMBOLS:", "TYPE:", "INTERFACE:", "PATTERNS:"):
                if field in line and current_file:
                    entries.append({
                        "source_file": current_file,
                        "changed_line": line[1:].strip(),
                    })
                    break

    return entries


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

def validate_room(project_root: Path) -> tuple[bool, str]:
    """Run LOI validation on the project. Returns (passed, message)."""
    try:
        # Import validate_loi from the same scripts directory
        scripts_dir = Path(__file__).resolve().parent
        sys.path.insert(0, str(scripts_dir))
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
    timestamp = datetime.now().strftime("%Y%m%d%H%M%S")
    # Derive room name from the first changed file
    room_names = []
    for f in changed_files:
        name = Path(f).stem
        if name not in room_names:
            room_names.append(name)
    room_label = "-".join(room_names[:3])
    branch = f"loi/intent-{room_label}-{timestamp}"

    def run(cmd: list[str]) -> subprocess.CompletedProcess:
        return subprocess.run(cmd, capture_output=True, text=True, cwd=project_root)

    # Save current branch to return to it
    current = run(["git", "rev-parse", "--abbrev-ref", "HEAD"]).stdout.strip()

    # Create branch and commit
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

    # Build PR body
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

    # Return to original branch
    run(["git", "checkout", current])

    return pr_url


# ---------------------------------------------------------------------------
# Slack notification
# ---------------------------------------------------------------------------

def _build_slack_message(
    repo_name: str,
    room_names: list[str],
    pr_url: str | None,
    entries: list[dict],
) -> str:
    """Build the Slack notification message as markdown."""
    rooms = ", ".join(room_names)
    source_files = sorted(set(e["source_file"] for e in entries))

    lines = [
        f"*LOI Intent Change — {rooms}*",
        "",
        f"*Repo:* {repo_name}  |  *Files affected:* {len(source_files)}",
        "",
    ]
    for e in entries[:10]:
        lines.append(f"• `{e['source_file']}` — {e['changed_line'][:60]}")

    if pr_url:
        lines += ["", f"<{pr_url}|Review PR>"]

    return "\n".join(lines)


def notify_slack_webhook(
    webhook_url: str,
    repo_name: str,
    room_names: list[str],
    pr_url: str | None,
    entries: list[dict],
) -> bool:
    """Post notification via Slack incoming webhook. Returns True on success."""
    rooms = ", ".join(room_names)
    source_files = sorted(set(e["source_file"] for e in entries))

    text = f"LOI intent change in *{repo_name}* — {rooms}"

    blocks = [
        {
            "type": "header",
            "text": {"type": "plain_text", "text": f"LOI Intent Change — {rooms}"},
        },
        {
            "type": "section",
            "fields": [
                {"type": "mrkdwn", "text": f"*Repo:*\n{repo_name}"},
                {"type": "mrkdwn", "text": f"*Files affected:*\n{len(source_files)}"},
            ],
        },
        {
            "type": "section",
            "text": {
                "type": "mrkdwn",
                "text": "\n".join(
                    f"• `{e['source_file']}` — {e['changed_line'][:60]}"
                    for e in entries[:10]
                ),
            },
        },
    ]

    if pr_url:
        blocks.append({
            "type": "actions",
            "elements": [{
                "type": "button",
                "text": {"type": "plain_text", "text": "Review PR"},
                "url": pr_url,
                "style": "primary",
            }],
        })

    payload = json.dumps({"text": text, "blocks": blocks}).encode("utf-8")

    try:
        req = urllib.request.Request(
            webhook_url,
            data=payload,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            ok = resp.status == 200
            if ok:
                print("[LOI Watcher] Slack notification sent (webhook).")
            else:
                print(f"[LOI Watcher] Slack webhook returned status {resp.status}")
            return ok
    except Exception as e:
        print(f"[LOI Watcher] Slack webhook failed: {e}")
        return False


def notify_slack_mcp(
    slack_channel: str,
    repo_name: str,
    room_names: list[str],
    pr_url: str | None,
    entries: list[dict],
    worker_cmd: str = "claude",
    project_root: Path | None = None,
) -> bool:
    """Fallback: send Slack notification via claude MCP tool. Returns True on success."""
    message = _build_slack_message(repo_name, room_names, pr_url, entries)

    prompt = (
        f"Send the following message to the Slack channel '{slack_channel}' "
        f"using the slack_send_message MCP tool. Do not modify the message content.\n\n"
        f"Message:\n{message}"
    )

    try:
        result = subprocess.run(
            [worker_cmd, "-p", prompt, "--allowedTools", "mcp__plugin_slack_slack__slack_send_message"],
            capture_output=True, text=True, timeout=30,
            cwd=project_root or Path.cwd(),
        )
        ok = result.returncode == 0
        if ok:
            print("[LOI Watcher] Slack notification sent (MCP).")
        else:
            print(f"[LOI Watcher] Slack MCP failed: {result.stderr.strip()[:200]}")
        return ok
    except Exception as e:
        print(f"[LOI Watcher] Slack MCP failed: {e}")
        return False


def notify_slack(
    webhook_url: str | None,
    slack_channel: str | None,
    repo_name: str,
    room_names: list[str],
    pr_url: str | None,
    entries: list[dict],
    worker_cmd: str = "claude",
    project_root: Path | None = None,
) -> bool:
    """Send Slack notification. Uses webhook if available, falls back to MCP."""
    if webhook_url:
        return notify_slack_webhook(webhook_url, repo_name, room_names, pr_url, entries)

    if slack_channel:
        print("[LOI Watcher] No webhook URL — falling back to Slack MCP tool.")
        return notify_slack_mcp(
            slack_channel, repo_name, room_names, pr_url, entries,
            worker_cmd=worker_cmd, project_root=project_root,
        )

    print("[LOI Watcher] No --slack-webhook or --slack-channel configured. Skipping notification.")
    return False


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

class LOIHandler(FileSystemEventHandler):
    def __init__(
        self,
        project_root: Path,
        mode: str,
        debounce: float,
        worker_cmd: str,
        slack_webhook: str | None,
        slack_channel: str | None,
    ):
        self.project_root = project_root
        self.mode = mode
        self.debounce = debounce
        self.worker_cmd = worker_cmd
        self.slack_webhook = slack_webhook
        self.slack_channel = slack_channel
        self._pending_files: set[str] = set()
        self._batch_timer: threading.Timer | None = None
        self._lock = threading.Lock()

    def on_modified(self, event):
        if not isinstance(event, FileModifiedEvent):
            return
        if not event.src_path.endswith(".md"):
            return

        with self._lock:
            self._pending_files.add(event.src_path)

            # Reset the batch timer — wait for more changes
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

        # Collect diffs and entries across all changed files
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

        # --- dry-run: stop here ---
        if self.mode == "dry-run":
            print("[LOI Watcher] DRY RUN — no action taken.")
            return

        # --- validate (notify + auto modes) ---
        passed, msg = validate_room(self.project_root)
        print(f"[LOI Watcher] {msg}")
        if not passed:
            print("[LOI Watcher] Skipping — fix validation errors first.")
            return

        # --- notify mode: draft PR + Slack ---
        if self.mode == "notify":
            pr_url = create_draft_pr(self.project_root, changed_room_files, all_entries)

            room_names = [Path(f).stem for f in changed_room_files]
            repo_name = get_repo_name(self.project_root)

            notify_slack(
                webhook_url=self.slack_webhook,
                slack_channel=self.slack_channel,
                repo_name=repo_name,
                room_names=room_names,
                pr_url=pr_url,
                entries=all_entries,
                worker_cmd=self.worker_cmd,
                project_root=self.project_root,
            )

            return

        # --- auto mode: implement via AI worker ---
        if self.mode == "auto":
            prompt = build_worker_prompt(changed_room_files, diffs, all_entries)
            print(f"[LOI Watcher] Triggering worker: {self.worker_cmd}")
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
        help="notify (default): validate + draft PR + Slack. auto: validate + implement. dry-run: log only.",
    )
    parser.add_argument(
        "--debounce", type=float, default=5.0,
        help="Seconds to wait for additional changes before processing batch (default: 5.0)",
    )
    parser.add_argument(
        "--worker-cmd", default="claude",
        help="Command to invoke the AI worker in auto mode (default: claude)",
    )
    parser.add_argument(
        "--slack-webhook",
        help="Slack incoming webhook URL for notifications (preferred)",
    )
    parser.add_argument(
        "--slack-channel",
        help="Slack channel name or ID for MCP fallback (e.g., #loi-approvals, C01234ABCDE)",
    )
    # Keep --dry-run as a convenience alias
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Shorthand for --mode dry-run",
    )
    args = parser.parse_args()

    # --dry-run flag overrides --mode
    if args.dry_run:
        args.mode = "dry-run"

    project_root, watch_dir = resolve_watch_dir(args.watch_path)

    if not watch_dir.is_dir():
        print(f"Error: {watch_dir} does not exist. Generate an LOI index first.", file=sys.stderr)
        sys.exit(1)

    if args.mode == "notify" and not args.slack_webhook and not args.slack_channel:
        print("[LOI Watcher] Warning: no --slack-webhook or --slack-channel set. Draft PRs will be created but no Slack notification sent.")

    print(f"[LOI Watcher] Project root: {project_root}")
    print(f"[LOI Watcher] Monitoring: {watch_dir}")
    print(f"[LOI Watcher] Mode: {args.mode}")
    print(f"[LOI Watcher] Batch window: {args.debounce}s")
    if args.mode == "auto":
        print(f"[LOI Watcher] Worker command: {args.worker_cmd}")
    if args.slack_webhook:
        print("[LOI Watcher] Slack: webhook configured")
    elif args.slack_channel:
        print(f"[LOI Watcher] Slack: MCP fallback → {args.slack_channel}")
    print("[LOI Watcher] Waiting for changes... (Ctrl+C to stop)\n")

    handler = LOIHandler(
        project_root=project_root,
        mode=args.mode,
        debounce=args.debounce,
        worker_cmd=args.worker_cmd,
        slack_webhook=args.slack_webhook,
        slack_channel=args.slack_channel,
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
