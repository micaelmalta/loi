#!/usr/bin/env -S python3 -u
"""LOI Level 7 — Background daemon that watches docs/index/ for markdown changes
and triggers AI implementation of intent deltas.

Usage:
    uv run --with watchdog watcher.py [options]
    uv run --with watchdog watcher.py --watch-path /path/to/project
    uv run --with watchdog watcher.py --watch-path /path/to/project/docs/index --dry-run

Options:
    --watch-path PATH    Project root or docs/index dir to watch (default: ./docs/index)
    --debounce SECS      Seconds to wait before processing (default: 2.0)
    --worker-cmd CMD     Command to invoke AI worker (default: claude)
    --dry-run            Print what would be triggered without executing

Requires: uv (https://docs.astral.sh/uv/)
"""

import argparse
import re
import subprocess
import sys
import threading
import time
from pathlib import Path

try:
    from watchdog.observers import Observer
    from watchdog.events import FileSystemEventHandler, FileModifiedEvent
except ImportError:
    print("Error: watchdog not installed. Run with: uv run --with watchdog watcher.py", file=sys.stderr)
    sys.exit(1)


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

    # If it points to a project root that has docs/index/, watch that
    if (p / "docs" / "index").is_dir():
        return find_git_root(p), p / "docs" / "index"

    # If it's already a docs/index dir or any dir with .md files, watch it directly
    if p.is_dir():
        return find_git_root(p), p

    # Treat as relative to cwd
    cwd = Path.cwd()
    rel = cwd / watch_path
    if (rel / "docs" / "index").is_dir():
        return find_git_root(rel), rel / "docs" / "index"
    if rel.is_dir():
        return find_git_root(rel), rel

    return find_git_root(cwd), rel


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
    def __init__(self, project_root: Path, debounce: float, worker_cmd: str, dry_run: bool):
        self.project_root = project_root
        self.debounce = debounce
        self.worker_cmd = worker_cmd
        self.dry_run = dry_run
        self._timers: dict[str, threading.Timer] = {}
        self._lock = threading.Lock()

    def on_modified(self, event):
        if not isinstance(event, FileModifiedEvent):
            return
        if not event.src_path.endswith(".md"):
            return

        # Debounce: cancel any pending timer for this file and start a new one
        with self._lock:
            existing = self._timers.pop(event.src_path, None)
            if existing:
                existing.cancel()

            timer = threading.Timer(self.debounce, self._process, args=[event.src_path])
            self._timers[event.src_path] = timer
            timer.start()

    def _process(self, filepath: str):
        with self._lock:
            self._timers.pop(filepath, None)

        print(f"\n[LOI Watcher] Change detected: {filepath}")

        diff = get_intent_diff(filepath, self.project_root)
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
            [
                self.worker_cmd, "-p", prompt,
                "--allowedTools", "Edit,Write,Read,Glob,Grep,Bash",
            ],
            cwd=self.project_root,
        )


def main():
    parser = argparse.ArgumentParser(
        description="LOI Level 7 — Watch docs/index/ and trigger AI implementation on intent changes",
    )
    parser.add_argument(
        "--watch-path", default="docs/index",
        help="Project root or docs/index/ directory to watch (default: ./docs/index)",
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

    project_root, watch_dir = resolve_watch_dir(args.watch_path)

    if not watch_dir.is_dir():
        print(f"Error: {watch_dir} does not exist. Generate an LOI index first.", file=sys.stderr)
        sys.exit(1)

    print(f"[LOI Watcher] Project root: {project_root}")
    print(f"[LOI Watcher] Monitoring: {watch_dir}")
    print(f"[LOI Watcher] Worker command: {args.worker_cmd}")
    print(f"[LOI Watcher] Debounce: {args.debounce}s")
    if args.dry_run:
        print("[LOI Watcher] DRY RUN mode — no commands will be executed")
    print("[LOI Watcher] Waiting for changes... (Ctrl+C to stop)\n")

    handler = LOIHandler(project_root, args.debounce, args.worker_cmd, args.dry_run)
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
