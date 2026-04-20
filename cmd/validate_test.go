package cmd_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_cleanIndex_exits0(t *testing.T) {
	root := initGitRepo(t)
	_, _, code := runLOI(t, root, "validate")
	if code != 0 {
		t.Errorf("expected exit 0 for valid index, got %d", code)
	}
}

func TestValidate_missingRootMD_exits1(t *testing.T) {
	root := initGitRepo(t)
	// Remove _root.md
	if err := os.Remove(filepath.Join(root, "docs", "index", "_root.md")); err != nil {
		t.Fatalf("remove _root.md: %v", err)
	}
	_, _, code := runLOI(t, root, "validate")
	if code == 0 {
		t.Error("expected non-zero exit when _root.md is missing")
	}
}

func TestValidate_ciMode_warningsPromotedToErrors(t *testing.T) {
	// The valid fixture has no warnings, so --ci should still exit 0.
	root := initGitRepo(t)
	_, _, code := runLOI(t, root, "validate", "--ci")
	if code != 0 {
		t.Errorf("expected exit 0 for valid index with --ci, got %d", code)
	}
}

func TestValidate_changedRoomsMode(t *testing.T) {
	root := initGitRepo(t)
	// --changed-rooms with no uncommitted changes should exit 0 (nothing to check).
	_, _, code := runLOI(t, root, "validate", "--changed-rooms")
	if code != 0 {
		t.Errorf("expected exit 0 for --changed-rooms with no changes, got %d", code)
	}
}
