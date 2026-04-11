#!/usr/bin/env python3
"""LOI Governance Aggregation — cross-repo risk view from GOVERNANCE WATCHLIST tables.

Scans all docs/index/_root.md files (campus and building routers) for GOVERNANCE
WATCHLIST entries and aggregates them by severity.

Usage:
    python3 governance.py <project-root>
    python3 governance.py <project-root> --security sensitive
    python3 governance.py <project-root> --health warning
    python3 governance.py <project-root> --format json

For multi-repo fleet views, pass multiple project roots:
    python3 governance.py /repos/alpha /repos/beta /repos/gamma
"""

import argparse
import json
import re
import sys
from pathlib import Path


# ---------------------------------------------------------------------------
# Severity ordering
# ---------------------------------------------------------------------------

SECURITY_SEVERITY = {"normal": 0, "high": 1, "sensitive": 2}
HEALTH_SEVERITY = {"normal": 0, "warning": 1, "critical": 2}


def _sec_rank(s: str) -> int:
    return SECURITY_SEVERITY.get(s.lower(), 0)


def _health_rank(h: str) -> int:
    return HEALTH_SEVERITY.get(h.lower(), 0)


# ---------------------------------------------------------------------------
# Parsing
# ---------------------------------------------------------------------------

def parse_governance_table(text: str) -> list[dict]:
    """Extract rows from the GOVERNANCE WATCHLIST table.

    Expected table columns: Room | Health | Security | Committee Note
    Returns list of dicts with keys: room, health, security, note
    """
    rows: list[dict] = []
    in_section = False
    header_seen = False

    for line in text.splitlines():
        if re.match(r"^#+.*\bGOVERNANCE\b", line, re.IGNORECASE):
            in_section = True
            header_seen = False
            continue

        if in_section and re.match(r"^#+\s+", line) and not re.match(r"^#+.*\bGOVERNANCE\b", line, re.IGNORECASE):
            in_section = False
            continue

        if not in_section:
            continue

        if not line.strip().startswith("|"):
            continue

        cells = [c.strip() for c in line.split("|")]
        cells = [c for c in cells if c]

        if not cells:
            continue

        # Skip separator lines
        if all(re.match(r"^[-:]+$", c) for c in cells):
            header_seen = True
            continue

        if not header_seen:
            header_seen = True
            continue

        # Expect: Room | Health | Security | Note
        room = cells[0] if len(cells) > 0 else ""
        health = cells[1] if len(cells) > 1 else "normal"
        security = cells[2] if len(cells) > 2 else "normal"
        note = cells[3] if len(cells) > 3 else ""

        # Strip backtick formatting from all cells
        room = room.strip("`")
        health = health.strip("`").lower()
        security = security.strip("`").lower()

        if room:
            rows.append({
                "room": room,
                "health": health,
                "security": security,
                "note": note,
            })

    return rows


def parse_room_frontmatter_flags(room_file: Path) -> dict:
    """Extract governance flags from a room file's YAML frontmatter."""
    try:
        text = room_file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return {}

    if not text.startswith("---"):
        return {}

    end = text.find("---", 3)
    if end == -1:
        return {}

    fm = {}
    for line in text[3:end].splitlines():
        if "architectural_health" in line:
            m = re.search(r"architectural_health\s*:\s*['\"]?(\w+)", line)
            if m:
                fm["health"] = m.group(1).lower()
        if "security_tier" in line:
            m = re.search(r"security_tier\s*:\s*['\"]?(\w+)", line)
            if m:
                fm["security"] = m.group(1).lower()
        if "committee_notes" in line:
            m = re.search(r'committee_notes\s*:\s*["\']?(.+)', line)
            if m:
                fm["note"] = m.group(1).strip().strip('"').strip("'")
    return fm


# ---------------------------------------------------------------------------
# Aggregation
# ---------------------------------------------------------------------------

