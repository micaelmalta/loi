#!/usr/bin/env python3
"""LOI Level 7 — Background daemon that watches docs/index/ for markdown changes
and triggers AI implementation of intent deltas.

Usage:
    python3 watcher.py [options]

Options:
    --watch-path PATH    Directory to watch (default: docs/index)
    --debounce SECS      Seconds to wait before processing (default: 2.0)
    --worker-cmd CMD     Command to run with the diff context (default: claude -p)
    --dry-run            Print what would be triggered without executing

Requires: pip install watchdog
"""

import argparse
import re
import subprocess
import sys
import time
from pathlib import Path

try:
    from watchdog.observers import Observer
    from watchdog.events import FileSystemEventHandler, FileModifiedEvent
except ImportError:
    print("Error: watchdog not installed. Run: pip install watchdog", file=sys.stderr)
    sys.exit(1)


def find_project_root() -> Path:
    """Walk up from cwd to find the git root."""
    p = Path.cwd()
    while p != p.parent:
        if (p / ".git").exists():
            return p
        p = p.parent
    return Path.cwd()


def get_intent_diff(filepath: str) -> str | None:
    """Run git diff on the file and return the diff output, or None if clean."""
    result = subprocess.run(
        ["git", "diff", "HEAD", "--", filepath],
        capture_output=True, text=True,
    )
    diff = result.stdout.strip()
    return diff if diff else None


def extract_changed_entries(diff: str) -> list[dict]:
    """Parse a git diff to find changed DOES/SYMBOLS fields and their file headings."""
    entries = []
    current_file = None

    for line in diff.splitlines():
        # Track which entry heading we're in
        heading_match = re.match(r"[+ ]# (\S+\.\w+)", line)
        if heading_match:
            current_file = heading_match.group(1)

        # Detect changed intent fields
        if line.startswith("+") and not line.startswith("+++"):
            for field in ("DOES:", "SYMBOLS:", "TYPE:", "INTERFACE:", "PATTERNS:"):
                if field in line and current_file:
                    entries.append({
                        "source_file": current_file,
                        "changed_line": line[1:].strip(),
                    })
                    break

    return entries


def build_worker_prompt(filepath: str, diff: str, entries: list[dict]) -> str:
    """Construct the prompt to send to the AI worker."""
    file_list = ", ".join(set(e["source_file"] for e in entries))
    return (
        f"The Architect has updated the LOI Contract in {filepath}.\n\n"
        f"Affected source files: {file_list}\n\n"
        f"Diff:\n```\n{diff}\n```\n\n"
        f"Task: Run /loi implement to sync the source code with the updated intent. "
        f"Create a branch, implement the changes, run tests, and open a PR."
    )


class LOIHandler(FileSystemEventHandler):
    def __init__(self, debounce: float, worker_cmd: str, dry_run: bool):
        self.debounce = debounce
        self.worker_cmd = worker_cmd
        self.dry_run = dry_run
        self._pending: dict[str, float] = {}

    def on_modified(self, event):
        if not isinstance(event, FileModifiedEvent):
            return
        if not event.src_path.endswith(".md"):
            return

        self._pending[event.src_path] = time.time()
        # Simple debounce: process after delay
        time.sleep(self.debounce)

        # Only process if no newer event arrived
        if self._pending.get(event.src_path, 0) <= time.time() - self.debounce:
            return

        self._process(event.src_path)
        self._pending.pop(event.src_path, None)

    def _process(self, filepath: str):
        print(f"\n[LOI Watcher] Change detected: {filepath}")

        diff = get_intent_diff(filepath)
        if not diff:
            print("[LOI Watcher] No uncommitted diff — skipping.")
            return

        entries = extract_changed_entries(diff)
        if not entries:
            print("[LOI Watcher] No intent fields changed — skipping.")
            return

        print(f"[LOI Watcher] Found {len(entries)} changed intent entries:")
        for e in entries:
            print(f"  - {e['source_file']}: {e['changed_line'][:80]}")

        prompt = build_worker_prompt(filepath, diff, entries)

        if self.dry_run:
            print(f"[LOI Watcher] DRY RUN — would execute: {self.worker_cmd}")
            print(f"[LOI Watcher] Prompt:\n{prompt[:500]}...")
            return

        print(f"[LOI Watcher] Triggering worker: {self.worker_cmd}")
        subprocess.run(
            [self.worker_cmd, "-p", prompt],
            cwd=find_project_root(),
        )


def main():
    parser = argparse.ArgumentParser(
        description="LOI Level 7 — Watch docs/index/ and trigger AI implementation on intent changes",
    )
    parser.add_argument(
        "--watch-path", default="docs/index",
        help="Directory to watch (default: docs/index)",
    )
    parser.add_argument(
        "--debounce", type=float, default=2.0,
        help="Seconds to wait before processing a change (default: 2.0)",
    )
    parser.add_argument(
        "--worker-cmd", default="claude",
        help="Command to invoke the AI worker (default: claude)",
    )
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Print what would be triggered without executing",
    )
    args = parser.parse_args()

    root = find_project_root()
    watch_dir = root / args.watch_path

    if not watch_dir.is_dir():
        print(f"Error: {watch_dir} does not exist. Generate an LOI index first.", file=sys.stderr)
        sys.exit(1)

    print(f"[LOI Watcher] Monitoring: {watch_dir}")
    print(f"[LOI Watcher] Worker command: {args.worker_cmd}")
    print(f"[LOI Watcher] Debounce: {args.debounce}s")
    if args.dry_run:
        print("[LOI Watcher] DRY RUN mode — no commands will be executed")
    print("[LOI Watcher] Waiting for changes... (Ctrl+C to stop)\n")

    handler = LOIHandler(args.debounce, args.worker_cmd, args.dry_run)
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
