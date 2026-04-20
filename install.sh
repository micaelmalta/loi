#!/usr/bin/env bash
set -euo pipefail

REPO="https://github.com/micaelmalta/loi.git"
SKILL_NAME="loi"
BIN_DIR="${LOI_BIN_DIR:-$HOME/.local/bin}"

# ── helpers ──────────────────────────────────────────────────────────────────

info()  { printf '\033[0;34m  %s\033[0m\n' "$*"; }
ok()    { printf '\033[0;32m✓ %s\033[0m\n' "$*"; }
warn()  { printf '\033[0;33m⚠ %s\033[0m\n' "$*"; }
die()   { printf '\033[0;31m✗ %s\033[0m\n' "$*" >&2; exit 1; }

# ── preflight ────────────────────────────────────────────────────────────────

command -v git >/dev/null 2>&1 || die "git is required but not found in PATH"

# ── resolve skill target ─────────────────────────────────────────────────────

GLOBAL_DIR="$HOME/.claude/skills/$SKILL_NAME"
LOCAL_DIR="$(pwd)/.claude/skills/$SKILL_NAME"

if [[ -n "${LOI_INSTALL_DIR:-}" ]]; then
  TARGET="$LOI_INSTALL_DIR"
elif [[ -d "$HOME/.claude" ]]; then
  echo ""
  echo "  Where do you want to install the LOI skill?"
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

# ── install or update skill ───────────────────────────────────────────────────

echo ""

if [[ -d "$TARGET/.git" ]]; then
  info "LOI skill already installed at $TARGET — updating..."
  git -C "$TARGET" pull --ff-only
  ok "Updated to $(git -C "$TARGET" rev-parse --short HEAD)"
  CLONE_DIR="$TARGET"
else
  info "Installing LOI skill into $TARGET ..."
  mkdir -p "$(dirname "$TARGET")"
  git clone --depth 1 "$REPO" "$TARGET"
  ok "Skill installed at $TARGET"
  CLONE_DIR="$TARGET"
fi

# ── build and install the loi binary ─────────────────────────────────────────

echo ""

if command -v go >/dev/null 2>&1; then
  info "Building loi binary..."
  mkdir -p "$BIN_DIR"
  (cd "$CLONE_DIR" && go build -o "$BIN_DIR/loi" .)
  ok "loi binary installed at $BIN_DIR/loi"

  # Remind the user to add BIN_DIR to PATH if needed
  if ! echo "$PATH" | tr ':' '\n' | grep -qx "$BIN_DIR"; then
    warn "$BIN_DIR is not in your PATH"
    echo ""
    echo "  Add this to your shell profile (~/.zshrc, ~/.bashrc, etc.):"
    echo ""
    echo "    export PATH=\"\$PATH:$BIN_DIR\""
    echo ""
  fi
else
  warn "Go not found — skipping binary build"
  echo ""
  echo "  To build the binary later:"
  echo ""
  echo "    cd $CLONE_DIR && go build -o $BIN_DIR/loi ."
  echo ""
  echo "  Or install Go from https://go.dev/dl/ and re-run this script."
  echo ""
fi

# ── done ─────────────────────────────────────────────────────────────────────

echo ""
echo "  Now tell your agent:"
echo ""
echo '    "generate loi"      — build full index for this project'
echo '    "update loi"        — refresh stale rooms after code changes'
echo ""
echo "  Or use the CLI directly:"
echo ""
echo "    loi validate        — check index structure"
echo "    loi watch           — start the background watcher"
echo "    loi --help          — full command reference"
echo ""