def aggregate_governance(project_roots: list[Path]) -> list[dict]:
    """Aggregate governance entries from all roots, sorted by severity."""
    all_entries: list[dict] = []

    for project_root in project_roots:
        index_dir = project_root / "docs" / "index"
        if not index_dir.is_dir():
            continue

        repo_name = project_root.name

        # Scan all _root.md files for GOVERNANCE WATCHLIST tables
        for root_file in index_dir.rglob("_root.md"):
            try:
                text = root_file.read_text(encoding="utf-8")
            except (OSError, UnicodeDecodeError):
                continue

            rows = parse_governance_table(text)
            for row in rows:
                all_entries.append({
                    "repo": repo_name,
                    "source_file": str(root_file.relative_to(project_root)),
                    **row,
                })

        # Also scan individual room frontmatter for flags not in watchlist
        for room_file in index_dir.rglob("*.md"):
            if room_file.name == "_root.md":
                continue
            flags = parse_room_frontmatter_flags(room_file)
            if not flags:
                continue

            health = flags.get("health", "normal")
            security = flags.get("security", "normal")

            # Only surface non-normal rooms that aren't already in the watchlist
            if _health_rank(health) == 0 and _sec_rank(security) == 0:
                continue

            room_path = str(room_file.relative_to(project_root))

            # Skip if already covered by watchlist.
            # room_path is "docs/index/auth/ucan.md" (relative to project_root).
            # Watchlist room values are "auth/ucan.md" (relative to docs/index/).
            # endswith covers both forms without false-positives from substring overlap.
            already = any(
                room_path.endswith(e.get("room", ""))
                for e in all_entries
                if e.get("repo") == repo_name
            )
            if already:
                continue

            all_entries.append({
                "repo": repo_name,
                "source_file": room_path,
                "room": room_path,
                "health": health,
                "security": security,
                "note": flags.get("note", ""),
            })

    # Sort by combined severity (descending)
    all_entries.sort(
        key=lambda e: (_sec_rank(e.get("security", "normal")) + _health_rank(e.get("health", "normal"))),
        reverse=True,
    )

    return all_entries


# ---------------------------------------------------------------------------
# Output formatting
# ---------------------------------------------------------------------------

def format_text(entries: list[dict], verbose: bool = False) -> str:
    if not entries:
        return "  No governance flags found."

    lines = [f"  {'ROOM':<50} {'HEALTH':<10} {'SECURITY':<12} NOTE"]
    lines.append("  " + "-" * 90)
    for e in entries:
        room = e.get("room", "")[:48]
        health = e.get("health", "normal")
        security = e.get("security", "normal")
        note = e.get("note", "")
        repo = e.get("repo", "")
        prefix = f"[{repo}] " if repo else ""
        if verbose:
            lines.append(f"  {prefix}{room:<50} {health:<10} {security:<12}")
            if note:
                lines.append(f"    {note}")
        else:
            lines.append(f"  {prefix}{room:<50} {health:<10} {security:<12} {note[:50]}")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="LOI Governance Aggregation — cross-repo risk view",
    )
    parser.add_argument(
        "project_roots", nargs="+",
        help="One or more project root directories",
    )
    parser.add_argument(
        "--security", help="Filter by security tier (e.g. sensitive, high)",
    )
    parser.add_argument(
        "--health", help="Filter by health status (e.g. warning, critical)",
    )
    parser.add_argument(
        "--format", choices=["text", "json"], default="text",
        help="Output format (default: text)",
    )
    parser.add_argument(
        "--verbose", "-v", action="store_true",
        help="Show full committee_notes without truncation",
    )
    args = parser.parse_args()

    roots = [Path(r).resolve() for r in args.project_roots]
    for r in roots:
        if not r.is_dir():
            print(f"Error: {r} is not a directory")
            sys.exit(2)

    entries = aggregate_governance(roots)

    # Filters
    if args.security:
        entries = [e for e in entries if e.get("security", "").lower() == args.security.lower()]
    if args.health:
        entries = [e for e in entries if e.get("health", "").lower() == args.health.lower()]

    if args.format == "json":
        print(json.dumps(entries, indent=2))
    else:
        root_label = ", ".join(r.name for r in roots)
        print(f"LOI Governance [{root_label}] — {len(entries)} flagged room(s)\n")
        print(format_text(entries, verbose=args.verbose))

    sys.exit(0)


if __name__ == "__main__":
    main()
