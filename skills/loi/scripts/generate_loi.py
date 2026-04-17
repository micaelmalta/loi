#!/usr/bin/env python3
"""Generate LOI room scaffolds from codetect's symbols.db.

Reads AST-parsed symbols from codetect and produces room markdown files
with mechanical fields (SYMBOLS, TYPE, DEPENDS) pre-populated. Semantic
fields (DOES, PATTERNS, USE WHEN, EMITS, CONSUMERS) are left as LLM-FILL
markers for the LLM to complete.

Usage:
    python3 generate_loi.py <project-root> --scaffold
    python3 generate_loi.py <project-root> --scaffold --room payroll_core/paycheck_gen
    python3 generate_loi.py <project-root> --scaffold --dry-run

Requires:
    - codetect index at <project-root>/.codetect/symbols.db
    - For Go projects: go.mod in <project-root>
"""

import argparse
import os
import re
import sqlite3
import sys
from collections import defaultdict
from pathlib import Path

# Maximum DEPENDS entries before truncation
DEPENDS_CAP = 4

# Kinds to extract as function signatures
FUNCTION_KINDS = {"function"}

# Kinds to group (not list individually)
GROUPED_KINDS = {"struct": "Types", "interface": "Interfaces"}

# Kinds to skip entirely
SKIP_KINDS = {"field", "constant", "variable", "receiver", "parameter",
              "package", "packageName", "anonMember", "local",
              "database", "index", "table", "talias"}

# Go stdlib prefixes (no dots in first path segment)
GO_STDLIB_RE = re.compile(r'^[a-z][a-z0-9]*(/[a-z][a-z0-9]*)*$')


def get_module_name(project_root: Path) -> str | None:
    """Extract Go module name from go.mod."""
    gomod = project_root / "go.mod"
    if not gomod.is_file():
        return None
    for line in gomod.read_text().splitlines():
        if line.startswith("module "):
            return line.split(None, 1)[1].strip()
    return None


def parse_go_imports(filepath: Path) -> list[str]:
    """Parse import paths from a Go source file."""
    content = filepath.read_text(errors="replace")
    imports = []

    # Single import: import "path"
    for m in re.finditer(r'import\s+"([^"]+)"', content):
        imports.append(m.group(1))

    # Block import: import ( ... )
    for block in re.finditer(r'import\s*\((.*?)\)', content, re.DOTALL):
        for m in re.finditer(r'"([^"]+)"', block.group(1)):
            imports.append(m.group(1))

    return imports


def classify_imports(imports: list[str], module_name: str | None) -> dict[str, list[str]]:
    """Classify imports as same-repo, external, or stdlib."""
    result: dict[str, list[str]] = {"same_repo": [], "external": []}
    for imp in imports:
        if GO_STDLIB_RE.match(imp):
            continue  # stdlib — skip
        if module_name and imp.startswith(module_name + "/"):
            # Same repo — strip module prefix
            short = imp[len(module_name) + 1:]
            result["same_repo"].append(short)
        elif "." in imp.split("/")[0]:
            # External (has a dot in first segment = domain name)
            result["external"].append(imp)
    return result


def format_depends(classified: dict[str, list[str]]) -> str | None:
    """Format DEPENDS field with cap."""
    all_deps = classified["same_repo"] + classified["external"]
    if not all_deps:
        return None
    if len(all_deps) <= DEPENDS_CAP:
        return ", ".join(all_deps)
    shown = all_deps[:DEPENDS_CAP]
    remaining = len(all_deps) - DEPENDS_CAP
    return ", ".join(shown) + f" (+{remaining} more)"


