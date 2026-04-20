package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/micaelmalta/loi/internal/git"
	"github.com/micaelmalta/loi/internal/index"
	"github.com/spf13/cobra"
)

var checkStaleCmd = &cobra.Command{
	Use:   "check-stale",
	Short: "Detect stale LOI index rooms when source files are staged",
	Long: `Check-stale inspects the git staging area. For each staged source file it finds
LOI room .md files that declare coverage over that path via "Source paths:" lines.

If any covering room is not itself staged (i.e. the intent index has not been
updated alongside the source change), it prints a warning listing those rooms.

By default check-stale exits 1 when stale rooms are found. Set the environment
variable LOI_STALE_BLOCK=0 to demote the exit code to 0 (warning-only mode).

This command is designed to run as a pre-commit git hook.`,
	RunE: runCheckStale,
}

func init() {
	rootCmd.AddCommand(checkStaleCmd)
}

func runCheckStale(cmd *cobra.Command, args []string) error {
	staged, err := git.StagedFiles(projectRoot)
	if err != nil {
		return fmt.Errorf("staged files: %w", err)
	}

	if len(staged) == 0 {
		return nil
	}

	indexPrefix := filepath.Join("docs", "index")

	// Partition staged files.
	stagedIndexSet := make(map[string]bool)
	var sourceFiles []string

	for _, f := range staged {
		if isUnderIndexDir(f, indexPrefix) {
			// Store as absolute path for set membership checks.
			stagedIndexSet[filepath.Join(projectRoot, f)] = true
		} else if index.SourceExts[strings.ToLower(filepath.Ext(f))] {
			sourceFiles = append(sourceFiles, f)
		}
	}

	if len(sourceFiles) == 0 {
		return nil
	}

	// For each source file find covering rooms not yet staged.
	type staleEntry struct {
		sourceFile  string
		coveringRoom string
	}
	var staleEntries []staleEntry

	for _, sf := range sourceFiles {
		covering, findErr := index.FindCoveringRooms(projectRoot, sf, index.CoverBySourcePaths)
		if findErr != nil {
			fmt.Fprintf(os.Stderr, "WARN: find covering rooms for %s: %v\n", sf, findErr)
			continue
		}
		for _, room := range covering {
			// Normalize to absolute path for set lookup.
			absRoom := room
			if !filepath.IsAbs(room) {
				absRoom = filepath.Join(projectRoot, room)
			}
			if !stagedIndexSet[absRoom] {
				staleEntries = append(staleEntries, staleEntry{
					sourceFile:  sf,
					coveringRoom: relOrAbs(projectRoot, absRoom),
				})
			}
		}
	}

	if len(staleEntries) == 0 {
		return nil
	}

	fmt.Fprintln(os.Stderr, "LOI STALE INDEX WARNING")
	fmt.Fprintln(os.Stderr, "The following LOI room files cover staged source files but are not staged:")
	fmt.Fprintln(os.Stderr)
	for _, e := range staleEntries {
		fmt.Fprintf(os.Stderr, "  source: %s\n  room:   %s\n\n", e.sourceFile, e.coveringRoom)
	}
	fmt.Fprintln(os.Stderr, "Update and stage the room files, or set LOI_STALE_BLOCK=0 to skip this check.")

	if os.Getenv("LOI_STALE_BLOCK") == "0" {
		return nil
	}
	os.Exit(1)
	return nil
}

// isUnderIndexDir returns true if the file path is under the docs/index directory.
// Accepts both slash-separated and OS-native paths.
func isUnderIndexDir(file, indexPrefix string) bool {
	clean := filepath.ToSlash(file)
	prefix := filepath.ToSlash(indexPrefix)
	return strings.HasPrefix(clean, prefix+"/") || clean == prefix
}
