#!/usr/bin/env python3
"""Validate a LOI (Library of Intent) index for structural integrity and coverage.

Usage:
    python3 validate_loi.py <project-root>
    python3 validate_loi.py <project-root> --changed-rooms
    python3 validate_loi.py <project-root> --ci

Modes:
    (default)        Validate the full index.
    --changed-rooms  Validate only rooms affected by uncommitted changes.
    --ci             Full validation with machine-readable exit codes; intended for CI pipelines.

Checks:
    1. Campus _root.md exists and has required sections
    2. Building routers exist and reference valid rooms
    3. Room files have YAML frontmatter with required fields (room, see_also)
    4. All cross-references resolve to existing files
    5. Source directory coverage (every dir with code is in at least one room)
    6. Room size limits (~150 entries max)
    7. (--changed-rooms) Only rooms touched by current git changes
    8. File/glob sanity: TASK rows referencing paths/globs that no longer resolve
"""

import argparse
import glob
import os
import re
import subprocess
import sys
from pathlib import Path

# Extensions considered "source code"
SOURCE_EXTS = {
    ".go", ".py", ".js", ".ts", ".tsx", ".jsx", ".rb", ".rs", ".java",
    ".kt", ".swift", ".c", ".cpp", ".h", ".hpp", ".cs", ".php", ".ex",
    ".exs", ".clj", ".scala", ".sh", ".bash", ".zsh",
}

# Tracked directories excluded from coverage
EXCLUDED_DIRS_HARDCODED = {"vendor", "node_modules"}

ENTRY_LIMIT = 150


def parse_gitignore_dirs(project_root: Path) -> set[str]:
    """Extract directory names from .gitignore (lines ending with /)."""
    gitignore = project_root / ".gitignore"
    if not gitignore.is_file():
        return set()
    dirs: set[str] = set()
    try:
        for line in gitignore.read_text(encoding="utf-8").splitlines():
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            if line.endswith("/"):
                name = line.strip("/").split("/")[-1]
                if name:
                    dirs.add(name)
    except OSError:
        pass
    return dirs


def get_excluded_dirs(project_root: Path) -> set[str]:
    """Combine .gitignore directory patterns with hardcoded exclusions."""
    return EXCLUDED_DIRS_HARDCODED | parse_gitignore_dirs(project_root)


class ValidationResult:
    def __init__(self):
        self.errors: list[str] = []
        self.warnings: list[str] = []
        self.total_rooms: int = 0
        self.total_entries: int = 0

    def error(self, msg: str):
        self.errors.append(msg)

    def warn(self, msg: str):
        self.warnings.append(msg)

    @property
    def ok(self) -> bool:
        return len(self.errors) == 0

    def report(self) -> str:
        lines = []
        if self.errors:
            lines.append(f"\n  ERRORS ({len(self.errors)}):")
            for e in self.errors:
                lines.append(f"    - {e}")
        if self.warnings:
            lines.append(f"\n  WARNINGS ({len(self.warnings)}):")
            for w in self.warnings:
                lines.append(f"    - {w}")
        if self.ok and not self.warnings:
            lines.append("\n  All checks passed.")
        return "\n".join(lines)


def parse_frontmatter(filepath: Path) -> dict[str, str] | None:
    """Extract YAML frontmatter as a simple key-value dict. Returns None if missing."""
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None

    if not text.startswith("---"):
        return None

    end = text.find("---", 3)
    if end == -1:
        return None

    fm = {}
    for line in text[3:end].strip().splitlines():
        if ":" in line:
            key, _, val = line.partition(":")
            fm[key.strip()] = val.strip().strip('"').strip("'")
    return fm


def count_entries(filepath: Path) -> int:
    """Count `# filename.ext` entry headings in a room file."""
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return 0
    return len(re.findall(r"^# \S+\.\w+", text, re.MULTILINE))


def extract_md_links(filepath: Path) -> list[str]:
    """Extract markdown table cell references that look like .md file paths."""
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []
    return re.findall(r"[\w./-]+\.md", text)


def find_source_dirs(root: Path, excluded_dirs: set[str]) -> set[str]:
    """Walk the project and return relative dir paths that contain source files."""
    dirs: set[str] = set()
    for dirpath, dirnames, filenames in os.walk(root):
        dirnames[:] = [
            d for d in dirnames
            if d not in excluded_dirs and not d.startswith(".")
        ]
        rel = os.path.relpath(dirpath, root)
        if rel == ".":
            rel = ""
        for f in filenames:
            ext = os.path.splitext(f)[1]
            if ext in SOURCE_EXTS:
                dirs.add(rel)
                break
    return dirs


