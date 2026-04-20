package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// stageFile stages a file modification in the temp git repo.
func stageFile(t *testing.T, root, relPath, content string) {
	t.Helper()
	path := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cmd := exec.Command("git", "add", relPath)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
}

func TestCheckStale_noSourceFilesStaged_exits0(t *testing.T) {
	root := initGitRepo(t)
	// Stage only a non-source file — check-stale should exit 0.
	stageFile(t, root, "README.md", "# Updated readme\n")
	_, _, code := runLOI(t, root, "check-stale")
	if code != 0 {
		t.Errorf("expected exit 0 when no source files staged, got %d", code)
	}
}

func TestCheckStale_staleIndex_exits1(t *testing.T) {
	root := initGitRepo(t)
	// Stage only the source file — no corresponding index update.
	stageFile(t, root, "internal/auth/login.go", "package auth\nfunc Login() {}\n// updated\n")
	_, _, code := runLOI(t, root, "check-stale")
	if code == 0 {
		t.Error("expected non-zero exit when source staged but index not updated")
	}
}

func TestCheckStale_LOI_STALE_BLOCK_0_exits0(t *testing.T) {
	root := initGitRepo(t)
	// Stale but LOI_STALE_BLOCK=0 should demote to warning, exit 0.
	stageFile(t, root, "internal/auth/login.go", "package auth\nfunc Login() {}\n// updated\n")
	cmd := exec.Command(loiBin, "check-stale")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "LOI_STALE_BLOCK=0")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() != 0 {
				t.Errorf("expected exit 0 with LOI_STALE_BLOCK=0, got %d", exitErr.ExitCode())
			}
		}
	}
}
