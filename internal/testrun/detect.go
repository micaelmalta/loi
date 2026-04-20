package testrun

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const runTimeout = 300 * time.Second

// DetectAndRun runs the project test suite.
//
// testCmd: explicit command string (split on whitespace and exec'd directly).
// If empty: auto-detect pytest → python3 -m pytest → go test → npm test.
// Returns (passed, output).
// Returns (true, "no test runner detected") if nothing is found.
// Timeout: 300 seconds.
func DetectAndRun(projectRoot, testCmd string) (bool, string) {
	if testCmd != "" {
		return runArgs(projectRoot, strings.Fields(testCmd))
	}

	// Auto-detection order mirrors watcher.py.

	// 1. pytest in PATH
	if _, err := exec.LookPath("pytest"); err == nil {
		return runArgs(projectRoot, []string{"pytest", "--tb=short", "-q"})
	}

	// 2. python3 -m pytest
	if _, err := exec.LookPath("python3"); err == nil {
		return runArgs(projectRoot, []string{"python3", "-m", "pytest", "--tb=short", "-q"})
	}

	// 3. go test (requires go.mod in projectRoot)
	goMod := filepath.Join(projectRoot, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		if _, err := exec.LookPath("go"); err == nil {
			return runArgs(projectRoot, []string{"go", "test", "./..."})
		}
	}

	// 4. npm test (requires package.json in projectRoot)
	pkgJSON := filepath.Join(projectRoot, "package.json")
	if _, err := os.Stat(pkgJSON); err == nil {
		if _, err := exec.LookPath("npm"); err == nil {
			return runArgs(projectRoot, []string{"npm", "test", "--", "--watchAll=false"})
		}
	}

	return true, "no test runner detected"
}

// runArgs executes args[0] with args[1:] in projectRoot under a 300-second
// timeout, returning (exit0, combined output).
func runArgs(projectRoot string, args []string) (bool, string) {
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = projectRoot

	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	err := cmd.Run()
	output := strings.TrimSpace(buf.String())

	if err != nil {
		if ctx.Err() != nil {
			return false, "test run timed out after 300s\n" + output
		}
		return false, output
	}
	return true, output
}
