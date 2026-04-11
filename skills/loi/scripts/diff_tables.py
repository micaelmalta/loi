#!/usr/bin/env python3
"""LOI Table Diff — compute semantic diffs over TASK / PATTERN / GOVERNANCE tables.

Compares two versions of a room file and reports which rows were added,
removed, or changed in each structured table.

Usage:
    python3 diff_tables.py <project-root> docs/index/auth/_root.md
    python3 diff_tables.py <project-root> docs/index/auth/_root.md --from HEAD~1 --to HEAD

The script reads both revisions from git and parses the TASK → LOAD,
PATTERN → LOAD, and GOVERNANCE WATCHLIST tables.

Can also be called programmatically:
    from diff_tables import diff_file_against_head
    summary = diff_file_against_head(project_root, "docs/index/auth/_root.md")
"""

import argparse
import re
import subprocess
import sys
from pathlib import Path


# ---------------------------------------------------------------------------
# Table parsing
# ---------------------------------------------------------------------------

TABLE_HEADERS = {
    "TASK": re.compile(r"^#+\s+TASK", re.IGNORECASE),
    "PATTERN": re.compile(r"^#+\s+PATTERN", re.IGNORECASE),
    "GOVERNANCE": re.compile(r"^#+\s+GOVERNANCE", re.IGNORECASE),
}


def _parse_table_rows(text: str, section_pattern: re.Pattern) -> list[tuple[str, ...]]:
    """Extract rows from a markdown table under the given section heading.

    Returns a list of tuples (col1, col2, ...) with whitespace stripped.
    """
    in_section = False
    rows: list[tuple[str, ...]] = []
    header_row_seen = False

    for line in text.splitlines():
        # Detect section start
        if section_pattern.match(line.strip()):
            in_section = True
            header_row_seen = False
            continue

        # Stop at next heading
        if in_section and re.match(r"^#+\s+", line) and not section_pattern.match(line.strip()):
            in_section = False
            continue

        if not in_section:
            continue

        if not line.strip().startswith("|"):
            continue

        cells = [c.strip() for c in line.split("|")]
        cells = [c for c in cells if c]  # remove empty from leading/trailing pipes

        if not cells:
            continue

        # Skip separator lines like | --- | --- |
        if all(re.match(r"^[-:]+$", c) for c in cells):
            header_row_seen = True
            continue

        if not header_row_seen:
            # First non-separator row is the header — skip it
            header_row_seen = True
            continue

        rows.append(tuple(cells))

    return rows


def parse_tables(text: str) -> dict[str, list[tuple]]:
    """Parse all three table types from a room file text."""
    return {
        name: _parse_table_rows(text, pattern)
        for name, pattern in TABLE_HEADERS.items()
    }


# ---------------------------------------------------------------------------
# Row key extraction
# ---------------------------------------------------------------------------

def _row_key(row: tuple) -> str:
    """Return a stable key for a row (first non-empty cell)."""
    return row[0] if row else ""


# ---------------------------------------------------------------------------
# Diff logic
# ---------------------------------------------------------------------------

def _rows_to_dict(rows: list[tuple], table_name: str) -> dict[str, tuple]:
    """Build a key→row dict, warning when the first-column key is not unique."""
    seen: dict[str, tuple] = {}
    for row in rows:
        key = _row_key(row)
        if key in seen:
            print(
                f"[LOI diff-tables] WARNING: duplicate key {key!r} in {table_name} table — "
                f"later row overwrites earlier one in diff"
            )
        seen[key] = row
    return seen


def diff_tables(old_text: str, new_text: str) -> dict[str, dict]:
    """Compare table contents between two versions of a room file.

    Returns:
        {
            "TASK": {
                "added": [row, ...],
                "removed": [row, ...],
                "changed": [(old_row, new_row), ...],
            },
            "PATTERN": { ... },
            "GOVERNANCE": { ... },
        }
    """
    old_tables = parse_tables(old_text)
    new_tables = parse_tables(new_text)

    result: dict[str, dict] = {}

    for table_name in ("TASK", "PATTERN", "GOVERNANCE"):
        old_rows = _rows_to_dict(old_tables.get(table_name, []), table_name)
        new_rows = _rows_to_dict(new_tables.get(table_name, []), table_name)

        added = [new_rows[k] for k in new_rows if k not in old_rows]
        removed = [old_rows[k] for k in old_rows if k not in new_rows]
        changed = [
            (old_rows[k], new_rows[k])
            for k in new_rows
            if k in old_rows and old_rows[k] != new_rows[k]
        ]

        result[table_name] = {
            "added": added,
            "removed": removed,
            "changed": changed,
        }

    return result


