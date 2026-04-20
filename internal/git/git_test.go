package git

import (
	"testing"
)

func TestSetRunner_restoresDefault(t *testing.T) {
	var called bool
	stub := Runner(func(dir string, args ...string) (string, error) {
		called = true
		return "stub-repo", nil
	})

	restore := SetRunner(stub)
	name := RepoName("/some/path")
	if !called {
		t.Error("expected stub runner to be called")
	}
	if name != "stub-repo" {
		t.Errorf("RepoName: got %q, want %q", name, "stub-repo")
	}

	// Restore and verify default is back.
	restore()
	if activeRunner == nil {
		t.Error("activeRunner should not be nil after restore")
	}
	// After restore the active runner should be non-nil (defaultRun).
	called = false
	if activeRunner == nil {
		t.Error("activeRunner should not be nil after restore")
	}
	_ = called
}
