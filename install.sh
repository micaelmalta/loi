#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/micaelmalta/loi.git"
SKILL_NAME="loi"

# ── helpers ──────────────────────────────────────────────────────────────────

info()  { printf '\033[0;34m  %s\033[0m\n' "$*"; }
ok()    { printf '\033[0;32m✓ %s\033[0m\n' "$*"; }
warn()  { printf '\033[0;33m⚠ %s\033[0m\n' "$*"; }
die()   { printf '\033[0;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

# ── preflight ────────────────────────────────────────────────────────────────

command -v git >/dev/null 2>&1 || die "git is required but not found in PATH"

# ── resolve target ───────────────────────────────────────────────────────────

GLOBAL_DIR="$HOME/.claude/skills/$SKILL_NAME"
LOCAL_DIR="$(pwd)/.claude/skills/$SKILL_NAME"

if [[ -n "${LOI_INSTALL_DIR:-}" ]]; then
  TARGET="$LOI_INSTALL_DIR"
elif [[ -d "$HOME/.claude" ]]; then
  echo ""
  echo "  Where do you want to install LOI?"
  echo "  1) Global — $GLOBAL_DIR  (all projects)"
  echo "  2) Local  — $LOCAL_DIR   (this project only)"
  echo ""
  read -r -p "  Choice [1]: " choice
  choice="${choice:-1}"
  case "$choice" in
    1) TARGET="$GLOBAL_DIR" ;;
    2) TARGET="$LOCAL_DIR" ;;
    *) die "Invalid choice: $choice" ;;
  esac
else
  warn "~/.claude not found — installing globally at $GLOBAL_DIR anyway"
  TARGET="$GLOBAL_DIR"
fi

# ── install or update ────────────────────────────────────────────────────────

echo ""

if [[ -d "$TARGET/.git" ]]; then
  info "LOI already installed at $TARGET — updating..."
  git -C "$TARGET" pull --ff-only
  ok "Updated to $(git -C "$TARGET" rev-parse --short HEAD)"
else
  info "Installing LOI into $TARGET ..."
  mkdir -p "$(dirname "$TARGET")"
  git clone --depth 1 "$REPO" "$TARGET"
  ok "Installed at $TARGET"
fi

# ── done ─────────────────────────────────────────────────────────────────────

echo ""
echo "  Now tell your agent:"
echo ""
echo '    "generate loi"      — build full index for this project'
echo '    "update loi"        — refresh stale rooms after code changes'
echo ""