def read_func_signature_from_source(project_root: Path, filepath: str, line: int) -> str | None:
    """Read the full function signature from source code at the given line."""
    abs_path = project_root / filepath
    if not abs_path.is_file():
        return None
    try:
        source_lines = abs_path.read_text(errors="replace").splitlines()
        if line < 1 or line > len(source_lines):
            return None
        # Read the func line (1-indexed)
        func_line = source_lines[line - 1].strip()
        # Collect continuation lines if signature spans multiple lines
        sig = func_line
        i = line  # next line (0-indexed = line)
        while i < len(source_lines) and "{" not in sig:
            sig += " " + source_lines[i].strip()
            i += 1
        # Strip everything from opening brace onward
        sig = re.sub(r'\s*\{.*', '', sig)
        # Remove 'func ' prefix
        sig = re.sub(r'^func\s+', '', sig)
        return sig
    except Exception:
        return None


def parse_function_signature(name: str, pattern: str, scope: str,
                             project_root: Path | None = None,
                             filepath: str | None = None,
                             line: int | None = None) -> str:
    """Extract a clean function signature, reading source if pattern is truncated."""
    # Try reading the actual source first for full signatures
    if project_root and filepath and line:
        full_sig = read_func_signature_from_source(project_root, filepath, line)
        if full_sig:
            return full_sig

    if not pattern:
        return name

    # pattern looks like: /^func (r *Receiver) Name(args) (returns)/
    sig = pattern.strip("/^$")
    sig = re.sub(r'^func\s+', '', sig)
    sig = re.sub(r'\s*\{.*', '', sig)
    sig = sig.rstrip()
    return sig


def build_symbols_section(symbols: list[dict], project_root: Path | None = None) -> list[str]:
    """Build SYMBOLS lines from codetect symbol records."""
    functions = []
    grouped: dict[str, list[str]] = defaultdict(list)

    for sym in symbols:
        kind = sym["kind"]
        name = sym["name"]

        if kind in SKIP_KINDS:
            continue

        # Skip duplicates with package prefix (codetect sometimes indexes both)
        if "." in name and kind in GROUPED_KINDS:
            continue

        if kind in FUNCTION_KINDS:
            sig = parse_function_signature(
                name, sym.get("pattern", ""), sym.get("scope", ""),
                project_root=project_root,
                filepath=sym.get("path"),
                line=sym.get("line"),
            )
            functions.append(f"- {sig}")
        elif kind in GROUPED_KINDS:
            label = GROUPED_KINDS[kind]
            grouped[label].append(name)

    lines = []
    lines.extend(functions)

    for label, names in sorted(grouped.items()):
        if len(names) <= 3:
            lines.append(f"- {label}: {', '.join(names)}")
        else:
            shown = names[:3]
            remaining = len(names) - 3
            lines.append(f"- {label}: {', '.join(shown)} (+{remaining} more)")

    return lines


def generate_file_entry(filepath: str, symbols: list[dict],
                        project_root: Path, module_name: str | None) -> tuple[str, list[str]]:
    """Generate a single LOI entry for a source file.

    Returns: (entry_text, list_of_same_repo_dep_paths)
    """
    filename = os.path.basename(filepath)
    lines = [f"# {filename}", ""]
    lines.append("<!-- LLM-FILL: DOES -->")
    same_repo_deps: list[str] = []

    # SYMBOLS
    sym_lines = build_symbols_section(symbols, project_root=project_root)
    if sym_lines:
        lines.append("SYMBOLS:")
        lines.extend(sym_lines)

    # DEPENDS (parse from source file)
    abs_path = project_root / filepath
    if abs_path.is_file() and filepath.endswith(".go"):
        imports = parse_go_imports(abs_path)
        classified = classify_imports(imports, module_name)
        same_repo_deps = classified["same_repo"]
        depends = format_depends(classified)
        if depends:
            lines.append(f"DEPENDS: {depends}")

    lines.append("<!-- LLM-FILL: PATTERNS, USE WHEN, EMITS, CONSUMERS -->")
    return "\n".join(lines), same_repo_deps


def build_see_also(room_name: str, all_room_deps: list[str],
                   dep_path_to_room: dict[str, str]) -> list[str]:
    """Infer see_also from DEPENDS cross-references to other rooms."""
    related_rooms: set[str] = set()
    for dep_path in all_room_deps:
        # Find which room contains files under this dep path
        for path_prefix, target_room in dep_path_to_room.items():
            if dep_path.startswith(path_prefix) and target_room != room_name:
                related_rooms.add(target_room)
    return sorted(related_rooms)


