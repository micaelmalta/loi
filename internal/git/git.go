package git

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes shell commands. Swapped out in tests to avoid real git calls.
type Runner func(dir string, args ...string) (string, error)

// activeRunner is the package-level runner; replaced by SetRunner in tests.
var activeRunner Runner = defaultRun

// SetRunner replaces the active runner and returns a restore function.
// Usage: defer git.SetRunner(fakeRunner)()
func SetRunner(r Runner) func() {
	prev := activeRunner
	activeRunner = r
	return func() { activeRunner = prev }
}

// run delegates to the active runner.
func run(dir string, args ...string) (string, error) {
	return activeRunner(dir, args...)
}

// defaultRun executes the given command in dir and returns trimmed stdout. If the
// command fails the error message includes both the original error and any
// output written to stderr.
func defaultRun(dir string, args ...string) (string, error) {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return "", fmt.Errorf("%s: %w\n%s", strings.Join(args, " "), err, stderr)
		}
		return "", fmt.Errorf("%s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Root returns the absolute path to the repository root by running
// git rev-parse --show-toplevel in the current working directory.
func Root() (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return "", fmt.Errorf("git rev-parse --show-toplevel: %w\n%s", err, stderr)
		}
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// RepoName returns the short repository name derived from the remote origin
// URL. If the remote cannot be queried it falls back to the base name of
// projectRoot.
func RepoName(projectRoot string) string {
	url, err := run(projectRoot, "git", "remote", "get-url", "origin")
	if err != nil {
		return filepath.Base(projectRoot)
	}
	// Strip a trailing .git suffix then take the last path segment.
	url = strings.TrimSuffix(url, ".git")
	// Handle both HTTPS and SSH remote formats.
	// SSH:   git@github.com:org/repo
	// HTTPS: https://github.com/org/repo
	if idx := strings.LastIndexAny(url, "/:"); idx >= 0 {
		url = url[idx+1:]
	}
	if url == "" {
		return filepath.Base(projectRoot)
	}
	return url
}

// Branch returns the name of the currently checked-out branch. Returns
// "unknown" if it cannot be determined.
func Branch(projectRoot string) string {
	branch, err := run(projectRoot, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "unknown"
	}
	return branch
}

// Diff returns the unified diff of path against HEAD. Returns an empty string
// when the path is clean.
func Diff(projectRoot, path string) (string, error) {
	out, err := run(projectRoot, "git", "diff", "HEAD", "--", path)
	if err != nil {
		return "", err
	}
	return out, nil
}

// Show returns the content of filepath at the given git ref.
func Show(projectRoot, ref, filePath string) (string, error) {
	arg := ref + ":" + filePath
	out, err := run(projectRoot, "git", "show", arg)
	if err != nil {
		return "", err
	}
	return out, nil
}

// DiffNameOnly returns the list of file names reported by git diff --name-only.
// Any additional arguments (e.g. a commit range) are appended to the command.
func DiffNameOnly(projectRoot string, extraArgs ...string) ([]string, error) {
	args := append([]string{"git", "diff", "--name-only"}, extraArgs...)
	out, err := run(projectRoot, args...)
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// StagedFiles returns the list of staged files (added, copied, modified,
// renamed, or type-changed) from the git index.
func StagedFiles(projectRoot string) ([]string, error) {
	out, err := run(projectRoot, "git", "diff", "--cached", "--name-only", "--diff-filter=ACMRT")
	if err != nil {
		return nil, err
	}
	return splitLines(out), nil
}

// CreatePR creates a GitHub pull request using the gh CLI and returns the PR
// URL. Pass draft=true to create a draft pull request.
func CreatePR(projectRoot, branch, title, body string, draft bool) (string, error) {
	args := []string{"gh", "pr", "create", "--title", title, "--body", body}
	if draft {
		args = append(args, "--draft")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = projectRoot
	out, err := cmd.Output()
	if err != nil {
		var stderr string
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if stderr != "" {
			return "", fmt.Errorf("gh pr create: %w\n%s", err, stderr)
		}
		return "", fmt.Errorf("gh pr create: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CheckoutNewBranch creates and checks out a new branch.
func CheckoutNewBranch(projectRoot, branch string) error {
	_, err := run(projectRoot, "git", "checkout", "-b", branch)
	return err
}

// AddAndCommit stages the given files and creates a commit with the provided
// message.
func AddAndCommit(projectRoot string, files []string, message string) error {
	addArgs := append([]string{"git", "add", "--"}, files...)
	if _, err := run(projectRoot, addArgs...); err != nil {
		return fmt.Errorf("git add: %w", err)
	}
	if _, err := run(projectRoot, "git", "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}
	return nil
}

// Push pushes branch to origin and sets the upstream tracking reference.
func Push(projectRoot, branch string) error {
	_, err := run(projectRoot, "git", "push", "-u", "origin", branch)
	return err
}

// CurrentBranch returns the name of the currently checked-out branch.
func CurrentBranch(projectRoot string) (string, error) {
	return run(projectRoot, "git", "rev-parse", "--abbrev-ref", "HEAD")
}

// splitLines splits a block of text into non-empty lines.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}
