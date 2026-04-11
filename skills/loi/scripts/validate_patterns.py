#!/usr/bin/env python3
"""LOI PATTERN semantic validator.

Checks that each PATTERN entry in the campus _root.md (or a subdomain router)
is actually supported by the room it points to.

Usage:
    python3 validate_patterns.py <project-root>
    python3 validate_patterns.py <project-root> --level 2

Validation levels:
    1  (default) Exact text: pattern label or a normalized form appears in the target room.
    2  Alias-aware: also checks optional `pattern_aliases` metadata in the room frontmatter.

Output distinguishes:
    - missing target room
    - weak semantic support (label not found in room body)
    - alias-only support (Level 2)
    - stale validation timestamp (if `last_validated` metadata present)
"""

import argparse
import re
import sys
from datetime import date, datetime
from pathlib import Path

STALE_DAYS = 14  # warn if last_validated older than this


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def normalize(text: str) -> str:
    """Lowercase, strip punctuation, collapse whitespace."""
    text = text.lower()
    text = re.sub(r"[^\w\s]", " ", text)
    return re.sub(r"\s+", " ", text).strip()


def parse_frontmatter_raw(filepath: Path) -> dict:
    """Parse YAML frontmatter into a dict, handling lists and scalars."""
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return {}

    if not text.startswith("---"):
        return {}
    end = text.find("---", 3)
    if end == -1:
        return {}

    fm: dict = {}
    current_key = None
    current_list: list | None = None

    for line in text[3:end].splitlines():
        stripped = line.strip()
        if not stripped or stripped.startswith("#"):
            continue

        list_item = re.match(r"^\s+-\s+(.*)", line)
        if list_item and current_key and current_list is not None:
            current_list.append(list_item.group(1).strip().strip('"').strip("'"))
            continue

        kv = re.match(r"^(\w[\w_-]*):\s*(.*)", line)
        if kv:
            # Commit previous list
            if current_key and current_list is not None:
                fm[current_key] = current_list

            current_key = kv.group(1)
            val = kv.group(2).strip().strip('"').strip("'")
            if val == "" or val == "[]":
                current_list = []
            elif val.startswith("["):
                # Inline list: [a, b, c]
                items = [x.strip().strip('"').strip("'") for x in val.strip("[]").split(",")]
                fm[current_key] = [i for i in items if i]
                current_key = None
                current_list = None
            else:
                fm[current_key] = val
                current_key = None
                current_list = None
        else:
            current_list = None
            current_key = None

    if current_key and current_list is not None:
        fm[current_key] = current_list

    return fm


def parse_pattern_metadata_block(room_file: Path) -> dict[str, dict]:
    """Extract pattern_metadata YAML list from a room file body.

    Looks for:
        pattern_metadata:
          - name: Token rotation without service restart
            first_introduced: 2026-04-09
            last_validated: 2026-04-10
            validation_source: refresh_token_test.go

    Returns a dict keyed by normalized pattern name.
    """
    try:
        text = room_file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return {}

    # Find pattern_metadata block
    m = re.search(r"pattern_metadata\s*:\s*\n((?:\s+.*\n?)*)", text)
    if not m:
        return {}

    block = m.group(1)
    entries: dict[str, dict] = {}
    current: dict | None = None

    for line in block.splitlines():
        item_m = re.match(r"^\s+-\s+name:\s*(.*)", line)
        if item_m:
            if current and "name" in current:
                entries[normalize(current["name"])] = current
            current = {"name": item_m.group(1).strip()}
            continue

        if current:
            kv = re.match(r"^\s+(\w[\w_-]*):\s*(.*)", line)
            if kv:
                current[kv.group(1)] = kv.group(2).strip()
            elif not line.strip():
                continue
            else:
                # End of block
                break

    if current and "name" in current:
        entries[normalize(current["name"])] = current

    return entries


def read_room_body(room_file: Path) -> str:
    """Read the full text of a room file for content search."""
    try:
        return room_file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return ""


# ---------------------------------------------------------------------------
# Pattern row extraction from _root.md files
# ---------------------------------------------------------------------------

def extract_pattern_rows(root_file: Path) -> list[dict]:
    """Extract PATTERN → LOAD rows from a campus or building _root.md.

    Returns list of dicts: {pattern, target_path, raw_line}
    """
    try:
        text = root_file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []

    rows: list[dict] = []
    in_pattern_section = False

    for line in text.splitlines():
        # Detect PATTERN section header
        if re.match(r"^#+\s+PATTERN", line, re.IGNORECASE):
            in_pattern_section = True
            continue
        # Stop at next heading that isn't pattern-related
        if in_pattern_section and re.match(r"^#+\s+", line):
            if not re.match(r"^#+\s+PATTERN", line, re.IGNORECASE):
                in_pattern_section = False
            continue

        if not in_pattern_section:
            continue

        # Table row: | Pattern label | path/to/room.md |
        if line.strip().startswith("|") and "|" in line:
            cells = [c.strip() for c in line.split("|")]
            cells = [c for c in cells if c]
            if len(cells) >= 2:
                label = cells[0]
                target = cells[1]
                # Skip header row
                if label.lower() in ("pattern", "---"):
                    continue
                if re.match(r"[-:]+", label):
                    continue
                rows.append({
                    "pattern": label,
                    "target_path": target,
                    "raw_line": line.strip(),
                })

    return rows


# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

class PatternValidationResult:
    def __init__(self):
        self.errors: list[str] = []
        self.warnings: list[str] = []
        self.orphans: list[str] = []

    def error(self, msg: str):
        self.errors.append(msg)

    def warn(self, msg: str):
        self.warnings.append(msg)

    def orphan(self, msg: str):
        self.orphans.append(msg)

    @property
    def ok(self) -> bool:
        return len(self.errors) == 0

    def report(self) -> str:
        lines = []
        if self.errors:
            lines.append(f"\n  ERRORS ({len(self.errors)}):")
            for e in self.errors:
                lines.append(f"    - {e}")
        if self.orphans:
            lines.append(f"\n  ORPHANED PATTERNS ({len(self.orphans)}):")
            for o in self.orphans:
                lines.append(f"    - {o}")
        if self.warnings:
            lines.append(f"\n  WARNINGS ({len(self.warnings)}):")
            for w in self.warnings:
                lines.append(f"    - {w}")
        if self.ok and not self.warnings and not self.orphans:
            lines.append("\n  All pattern checks passed.")
        return "\n".join(lines)


def validate_patterns(project_root: Path, level: int = 1) -> PatternValidationResult:
    result = PatternValidationResult()
    index_dir = project_root / "docs" / "index"

    # Collect all _root.md files (campus + buildings), deduplicated
    seen: set[Path] = set()
    root_files: list[Path] = []
    for f in [index_dir / "_root.md"] + list(index_dir.rglob("_root.md")):
        if f.is_file() and f not in seen:
            seen.add(f)
            root_files.append(f)

    today = date.today()

    for root_file in root_files:
        rows = extract_pattern_rows(root_file)
        if not rows:
            continue

        rel_root = root_file.relative_to(project_root)

        for row in rows:
            pattern_label = row["pattern"]
            target_path = row["target_path"]

            # Resolve target room path
            target_file = (index_dir / target_path).resolve()
            if not target_file.is_file():
                # Also try relative to the _root.md's directory
                target_file2 = (root_file.parent / target_path).resolve()
                if target_file2.is_file():
                    target_file = target_file2
                else:
                    result.error(
                        f"{rel_root}: PATTERN '{pattern_label}' → missing target: {target_path}"
                    )
                    continue

            body = read_room_body(target_file)
            norm_label = normalize(pattern_label)
            norm_body = normalize(body)

            # Level 1: exact text check
            if norm_label in norm_body:
                support = "exact"
            else:
                support = "missing"

            # Level 2: alias check
            aliases: list[str] = []
            if level >= 2:
                fm = parse_frontmatter_raw(target_file)
                raw_aliases = fm.get("pattern_aliases", [])
                if isinstance(raw_aliases, str):
                    raw_aliases = [raw_aliases]
                aliases = raw_aliases

                if support == "missing":
                    for alias in aliases:
                        if normalize(alias) in norm_body:
                            support = "alias"
                            break

            # Check pattern_metadata for freshness
            meta_entries = parse_pattern_metadata_block(target_file)
            meta = meta_entries.get(norm_label)

            if meta is None and level >= 2:
                for alias in aliases:
                    meta = meta_entries.get(normalize(alias))
                    if meta:
                        break

            last_validated_str = meta.get("last_validated") if meta else None
            stale = False
            if last_validated_str:
                try:
                    lv = datetime.strptime(last_validated_str, "%Y-%m-%d").date()
                    if (today - lv).days > STALE_DAYS:
                        stale = True
                except ValueError:
                    pass
            elif meta is None and level >= 2:
                # Missing metadata entirely counts as stale warning
                stale = True

            # Report findings
            rel_target = target_file.relative_to(project_root)

            if support == "exact":
                if stale:
                    result.warn(
                        f"{rel_root}: PATTERN '{pattern_label}' → {rel_target} "
                        f"(exact match, but last_validated is stale or missing)"
                    )
            elif support == "alias":
                result.warn(
                    f"{rel_root}: PATTERN '{pattern_label}' → {rel_target} "
                    f"(alias-only support — label not in room body)"
                )
                if stale:
                    result.warn(
                        f"  └─ also stale: last_validated missing or >14d ago"
                    )
            else:
                # No support found
                result.orphan(
                    f"{rel_root}: PATTERN '{pattern_label}' → {rel_target} "
                    f"(label not found in room body; add content or aliases)"
                )

    return result


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Validate LOI PATTERN table entries for semantic grounding.",
    )
    parser.add_argument("project_root", help="Path to the project root")
    parser.add_argument(
        "--level", type=int, choices=[1, 2], default=1,
        help="Validation level: 1=exact text (default), 2=alias-aware + freshness",
    )
    args = parser.parse_args()

    project_root = Path(args.project_root).resolve()
    if not project_root.is_dir():
        print(f"Error: {project_root} is not a directory")
        sys.exit(2)

    print(f"Validating LOI PATTERN tables [level {args.level}]: {project_root / 'docs' / 'index'}")

    result = validate_patterns(project_root, level=args.level)
    print(result.report())

    total = len(result.errors) + len(result.warnings) + len(result.orphans)
    print(f"\n  Summary: {len(result.errors)} errors, {len(result.orphans)} orphans, {len(result.warnings)} warnings")

    sys.exit(0 if result.ok else 1)


if __name__ == "__main__":
    main()