# ---------------------------------------------------------------------------
# Formatting
# ---------------------------------------------------------------------------

def _format_row(row: tuple) -> str:
    return " → ".join(row)


def format_diff(diff: dict[str, dict]) -> str:
    """Render a table diff as a human-readable summary string."""
    lines: list[str] = []

    for table_name in ("TASK", "PATTERN", "GOVERNANCE"):
        changes = diff[table_name]
        if not any(changes[k] for k in ("added", "removed", "changed")):
            continue

        lines.append(f"{table_name} changes")
        for row in changes["added"]:
            lines.append(f"  + added:   {_format_row(row)}")
        for row in changes["removed"]:
            lines.append(f"  - removed: {_format_row(row)}")
        for old, new in changes["changed"]:
            lines.append(f"  ~ changed: {_format_row(old)}")
            lines.append(f"         → : {_format_row(new)}")
        lines.append("")

    return "\n".join(lines).rstrip()


# ---------------------------------------------------------------------------
# Git helpers
# ---------------------------------------------------------------------------

def _git_show(project_root: Path, ref: str, filepath: str) -> str | None:
    """Return file contents at a given git ref, or None if not found."""
    result = subprocess.run(
        ["git", "show", f"{ref}:{filepath}"],
        capture_output=True, text=True, cwd=project_root,
    )
    if result.returncode == 0:
        return result.stdout
    return None


def diff_file_between_refs(
    project_root: Path,
    filepath: str,
    from_ref: str = "HEAD~1",
    to_ref: str = "HEAD",
) -> str | None:
    """Diff tables in *filepath* between *from_ref* and *to_ref*.

    Returns formatted summary string, or None if no table changes.
    """
    old_text = _git_show(project_root, from_ref, filepath) or ""
    new_text = _git_show(project_root, to_ref, filepath) or ""

    if old_text == new_text:
        return None

    diff = diff_tables(old_text, new_text)
    summary = format_diff(diff)
    return summary if summary else None


def diff_file_against_head(project_root: Path, filepath: str) -> str | None:
    """Diff tables in *filepath* between HEAD and the working tree.

    Used by the watcher to attach table deltas to notifications.
    """
    # Get HEAD version
    old_text = _git_show(project_root, "HEAD", filepath) or ""

    # Get current working-tree version
    full = project_root / filepath
    if not full.is_file():
        return None
    try:
        new_text = full.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None

    if old_text == new_text:
        return None

    diff = diff_tables(old_text, new_text)
    summary = format_diff(diff)
    return summary if summary else None


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="LOI Table Diff — semantic diff over TASK/PATTERN/GOVERNANCE tables",
    )
    parser.add_argument("project_root", help="Path to the project root")
    parser.add_argument("filepath", help="Relative path to the room file, e.g. docs/index/auth/_root.md")
    parser.add_argument("--from", dest="from_ref", default="HEAD~1",
                        help="Git ref for the old version (default: HEAD~1)")
    parser.add_argument("--to", dest="to_ref", default="HEAD",
                        help="Git ref for the new version (default: HEAD)")
    args = parser.parse_args()

    project_root = Path(args.project_root).resolve()
    if not project_root.is_dir():
        print(f"Error: {project_root} is not a directory")
        sys.exit(2)

    print(f"Table diff: {args.filepath}  ({args.from_ref} → {args.to_ref})\n")

    summary = diff_file_between_refs(project_root, args.filepath, args.from_ref, args.to_ref)

    if summary:
        print(summary)
    else:
        print("(No TASK / PATTERN / GOVERNANCE table changes detected)")


if __name__ == "__main__":
    main()
