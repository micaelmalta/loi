package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
)

// projectRoot holds the absolute path to the repository root, resolved during
// PersistentPreRunE and made available to all sub-commands.
var projectRoot string

var rootCmd = &cobra.Command{
	Use:   "loi",
	Short: "LOI – Library of Intent CLI",
	Long: `loi – Library of Intent CLI

The loi binary is the canonical entry-point for all LOI operations.
It replaces the collection of Python scripts that previously managed index
generation, validation, git-hook wiring, and governance workflows.

Run 'loi help <command>' for detailed usage of any sub-command.`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		root, err := gitRepoRoot()
		if err != nil {
			cwd, wdErr := os.Getwd()
			if wdErr != nil {
				return fmt.Errorf("cannot determine working directory: %w", wdErr)
			}
			projectRoot = cwd
			return nil
		}
		projectRoot = root
		return nil
	},
}

// Execute is the entry-point called by main. It runs the root command and
// exits with a non-zero status code on error.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// gitRepoRoot runs git rev-parse --show-toplevel and returns the trimmed
// result, or an error if the current directory is not inside a git repository.
func gitRepoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
