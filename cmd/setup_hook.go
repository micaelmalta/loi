package cmd

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed hooks
var hookFS embed.FS

var (
	setupHookMode  string
	setupHookForce bool
)

var setupHookCmd = &cobra.Command{
	Use:   "setup-hook",
	Short: "Install LOI git hooks into .git/hooks/",
	Long: `Install LOI git hooks that enforce index validation and stale-check
policies at commit/push time.

Modes:
  pre-push           installs .git/hooks/pre-push
  pre-commit-stale   installs .git/hooks/pre-commit
  all                installs both hooks

Use --force to overwrite an existing hook file.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		switch setupHookMode {
		case "pre-push":
			return runInstallHook(projectRoot, "pre-push", "hooks/pre-push.sample", setupHookForce)
		case "pre-commit-stale":
			return runInstallHook(projectRoot, "pre-commit", "hooks/pre-commit-stale.sample", setupHookForce)
		case "all":
			if err := runInstallHook(projectRoot, "pre-push", "hooks/pre-push.sample", setupHookForce); err != nil {
				return err
			}
			return runInstallHook(projectRoot, "pre-commit", "hooks/pre-commit-stale.sample", setupHookForce)
		default:
			return fmt.Errorf("unknown --mode %q; choose pre-push, pre-commit-stale, or all", setupHookMode)
		}
	},
}

// runInstallHook reads the named template from the embedded FS, writes it to
// .git/hooks/<hookName>, marks it executable, and updates .gitignore.
func runInstallHook(projectRoot, hookName, templatePath string, force bool) error {
	content, err := fs.ReadFile(hookFS, templatePath)
	if err != nil {
		return fmt.Errorf("reading embedded template %s: %w", templatePath, err)
	}

	dest := filepath.Join(projectRoot, ".git", "hooks", hookName)

	if !force {
		if _, statErr := os.Stat(dest); statErr == nil {
			fmt.Fprintf(os.Stderr,
				"hook already exists: %s\nUse --force to overwrite.\n", dest)
			return fmt.Errorf("hook already exists: %s", dest)
		}
	}

	if err := os.WriteFile(dest, content, 0644); err != nil {
		return fmt.Errorf("writing hook %s: %w", dest, err)
	}
	if err := os.Chmod(dest, 0755); err != nil {
		return fmt.Errorf("chmod hook %s: %w", dest, err)
	}

	if err := ensureGitignore(projectRoot); err != nil {
		// Non-fatal: the hook was installed; just warn.
		fmt.Fprintf(os.Stderr, "warning: could not update .gitignore: %v\n", err)
	}

	effect := hookEffectDescription(hookName)
	fmt.Printf("Installed: %s\n  Effect: %s\n", dest, effect)
	return nil
}

// hookEffectDescription returns a human-readable description of what the
// installed hook does.
func hookEffectDescription(hookName string) string {
	switch hookName {
	case "pre-push":
		return "runs 'loi validate --changed-rooms' and 'loi governance' before every push"
	case "pre-commit":
		return "runs 'loi check-stale' before every commit"
	default:
		return fmt.Sprintf("runs LOI checks for %s", hookName)
	}
}

// ensureGitignore appends .loi-claims.json and .loi-claims.json.lock to
// projectRoot/.gitignore if they are not already present.
func ensureGitignore(projectRoot string) error {
	gitignorePath := filepath.Join(projectRoot, ".gitignore")

	entries := []string{".loi-claims.json", ".loi-claims.json.lock"}

	// Read existing lines to check what is already present.
	existing := map[string]bool{}
	f, err := os.Open(gitignorePath)
	if err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			existing[strings.TrimSpace(scanner.Text())] = true
		}
		f.Close()
		if scanErr := scanner.Err(); scanErr != nil {
			return fmt.Errorf("reading .gitignore: %w", scanErr)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("opening .gitignore: %w", err)
	}

	var toAdd []string
	for _, entry := range entries {
		if !existing[entry] {
			toAdd = append(toAdd, entry)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	// Append missing entries (create file if absent).
	out, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("opening .gitignore for append: %w", err)
	}
	defer out.Close()

	// Ensure we start on a new line.
	if len(existing) > 0 {
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "# LOI claims files (local-only, not to be committed)")
	for _, entry := range toAdd {
		fmt.Fprintln(out, entry)
	}
	return nil
}

func init() {
	setupHookCmd.Flags().StringVar(&setupHookMode, "mode", "all",
		"Which hook(s) to install: pre-push, pre-commit-stale, or all")
	setupHookCmd.Flags().BoolVar(&setupHookForce, "force", false,
		"Overwrite an existing hook file")
	rootCmd.AddCommand(setupHookCmd)
}
