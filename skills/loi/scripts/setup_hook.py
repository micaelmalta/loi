#!/usr/bin/env python3
"""LOI Hook Installer — install LOI git hooks into a target repository.

Usage:
    python3 setup_hook.py <project-root>
    python3 setup_hook.py <project-root> --mode pre-push
    python3 setup_hook.py <project-root> --mode pre-push --force

The hook is sourced from skills/loi/hooks/<mode>.sample relative to this script.
"""

import argparse
import shutil
import stat
import sys
from pathlib import Path


SUPPORTED_HOOKS = {
    "pre-push": "pre-push.sample",
}


def find_hooks_dir() -> Path:
    """Return the hooks/ directory co-located with this skill."""
    return Path(__file__).resolve().parent.parent / "hooks"


def install_hook(project_root: Path, mode: str, force: bool) -> int:
    sample_name = SUPPORTED_HOOKS.get(mode)
    if not sample_name:
        print(f"Error: unsupported hook mode '{mode}'. Supported: {', '.join(SUPPORTED_HOOKS)}")
        return 2

    source = find_hooks_dir() / sample_name
    if not source.is_file():
        print(f"Error: hook template not found at {source}")
        return 2

    git_hooks_dir = project_root / ".git" / "hooks"
    if not git_hooks_dir.is_dir():
        print(f"Error: {git_hooks_dir} does not exist — is {project_root} a git repository?")
        return 2

    dest = git_hooks_dir / mode

    if dest.exists() and not force:
        print(f"[LOI setup-hook] Hook already exists: {dest}")
        print("  Use --force to overwrite.")
        return 1

    shutil.copy2(source, dest)
    # Ensure executable bit is set
    dest.chmod(dest.stat().st_mode | stat.S_IEXEC | stat.S_IXGRP | stat.S_IXOTH)

    print(f"[LOI setup-hook] Installed {mode} hook → {dest}")
    print(f"  Source:  {source}")
    print(f"  Effect:  runs validate_loi.py --changed-rooms before every push.")
    print(f"           Push is blocked if any changed index room has broken references.")

    _ensure_gitignore(project_root)
    return 0


def _ensure_gitignore(project_root: Path) -> None:
    """Add .loi-claims.json* to .gitignore if not already present."""
    gitignore = project_root / ".gitignore"
    entries = [".loi-claims.json\n", ".loi-claims.json.lock\n"]

    existing = gitignore.read_text(encoding="utf-8") if gitignore.is_file() else ""

    to_add = [e for e in entries if e.strip() not in existing.splitlines()]
    if not to_add:
        return

    separator = "" if existing.endswith("\n") or not existing else "\n"
    with gitignore.open("a", encoding="utf-8") as f:
        f.write(separator + "# LOI runtime state (ephemeral, never commit)\n")
        for entry in to_add:
            f.write(entry)
    print(f"[LOI setup-hook] Added to .gitignore: {', '.join(e.strip() for e in to_add)}")


def main():
    parser = argparse.ArgumentParser(
        description="LOI Hook Installer — install LOI git hooks into a repository",
    )
    parser.add_argument("project_root", help="Path to the git repository root")
    parser.add_argument(
        "--mode", choices=list(SUPPORTED_HOOKS), default="pre-push",
        help="Which hook to install (default: pre-push)",
    )
    parser.add_argument(
        "--force", action="store_true",
        help="Overwrite an existing hook at the destination",
    )
    args = parser.parse_args()

    project_root = Path(args.project_root).resolve()
    if not project_root.is_dir():
        print(f"Error: {project_root} is not a directory")
        sys.exit(2)

    sys.exit(install_hook(project_root, args.mode, args.force))


if __name__ == "__main__":
    main()
