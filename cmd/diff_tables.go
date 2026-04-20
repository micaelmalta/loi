package cmd

import (
	"fmt"
	"os"

	"github.com/micaelmalta/loi/internal/git"
	"github.com/micaelmalta/loi/internal/index"
	"github.com/spf13/cobra"
)

var (
	diffTablesFrom string
	diffTablesTo   string
)

var diffTablesCmd = &cobra.Command{
	Use:   "diff-tables <filepath>",
	Short: "Show semantic table diff for a room file between two git refs",
	Long: `Diff-tables shows how the TASK, PATTERN, and GOVERNANCE tables in a room .md
file have changed between two git refs (default: HEAD~1 to HEAD).

The filepath argument is a path to a room .md file, relative to the project root.

Output uses +/- prefix notation similar to git diff:
  + added:   row that appeared in the new version
  - removed: row that disappeared from the old version
  ~ changed: row whose non-key cells changed

Exit code is always 0. Empty output means no table changes.`,
	Args: cobra.ExactArgs(1),
	RunE: runDiffTables,
}

func init() {
	diffTablesCmd.Flags().StringVar(&diffTablesFrom, "from", "HEAD~1", "Starting git ref (default HEAD~1)")
	diffTablesCmd.Flags().StringVar(&diffTablesTo, "to", "HEAD", "Ending git ref (default HEAD)")
	rootCmd.AddCommand(diffTablesCmd)
}

func runDiffTables(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	oldText, err := git.Show(projectRoot, diffTablesFrom, filePath)
	if err != nil {
		// If the file didn't exist at fromRef treat it as empty.
		oldText = ""
	}

	newText, err := git.Show(projectRoot, diffTablesTo, filePath)
	if err != nil {
		// If the file doesn't exist at toRef treat it as empty.
		newText = ""
	}

	oldTables := index.ParseTables(oldText)
	newTables := index.ParseTables(newText)

	diff := index.DiffTables(oldTables, newTables)
	output := index.FormatDiff(diff)

	if output != "" {
		fmt.Fprintln(os.Stdout, output)
	}
	return nil
}