def generate_room(room_name: str, file_entries: list[str],
                  see_also: list[str] | None = None) -> str:
    """Generate a complete room markdown file."""
    see_also_str = str(see_also) if see_also else "[]"
    header = f"""---
room: {room_name}
see_also: {see_also_str}
# LLM-FILL: architectural_health, security_tier (set by Committee review)
architectural_health: normal
security_tier: normal
---

# {room_name}
"""
    body = "\n\n---\n\n".join(file_entries)
    return header + "\n" + body + "\n"


def load_existing_rooms(index_dir: Path) -> dict[str, list[str]]:
    """Parse existing room files to find file-to-room assignments.

    Returns: {room_path: [source_file_paths]}
    """
    rooms: dict[str, list[str]] = {}
    for md_file in index_dir.rglob("*.md"):
        if md_file.name == "_root.md":
            continue
        rel = md_file.relative_to(index_dir)
        room_key = str(rel.with_suffix(""))
        files = []
        content = md_file.read_text(errors="replace")
        # Match LOI entry headings: # filename.ext
        for m in re.finditer(r'^# (\S+\.\w+)\s*$', content, re.MULTILINE):
            files.append(m.group(1))
        if files:
            rooms[room_key] = files
    return rooms


def find_source_files_in_db(db_path: Path) -> dict[str, list[dict]]:
    """Query symbols.db and group symbols by file path (deduplicated)."""
    # Use immutable mode so SQLite doesn't need write access for WAL/SHM locks
    uri = f"file:{db_path}?immutable=1"
    conn = sqlite3.connect(uri, uri=True)
    conn.row_factory = sqlite3.Row
    cursor = conn.execute(
        "SELECT name, kind, path, line, pattern, scope, signature "
        "FROM symbols ORDER BY path, line"
    )
    files: dict[str, list[dict]] = defaultdict(list)
    seen: dict[str, set[tuple]] = defaultdict(set)  # path -> set of (normalized_name, kind, line)
    for row in cursor:
        name = row["name"]
        # Normalize: strip package prefix (e.g., "paycheck_gen.calculationDeductions" → "calculationDeductions")
        if "." in name:
            name = name.split(".")[-1]
        key = (name, row["kind"], row["line"])
        path = row["path"]
        if key not in seen[path]:
            seen[path].add(key)
            d = dict(row)
            d["name"] = name  # Use normalized name
            files[path].append(d)
    conn.close()
    return files


def group_files_by_directory(file_paths: list[str]) -> dict[str, list[str]]:
    """Group source files by their parent directory as default rooms."""
    groups: dict[str, list[str]] = defaultdict(list)
    for fp in file_paths:
        parent = os.path.dirname(fp)
        if parent:
            # Use last two path segments as room name
            parts = parent.split("/")
            room_name = "/".join(parts[-2:]) if len(parts) >= 2 else parts[-1]
            groups[room_name].append(fp)
    return groups