def extract_source_paths_from_rooms(index_dir: Path) -> set[str]:
    """Collect all 'Source paths:' values from room and router files."""
    paths: set[str] = set()
    pattern = re.compile(r"Source paths?:\s*(.+)", re.IGNORECASE)
    for md in index_dir.rglob("*.md"):
        try:
            text = md.read_text(encoding="utf-8")
        except (OSError, UnicodeDecodeError):
            continue
        for match in pattern.finditer(text):
            for part in match.group(1).split(","):
                cleaned = part.strip().rstrip("/")
                if cleaned:
                    paths.add(cleaned)
    return paths


# ---------------------------------------------------------------------------
# Gap 1B: file/glob sanity checks
# ---------------------------------------------------------------------------

def extract_task_file_refs(room_file: Path, project_root: Path) -> list[str]:
    """Extract file paths and glob patterns from TASK table Load cells.

    Looks for patterns like `path/to/file.ext` or `path/to/*.ext` in table rows.
    """
    try:
        text = room_file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return []

    refs: list[str] = []
    # Match table rows: | description | path/file.ext |
    for line in text.splitlines():
        if not line.strip().startswith("|"):
            continue
        cells = [c.strip() for c in line.split("|")]
        for cell in cells:
            # Skip markdown link text, focus on bare paths
            # A file ref: has an extension and a slash or looks like a script
            if re.match(r"^[\w./-]+\.\w+$", cell) and "/" in cell:
                refs.append(cell)
            # A glob pattern
            elif "*" in cell and "/" in cell:
                refs.append(cell)
    return refs


def check_file_refs(room_file: Path, project_root: Path, result: ValidationResult) -> None:
    """Check that file paths and glob patterns in a room still resolve."""
    rel_room = room_file.relative_to(project_root)
    refs = extract_task_file_refs(room_file, project_root)

    for ref in refs:
        if "*" in ref:
            # Glob check
            matches = glob.glob(str(project_root / ref), recursive=True)
            if not matches:
                result.warn(
                    f"{rel_room}: glob pattern no longer resolves: {ref}"
                )
        else:
            full = project_root / ref
            if not full.exists():
                result.warn(
                    f"{rel_room}: referenced path does not exist: {ref}"
                )


# ---------------------------------------------------------------------------
# Gap 1A: detect changed rooms
# ---------------------------------------------------------------------------

def get_changed_index_files(project_root: Path) -> list[Path]:
    """Return index files that have uncommitted changes (staged or unstaged).

    git diff HEAD covers both staged and unstaged changes relative to the last commit.
    """
    result = subprocess.run(
        ["git", "diff", "--name-only", "HEAD"],
        capture_output=True, text=True, cwd=project_root,
    )
    changed: list[Path] = []
    for line in result.stdout.splitlines():
        p = project_root / line.strip()
        if "docs/index" in str(p) and p.suffix == ".md" and p.is_file():
            changed.append(p)
    return changed


# ---------------------------------------------------------------------------
# Core validator
# ---------------------------------------------------------------------------

