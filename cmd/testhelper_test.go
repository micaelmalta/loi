package cmd_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// loiBin is the path to the compiled loi binary, set by TestMain.
var loiBin string

func TestMain(m *testing.M) {
	// Build the loi binary into a temp dir.
	bin, err := buildBinary()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: could not build loi binary: %v\n", err)
		os.Exit(1)
	}
	loiBin = bin
	defer os.Remove(bin)
	os.Exit(m.Run())
}

// buildBinary compiles the module root into a temp binary.
func buildBinary() (string, error) {
	dir, err := os.MkdirTemp("", "loi-test-bin-*")
	if err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "loi")

	// Resolve module root (parent of cmd/).
	moduleRoot, err := filepath.Abs("..")
	if err != nil {
		return "", err
	}

	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = moduleRoot
	var out bytes.Buffer
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build: %w\n%s", err, out.String())
	}
	return bin, nil
}

// runLOI runs the loi binary with args in dir, returning stdout, stderr, exit code.
func runLOI(t *testing.T, dir string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(loiBin, args...)
	cmd.Dir = dir
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected exec error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// initGitRepo creates a temp git repo with the valid_index fixture tree
// and returns its root path.
func initGitRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")

	// Copy fixture tree.
	fixture, err := filepath.Abs(filepath.Join("testdata", "valid_index"))
	if err != nil {
		t.Fatalf("fixture abs: %v", err)
	}
	copyDir(t, fixture, root)

	run("git", "add", ".")
	run("git", "commit", "-m", "init")
	return root
}

// copyDir recursively copies src to dst.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copyDir %s → %s: %v", src, dst, err)
	}
}
