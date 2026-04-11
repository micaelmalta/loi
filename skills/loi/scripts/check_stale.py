#!/usr/bin/env python3
"""LOI Stale Index Check — warns when staged source files have a LOI room that wasn't updated.

Run as a pre-commit hook to catch index drift at commit time, before it accumulates.

Usage:
    python3 check_stale.py <project-root>

Exit codes:
    0 — no stale coverage detected, or LOI_STALE_BLOCK is not set
    1 — stale coverage found AND LOI_STALE_BLOCK=1

Environment:
    LOI_STALE_BLOCK=1   Exit 1 to block the commit (default: warn and exit 0)
    LOI_SKIP=1          Skip this check entirely
"""

import os
import re
import subprocess
import sys
from pathlib import Path


# ---------------------------------------------------------------------------
# Git helpers
# ---------------------------------------------------------------------------

def get_staged_files(project_root: Path) -> list[str]:
    """Return staged, non-deleted files relative to project root."""
    result = subprocess.run(
        ["git", "diff", "--cached", "--name-only", "--diff-filter=ACMRT"],
        capture_output=True, text=True, cwd=project_root,
    )
    return [line.strip() for line in result.stdout.splitlines() if line.strip()]


# ---------------------------------------------------------------------------
# Coverage resolution
# ---------------------------------------------------------------------------

def extract_source_paths(room_file: Path) -> list[str]:
    """Extract 'Source paths:' values from a room or router file."""
    try:
        text = room_file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []
    paths: list[str] = []
    for match in re.finditer(r"Source paths?:\s*(.+)", text, re.IGNORECASE):
        for part in match.group(1).split(","):
            cleaned = part.strip().rstrip("/")
            if cleaned:
                paths.append(cleaned)
    return paths


def find_covering_rooms(project_root: Path, source_file: str) -> list[Path]:
    """Return room/router files whose Source paths: cover *source_file*."""
    index_dir = project_root / "docs" / "index"
    if not index_dir.is_dir():
        return []
    covering: list[Path] = []
    for md in index_dir.rglob("*.md"):
        for sp in extract_source_paths(md):
            if source_file.startswith(sp + "/") or source_file == sp or source_file.startswith(sp):
                covering.append(md)
                break
    return covering


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    if os.environ.get("LOI_SKIP", "0") == "1":
        return 0

    project_root = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path.cwd()
    block = os.environ.get("LOI_STALE_BLOCK", "0") == "1"

    staged = get_staged_files(project_root)
    if not staged:
        return 0

    # Index files being staged — any room update counts as "index touched"
    staged_index: set[str] = {f for f in staged if "docs/index" in f and f.endswith(".md")}

    # Source files = staged files outside docs/index/
    source_files = [
        f for f in staged
        if not f.startswith("docs/index/")
    ]
    if not source_files:
        return 0

    # Find rooms covering each source file that were NOT also staged
    stale: dict[str, list[str]] = {}  # room_rel_path → [source_files]
    for sf in source_files:
        for room in find_covering_rooms(project_root, sf):
            rel = str(room.relative_to(project_root))
            if rel not in staged_index:
                stale.setdefault(rel, []).append(sf)

    if not stale:
        return 0

    print("[LOI] Index may be stale — changed source files are covered by these rooms,")
    print("      but the rooms were not updated in this commit:\n")
    for room, files in sorted(stale.items()):
        print(f"  {room}")
        for f in files[:3]:
            print(f"    ← {f}")
        if len(files) > 3:
            print(f"    ← ... and {len(files) - 3} more")
    print()
    if block:
        print("[LOI] Commit blocked. Run '/loi update' to refresh the index.")
        print("      To skip: LOI_SKIP=1 git commit ...")
        return 1
    else:
        print("[LOI] (warning only — set LOI_STALE_BLOCK=1 to block commits)")
        return 0


if __name__ == "__main__":
    sys.exit(main())