def validate(project_root: Path, changed_rooms_only: bool = False) -> ValidationResult:
    result = ValidationResult()
    index_dir = project_root / "docs" / "index"

    # --- 1. Campus _root.md ---
    campus = index_dir / "_root.md"
    if not campus.is_file():
        result.error("Missing campus map: docs/index/_root.md")
        return result

    campus_text = campus.read_text(encoding="utf-8")
    if "TASK" not in campus_text or "LOAD" not in campus_text:
        result.error("Campus _root.md missing TASK → LOAD table")
    if "Buildings" not in campus_text and "Subdomain" not in campus_text:
        result.warn("Campus _root.md missing Buildings listing")

    # --- Changed-rooms mode: restrict scope ---
    changed_files: set[Path] | None = None
    if changed_rooms_only:
        changed_files = set(get_changed_index_files(project_root))
        if not changed_files:
            result.warn("No changed index files detected — nothing to validate in --changed-rooms mode.")
            return result
        print(f"  Changed rooms detected: {len(changed_files)}")
        for f in sorted(changed_files):
            print(f"    - {f.relative_to(project_root)}")

    # --- 2. Discover subdomains and rooms ---
    subdomains = [
        d for d in index_dir.iterdir()
        if d.is_dir() and not d.name.startswith(".")
    ]

    all_rooms: list[Path] = []

    for sub in subdomains:
        router = sub / "_root.md"
        if not router.is_file():
            result.error(f"Missing building router: {router.relative_to(project_root)}")
            continue

        router_text = router.read_text(encoding="utf-8")
        if "TASK" not in router_text or "LOAD" not in router_text:
            result.warn(
                f"Building router {router.relative_to(project_root)} "
                f"missing TASK → LOAD table"
            )

        referenced = extract_md_links(router)
        for ref in referenced:
            if ref == "_root.md" or ref.endswith("/_root.md"):
                continue
            room_path = sub / ref if "/" not in ref else index_dir / ref
            if room_path.is_file():
                all_rooms.append(room_path)
            else:
                result.error(
                    f"Router {router.relative_to(project_root)} references "
                    f"non-existent room: {ref}"
                )

    for md in index_dir.rglob("*.md"):
        if md.name == "_root.md":
            continue
        if md not in all_rooms:
            all_rooms.append(md)
            result.warn(
                f"Room {md.relative_to(project_root)} exists but is not "
                f"referenced by any building router"
            )

    # --- 3. Validate room files ---
    for room in all_rooms:
        # In changed-rooms mode skip rooms not in the change set
        if changed_files is not None and room not in changed_files:
            continue

        fm = parse_frontmatter(room)
        rel = room.relative_to(project_root)

        if fm is None:
            result.error(f"Room {rel} missing YAML frontmatter")
        else:
            if "room" not in fm:
                result.warn(f"Room {rel} frontmatter missing 'room' field")
            if "see_also" not in fm:
                result.warn(f"Room {rel} frontmatter missing 'see_also' field")

        entries = count_entries(room)
        result.total_rooms += 1
        result.total_entries += entries
        if entries > ENTRY_LIMIT:
            result.warn(
                f"Room {rel} has {entries} entries (limit ~{ENTRY_LIMIT}). "
                f"Consider splitting."
            )

        # Gap 1B: file/glob sanity
        check_file_refs(room, project_root, result)

    # --- 4. Source coverage (skipped in changed-rooms mode) ---
    if not changed_rooms_only:
        excluded_dirs = get_excluded_dirs(project_root)
        source_dirs = find_source_dirs(project_root, excluded_dirs)
        source_dirs -= {"docs", "docs/index"}
        source_dirs = {
            d for d in source_dirs
            if not d.startswith("docs/index")
        }

        if source_dirs:
            covered = extract_source_paths_from_rooms(index_dir)
            uncovered = []
            for sd in sorted(source_dirs):
                normalized_covered = covered | {"" if c == "." else c for c in covered}
                is_covered = any(
                    sd == c or sd.startswith(c + "/") or c.startswith(sd + "/")
                    for c in normalized_covered
                )
                if not is_covered:
                    uncovered.append(sd)

            if uncovered:
                result.warn(
                    f"{len(uncovered)} source directories not covered by any room: "
                    f"{', '.join(uncovered[:10])}"
                    + (f" (+{len(uncovered) - 10} more)" if len(uncovered) > 10 else "")
                )

    return result


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="Validate a LOI index for structural integrity and coverage.",
    )
    parser.add_argument("project_root", help="Path to the project root")
    parser.add_argument(
        "--changed-rooms", action="store_true",
        help="Validate only rooms that have uncommitted git changes",
    )
    parser.add_argument(
        "--ci", action="store_true",
        help="CI mode: full validation, exit 1 on any error (no warnings-only pass)",
    )
    args = parser.parse_args()

    project_root = Path(args.project_root).resolve()
    if not project_root.is_dir():
        print(f"Error: {project_root} is not a directory")
        sys.exit(2)

    mode_label = "changed-rooms" if args.changed_rooms else ("CI" if args.ci else "full")
    print(f"Validating LOI index [{mode_label}]: {project_root / 'docs' / 'index'}")

    result = validate(project_root, changed_rooms_only=args.changed_rooms)
    print(result.report())
    print(f"\n  Summary: {result.total_rooms} rooms validated, {result.total_entries} total entries")

    if not result.ok:
        sys.exit(1)

    # In CI mode, treat warnings as failures too
    if args.ci and result.warnings:
        print(f"\n  CI mode: {len(result.warnings)} warning(s) treated as errors.")
        sys.exit(1)

    sys.exit(0)


if __name__ == "__main__":
    main()
