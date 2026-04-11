#!/usr/bin/env python3
"""LOI Proposal Provenance — validate, query, and summarise proposal files.

Each proposal file should begin with a machine-readable provenance block:

    proposal_metadata:
      proposal_id: proposal-2026-04-10T08-00-00Z-refusal-context
      generated_at: 2026-04-10T08:00:00Z
      source_run_id: eval-run-2026-04-10T07:55:00Z
      source_run_file: runs/cd-eval-2026-04-10T07-55-00Z.jsonl
      grader_version: v2.3
      failure_reason: REFUSAL_CONTEXT
      failure_count: 3
      target_room: evals-corpus/prompts.md
      source_operator_rules:
        - test before shipping
        - skill robustness

Usage:
    python3 proposals.py <project-root>
    python3 proposals.py <project-root> --target-room auth/ucan.md
    python3 proposals.py <project-root> --grader-version v2.3
    python3 proposals.py <project-root> --failure-reason REFUSAL_CONTEXT
    python3 proposals.py <project-root> --validate
"""

import argparse
import re
import sys
from datetime import datetime
from pathlib import Path

# ---------------------------------------------------------------------------
# Frontmatter parser
# ---------------------------------------------------------------------------

def parse_proposal_metadata(filepath: Path) -> dict | None:
    """Extract the proposal_metadata block from a proposal markdown file.

    Returns a flat dict of metadata fields, or None if the block is absent.
    """
    try:
        text = filepath.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None

    # Match `proposal_metadata:` block (YAML-ish, not inside ```fences```)
    m = re.search(r"^proposal_metadata\s*:\s*\n((?:[ \t]+.*\n?)*)", text, re.MULTILINE)
    if not m:
        return None

    block = m.group(1)
    meta: dict = {}
    current_list_key: str | None = None
    current_list: list | None = None

    for line in block.splitlines():
        list_item = re.match(r"^\s+-\s+(.*)", line)
        if list_item and current_list_key and current_list is not None:
            current_list.append(list_item.group(1).strip())
            continue

        kv = re.match(r"^\s+(\w[\w_-]*):\s*(.*)", line)
        if kv:
            if current_list_key and current_list is not None:
                meta[current_list_key] = current_list
            current_list_key = None
            current_list = None

            key = kv.group(1)
            val = kv.group(2).strip().strip('"').strip("'")
            if val == "" or val == "[]":
                current_list_key = key
                current_list = []
            else:
                meta[key] = val
        elif not line.strip():
            if current_list_key and current_list is not None:
                meta[current_list_key] = current_list
            current_list_key = None
            current_list = None

    if current_list_key and current_list is not None:
        meta[current_list_key] = current_list

    return meta or None


REQUIRED_FIELDS = [
    "proposal_id",
    "generated_at",
    "source_run_id",
    "target_room",
]

OPTIONAL_FIELDS = [
    "source_run_file",
    "grader_version",
    "failure_reason",
    "failure_count",
    "source_operator_rules",
]


# ---------------------------------------------------------------------------
# Proposal discovery
# ---------------------------------------------------------------------------

def find_proposal_files(project_root: Path) -> list[Path]:
    """Find all proposal markdown files under docs/index/proposals/ or similar."""
    index_dir = project_root / "docs" / "index"
    proposals: list[Path] = []

    # Check canonical proposals directory
    proposals_dir = index_dir / "proposals"
    if proposals_dir.is_dir():
        proposals.extend(proposals_dir.rglob("*.md"))

    # Also scan all index rooms for files named *proposal* or *proposals*
    for md in index_dir.rglob("*.md"):
        if "proposal" in md.name.lower() and md not in proposals:
            proposals.append(md)

    return proposals


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

def cmd_list(project_root: Path, args) -> int:
    proposals = find_proposal_files(project_root)
    if not proposals:
        print("[LOI proposals] No proposal files found under docs/index/proposals/")
        return 0

    results = []
    for p in proposals:
        meta = parse_proposal_metadata(p)
        if meta is None:
            meta = {}
        results.append((p, meta))

    # Filter
    if args.target_room:
        results = [(p, m) for p, m in results if args.target_room in m.get("target_room", "")]
    if args.grader_version:
        results = [(p, m) for p, m in results if m.get("grader_version") == args.grader_version]
    if args.failure_reason:
        results = [(p, m) for p, m in results if args.failure_reason in m.get("failure_reason", "")]

    if not results:
        print("[LOI proposals] No proposals match the filters.")
        return 0

    print(f"[LOI proposals] {len(results)} proposal(s):\n")
    for p, meta in results:
        rel = p.relative_to(project_root)
        pid = meta.get("proposal_id", "—")
        target = meta.get("target_room", "—")
        gen_at = meta.get("generated_at", "—")
        reason = meta.get("failure_reason", "")
        grader = meta.get("grader_version", "")
        count = meta.get("failure_count", "")
        run_id = meta.get("source_run_id", "—")

        print(f"  {rel}")
        print(f"    id:           {pid}")
        print(f"    target_room:  {target}")
        print(f"    generated_at: {gen_at}")
        if run_id != "—":
            print(f"    run_id:       {run_id}")
        if grader:
            print(f"    grader:       {grader}")
        if reason:
            print(f"    failure:      {reason}" + (f" (x{count})" if count else ""))
        if not meta:
            print(f"    [WARNING: no proposal_metadata block found]")
        print()

    return 0


def cmd_validate(project_root: Path) -> int:
    proposals = find_proposal_files(project_root)
    if not proposals:
        print("[LOI proposals] No proposal files found.")
        return 0

    errors = 0
    warnings = 0

    for p in proposals:
        rel = p.relative_to(project_root)
        meta = parse_proposal_metadata(p)

        if meta is None:
            print(f"  MISSING  {rel} — no proposal_metadata block")
            errors += 1
            continue

        missing = [f for f in REQUIRED_FIELDS if f not in meta]
        if missing:
            print(f"  INVALID  {rel} — missing required fields: {', '.join(missing)}")
            errors += 1
            continue

        # Validate generated_at is parseable
        try:
            datetime.fromisoformat(meta["generated_at"].replace("Z", "+00:00"))
        except ValueError:
            print(f"  WARN     {rel} — generated_at is not valid ISO 8601: {meta['generated_at']}")
            warnings += 1

        missing_optional = [f for f in OPTIONAL_FIELDS if f not in meta]
        if missing_optional:
            print(f"  PARTIAL  {rel} — missing optional fields: {', '.join(missing_optional)}")
            warnings += 1
        else:
            print(f"  OK       {rel}")

    print(f"\n  Summary: {len(proposals)} proposals, {errors} errors, {warnings} warnings")
    return 1 if errors else 0


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description="LOI Proposal Provenance — query and validate proposal metadata",
    )
    parser.add_argument("project_root", help="Path to the project root")
    parser.add_argument("--target-room", help="Filter by target_room value")
    parser.add_argument("--grader-version", help="Filter by grader_version value")
    parser.add_argument("--failure-reason", help="Filter by failure_reason value")
    parser.add_argument(
        "--validate", action="store_true",
        help="Validate all proposal files for required metadata fields",
    )
    args = parser.parse_args()

    project_root = Path(args.project_root).resolve()
    if not project_root.is_dir():
        print(f"Error: {project_root} is not a directory")
        sys.exit(2)

    if args.validate:
        sys.exit(cmd_validate(project_root))
    else:
        sys.exit(cmd_list(project_root, args))


if __name__ == "__main__":
    main()
