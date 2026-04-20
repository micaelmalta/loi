package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/micaelmalta/loi/internal/index"
	"github.com/spf13/cobra"
)

var (
	proposalsTargetRoom    string
	proposalsGraderVersion string
	proposalsFailureReason string
	proposalsValidate      bool
)

// proposalRecord pairs a file path with its parsed proposal metadata.
type proposalRecord struct {
	path string
	meta map[string]interface{}
}

var proposalsCmd = &cobra.Command{
	Use:   "proposals",
	Short: "List or validate LOI proposal files",
	Long: `Proposals lists or validates proposal .md files found under docs/index/proposals/
and any other *proposal*.md files under docs/index/.

List mode (default): prints a table of proposal_id, generated_at, target_room,
and failure_reason for each proposal. Optional filters narrow the output.

Validate mode (--validate): checks that required fields are present and that
generated_at is a valid RFC3339 timestamp. Exits 1 if errors are found.`,
	RunE: runProposals,
}

func init() {
	proposalsCmd.Flags().StringVar(&proposalsTargetRoom, "target-room", "", "Filter by target room (substring match)")
	proposalsCmd.Flags().StringVar(&proposalsGraderVersion, "grader-version", "", "Filter by grader_version (exact match)")
	proposalsCmd.Flags().StringVar(&proposalsFailureReason, "failure-reason", "", "Filter by failure_reason (substring match)")
	proposalsCmd.Flags().BoolVar(&proposalsValidate, "validate", false, "Validate proposal files instead of listing them")
	rootCmd.AddCommand(proposalsCmd)
}

func runProposals(cmd *cobra.Command, args []string) error {
	indexDir := filepath.Join(projectRoot, "docs", "index")

	files, err := findProposalFiles(indexDir)
	if err != nil {
		return fmt.Errorf("find proposal files: %w", err)
	}

	var records []proposalRecord

	for _, f := range files {
		meta, parseErr := index.ParseProposalMetadata(f)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "WARN: %s: parse proposal metadata: %v\n", relOrAbs(projectRoot, f), parseErr)
			continue
		}
		if meta == nil {
			fmt.Fprintf(os.Stderr, "WARN: %s: no proposal_metadata block found\n", relOrAbs(projectRoot, f))
			meta = make(map[string]interface{})
		}
		records = append(records, proposalRecord{path: f, meta: meta})
	}

	// Apply filters.
	filtered := records[:0]
	for _, r := range records {
		if proposalsTargetRoom != "" {
			tr, _ := r.meta["target_room"].(string)
			if !strings.Contains(tr, proposalsTargetRoom) {
				continue
			}
		}
		if proposalsGraderVersion != "" {
			gv, _ := r.meta["grader_version"].(string)
			if gv != proposalsGraderVersion {
				continue
			}
		}
		if proposalsFailureReason != "" {
			fr, _ := r.meta["failure_reason"].(string)
			if !strings.Contains(fr, proposalsFailureReason) {
				continue
			}
		}
		filtered = append(filtered, r)
	}

	if proposalsValidate {
		return runProposalsValidate(filtered)
	}

	printProposalsTable(filtered)
	return nil
}

// findProposalFiles returns proposal .md file paths.
// It includes docs/index/proposals/*.md and any *proposal*.md under docs/index/.
func findProposalFiles(indexDir string) ([]string, error) {
	seen := make(map[string]bool)
	var files []string

	// docs/index/proposals/*.md
	proposalsDir := filepath.Join(indexDir, "proposals")
	if _, err := os.Stat(proposalsDir); err == nil {
		entries, err := os.ReadDir(proposalsDir)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() && filepath.Ext(e.Name()) == ".md" {
					p := filepath.Join(proposalsDir, e.Name())
					if !seen[p] {
						seen[p] = true
						files = append(files, p)
					}
				}
			}
		}
	}

	// Any *proposal*.md under docs/index/ (recursive).
	walkErr := filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) == ".md" && strings.Contains(d.Name(), "proposal") {
			if !seen[path] {
				seen[path] = true
				files = append(files, path)
			}
		}
		return nil
	})
	return files, walkErr
}

var proposalRequiredFields = []string{"proposal_id", "generated_at", "source_run_id", "target_room"}

func runProposalsValidate(records []proposalRecord) error {
	type validationError struct {
		path string
		msg  string
	}
	var errs []validationError

	for _, r := range records {
		rel := relOrAbs(projectRoot, r.path)
		for _, field := range proposalRequiredFields {
			val, ok := r.meta[field]
			if !ok || val == "" {
				errs = append(errs, validationError{
					path: rel,
					msg:  fmt.Sprintf("missing required field: %s", field),
				})
			}
		}
		// Parse generated_at as RFC3339.
		if ga, ok := r.meta["generated_at"].(string); ok && ga != "" {
			if _, parseErr := time.Parse(time.RFC3339, ga); parseErr != nil {
				errs = append(errs, validationError{
					path: rel,
					msg:  fmt.Sprintf("generated_at %q is not a valid RFC3339 timestamp: %v", ga, parseErr),
				})
			}
		}
	}

	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "ERROR: %s: %s\n", e.path, e.msg)
	}

	fmt.Printf("Proposal validation: %d files checked, %d errors\n", len(records), len(errs))

	if len(errs) > 0 {
		os.Exit(1)
	}
	return nil
}

func printProposalsTable(records []proposalRecord) {
	if len(records) == 0 {
		fmt.Println("No proposals found.")
		return
	}

	// Column widths.
	wID, wGenAt, wTarget, wFail := 11, 12, 11, 14
	for _, r := range records {
		id, _ := r.meta["proposal_id"].(string)
		ga, _ := r.meta["generated_at"].(string)
		tr, _ := r.meta["target_room"].(string)
		fr, _ := r.meta["failure_reason"].(string)
		if len(id) > wID {
			wID = len(id)
		}
		if len(ga) > wGenAt {
			wGenAt = len(ga)
		}
		if len(tr) > wTarget {
			wTarget = len(tr)
		}
		if len(fr) > wFail {
			wFail = len(fr)
		}
	}

	format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%s\n", wID, wGenAt, wTarget)
	sep := fmt.Sprintf("%s  %s  %s  %s",
		strings.Repeat("-", wID),
		strings.Repeat("-", wGenAt),
		strings.Repeat("-", wTarget),
		strings.Repeat("-", wFail),
	)

	fmt.Printf(format, "PROPOSAL_ID", "GENERATED_AT", "TARGET_ROOM", "FAILURE_REASON")
	fmt.Println(sep)
	for _, r := range records {
		id, _ := r.meta["proposal_id"].(string)
		ga, _ := r.meta["generated_at"].(string)
		tr, _ := r.meta["target_room"].(string)
		fr, _ := r.meta["failure_reason"].(string)
		fmt.Printf(format, id, ga, tr, fr)
	}
	fmt.Printf("\n%d proposals\n", len(records))
}
