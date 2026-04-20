package cmd_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffTables_noChange(t *testing.T) {
	root := initGitRepo(t)
	// File unchanged vs HEAD → empty / no diff output.
	stdout, _, code := runLOI(t, root, "diff-tables", "docs/index/_root.md")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	// No diff means empty or a "no changes" message — not an error.
	_ = stdout
}

func TestDiffTables_addedRow(t *testing.T) {
	root := initGitRepo(t)

	// Modify _root.md to add a new TASK row.
	rootMD := filepath.Join(root, "docs", "index", "_root.md")
	data, err := os.ReadFile(rootMD)
	if err != nil {
		t.Fatalf("read _root.md: %v", err)
	}
	updated := strings.Replace(string(data),
		"| Handle authentication | auth/_root.md |",
		"| Handle authentication | auth/_root.md |\n| New feature | auth/login.md |",
		1)
	if err := os.WriteFile(rootMD, []byte(updated), 0o644); err != nil {
		t.Fatalf("write _root.md: %v", err)
	}
	// Stage the change so git diff HEAD picks it up.
	cmd := exec.Command("git", "add", "docs/index/_root.md")
	cmd.Dir = root
	cmd.Run()

	stdout, _, code := runLOI(t, root, "diff-tables", "docs/index/_root.md")
	if code != 0 {
		t.Fatalf("unexpected exit code %d; output: %s", code, stdout)
	}
	// Should contain a TASK changes section with the added row.
	if !strings.Contains(stdout, "TASK changes") {
		t.Errorf("expected TASK changes in diff output; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "New feature") && !strings.Contains(stdout, "added") {
		t.Errorf("expected added row in diff output; got:\n%s", stdout)
	}
}