def main():
    parser = argparse.ArgumentParser(description="Generate LOI scaffolds from codetect symbols.db")
    parser.add_argument("project_root", type=Path, help="Project root directory")
    parser.add_argument("--scaffold", action="store_true", required=True,
                        help="Generate scaffold room files")
    parser.add_argument("--room", type=str, default=None,
                        help="Generate only a specific room (e.g., payroll_core/paycheck_gen)")
    parser.add_argument("--dry-run", action="store_true",
                        help="Print output instead of writing files")
    args = parser.parse_args()

    project_root = args.project_root.resolve()
    db_path = project_root / ".codetect" / "symbols.db"

    if not db_path.is_file():
        print(f"ERROR: No codetect index found at {db_path}", file=sys.stderr)
        print("Run 'codetect index' in the project first.", file=sys.stderr)
        sys.exit(1)

    module_name = get_module_name(project_root)
    if module_name:
        print(f"Go module: {module_name}")

    # Load all symbols grouped by file
    all_symbols = find_source_files_in_db(db_path)
    print(f"Found {len(all_symbols)} files with symbols in codetect index")

    # Determine room assignments
    index_dir = project_root / "docs" / "index"
    if index_dir.is_dir():
        existing_rooms = load_existing_rooms(index_dir)
        if existing_rooms:
            print(f"Found {len(existing_rooms)} existing rooms in docs/index/")
        else:
            existing_rooms = None
    else:
        existing_rooms = None

    if existing_rooms:
        # Update mode: scaffold entries for files in existing rooms
        rooms_to_generate = existing_rooms
        if args.room:
            if args.room in rooms_to_generate:
                rooms_to_generate = {args.room: rooms_to_generate[args.room]}
            else:
                print(f"ERROR: Room '{args.room}' not found in existing index", file=sys.stderr)
                sys.exit(1)
    else:
        # New index mode: group by directory
        rooms_to_generate = group_files_by_directory(list(all_symbols.keys()))
        if args.room:
            if args.room in rooms_to_generate:
                rooms_to_generate = {args.room: rooms_to_generate[args.room]}
            else:
                print(f"ERROR: Room '{args.room}' not found", file=sys.stderr)
                sys.exit(1)

    # Build basename → [full_path] index for matching
    basename_index: dict[str, list[str]] = defaultdict(list)
    for db_path in all_symbols:
        basename_index[os.path.basename(db_path)].append(db_path)

    # Build dep_path_to_room: maps source directory prefixes to room names
    # Used for inferring see_also from DEPENDS cross-references
    # First, resolve basenames to full DB paths per room
    dep_path_to_room: dict[str, str] = {}
    all_rooms = existing_rooms if existing_rooms else rooms_to_generate
    for rname, rfiles in all_rooms.items():
        for f in rfiles:
            # Resolve basename to full DB path
            basename = os.path.basename(f)
            candidates = basename_index.get(basename, [])
            resolved = candidates[0] if len(candidates) == 1 else f
            dirname = os.path.dirname(resolved)
            if dirname:
                dep_path_to_room[dirname] = rname

    # Generate room files
    for room_name, source_files in sorted(rooms_to_generate.items()):
        entries = []
        all_room_deps: list[str] = []
        for src_file in sorted(source_files):
            # Find matching symbols — try exact match first
            symbols = all_symbols.get(src_file, [])
            db_file_path = src_file

            if not symbols:
                # Match by basename (existing rooms use basenames like "paycheck_generator.go")
                basename = os.path.basename(src_file)
                candidates = basename_index.get(basename, [])
                if len(candidates) == 1:
                    db_file_path = candidates[0]
                    symbols = all_symbols[db_file_path]
                elif len(candidates) > 1:
                    # Multiple files with same basename — try suffix match
                    for c in candidates:
                        if c.endswith(src_file):
                            db_file_path = c
                            symbols = all_symbols[c]
                            break

            if not symbols:
                entries.append(f"# {os.path.basename(src_file)}\n\n<!-- LLM-FILL: DOES -->\n<!-- No symbols found in codetect index -->")
                continue

            entry, file_deps = generate_file_entry(db_file_path, symbols, project_root, module_name)
            entries.append(entry)
            all_room_deps.extend(file_deps)

        # Infer see_also from cross-room dependencies
        see_also = build_see_also(room_name, all_room_deps, dep_path_to_room)
        room_content = generate_room(room_name, entries, see_also=see_also)

        if args.dry_run:
            print(f"\n{'='*60}")
            print(f"ROOM: {room_name}")
            print(f"{'='*60}")
            print(room_content)
        else:
            # Write to docs/index/<room>.md
            out_path = index_dir / f"{room_name}.md"
            out_path.parent.mkdir(parents=True, exist_ok=True)
            out_path.write_text(room_content)
            print(f"  Wrote {out_path.relative_to(project_root)}")

    if not args.dry_run:
        print(f"\nScaffolded {len(rooms_to_generate)} room(s). Run LLM to fill DOES/PATTERNS.")


if __name__ == "__main__":
    main()
