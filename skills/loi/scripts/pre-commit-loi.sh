#!/usr/bin/env bash
# LOI Level 7 — Pre-Commit Hook (Option B)
#
# Intercepts commits that modify docs/index/ markdown files and triggers
# AI implementation of intent changes before the commit completes.
#
# Install:
#   cp skills/loi/scripts/pre-commit-loi.sh .git/hooks/pre-commit
#   chmod +x .git/hooks/pre-commit
#
# Or with a hook manager (e.g., husky, lefthook):
#   Add this script to your pre-commit hook chain.
#
# Configuration (environment variables):
#   LOI_WORKER_CMD    — Command to invoke AI worker (default: claude)
#   LOI_INDEX_PATH    — Path to LOI index directory (default: docs/index)
#   LOI_AUTO_STAGE    — Auto-stage generated files (default: true)
#   LOI_SKIP          — Set to "1" to skip this hook entirely

set -euo pipefail

# Skip if explicitly disabled
if [ "${LOI_SKIP:-0}" = "1" ]; then
    exit 0
fi

WORKER_CMD="${LOI_WORKER_CMD:-claude}"
INDEX_PATH="${LOI_INDEX_PATH:-docs/index}"
AUTO_STAGE="${LOI_AUTO_STAGE:-true}"

# Check if any staged files are under docs/index/
STAGED_INDEX_FILES=$(git diff --cached --name-only --diff-filter=ACMR -- "${INDEX_PATH}/" 2>/dev/null | grep '\.md$' || true)

if [ -z "$STAGED_INDEX_FILES" ]; then
    # No index files changed — pass through
    exit 0
fi

echo "[LOI Pre-Commit] Detected staged LOI index changes:"
echo "$STAGED_INDEX_FILES" | while read -r f; do echo "  - $f"; done

# Extract the diff for changed index files
DIFF=$(git diff --cached -- ${INDEX_PATH}/ 2>/dev/null)

# Check if any intent fields (DOES, SYMBOLS, TYPE, etc.) actually changed
if ! echo "$DIFF" | grep -qE '^\+.*(DOES:|SYMBOLS:|TYPE:|INTERFACE:|PATTERNS:)'; then
    echo "[LOI Pre-Commit] No intent fields changed — proceeding with commit."
    exit 0
fi

echo "[LOI Pre-Commit] Intent fields changed. Triggering AI implementation..."

# Build the prompt for the worker
PROMPT="The Architect has updated LOI intent contracts in a pre-commit hook.

Changed files:
${STAGED_INDEX_FILES}

Diff:
\`\`\`
${DIFF}
\`\`\`

Task: Implement the intent changes in the source code. Do NOT create a branch (we are
already in a commit flow). Modify only the source files referenced in the changed entries.
Run the test suite. If tests pass, the changes will be auto-staged into this commit."

# Invoke the AI worker
if ! $WORKER_CMD -p "$PROMPT" 2>&1; then
    echo "[LOI Pre-Commit] Worker failed. Aborting commit."
    echo "[LOI Pre-Commit] Set LOI_SKIP=1 to bypass: LOI_SKIP=1 git commit ..."
    exit 1
fi

# Auto-stage any source files the worker modified
if [ "$AUTO_STAGE" = "true" ]; then
    MODIFIED=$(git diff --name-only 2>/dev/null || true)
    if [ -n "$MODIFIED" ]; then
        echo "[LOI Pre-Commit] Auto-staging modified files:"
        echo "$MODIFIED" | while read -r f; do
            echo "  + $f"
            git add "$f"
        done
    fi
fi

echo "[LOI Pre-Commit] Implementation complete. Proceeding with commit."
exit 0
