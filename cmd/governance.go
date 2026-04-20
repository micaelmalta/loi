package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/micaelmalta/loi/internal/index"
	"github.com/spf13/cobra"
)

var (
	governanceSecurityFilter string
	governanceHealthFilter   string
	governanceFormat         string
	governanceVerbose        bool
)

var secSeverity = map[string]int{"normal": 0, "high": 1, "sensitive": 2}
var healthSeverity = map[string]int{"normal": 0, "warning": 1, "critical": 2}

var governanceCmd = &cobra.Command{
	Use:   "governance [roots...]",
	Short: "Show governance watchlist entries across one or more roots",
	Long: `Governance aggregates GOVERNANCE WATCHLIST table entries from all
_root.md files under docs/index/ in the given roots (default: project root).

It also promotes individual room files whose frontmatter declares non-normal
health or security values to the watchlist if not already present.

Entries are sorted by combined severity (security + health) descending.

Exit code is always 0.`,
	RunE: runGovernance,
}

func init() {
	governanceCmd.Flags().StringVar(&governanceSecurityFilter, "security", "", "Filter by security tier (normal|high|sensitive)")
	governanceCmd.Flags().StringVar(&governanceHealthFilter, "health", "", "Filter by health status (normal|warning|critical)")
	governanceCmd.Flags().StringVar(&governanceFormat, "format", "text", "Output format: text or json")
	governanceCmd.Flags().BoolVar(&governanceVerbose, "verbose", false, "Include source file path in output")
	rootCmd.AddCommand(governanceCmd)
}

func runGovernance(cmd *cobra.Command, args []string) error {
	roots := args
	if len(roots) == 0 {
		roots = []string{projectRoot}
	}

	var all []index.GovEntry

	for _, root := range roots {
		indexDir := filepath.Join(root, "docs", "index")
		if _, err := os.Stat(indexDir); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "WARN: docs/index/ not found under %s\n", root)
			continue
		}

		entries, err := collectGovernanceEntries(root, indexDir)
		if err != nil {
			return fmt.Errorf("collect governance entries for %s: %w", root, err)
		}
		all = append(all, entries...)
	}

	// Sort by combined severity descending (security + health).
	sort.Slice(all, func(i, j int) bool {
		ri := secSeverity[all[i].Security] + healthSeverity[all[i].Health]
		rj := secSeverity[all[j].Security] + healthSeverity[all[j].Health]
		if ri != rj {
			return ri > rj
		}
		return all[i].Room < all[j].Room
	})

	// Apply filters.
	filtered := all[:0]
	for _, e := range all {
		if governanceSecurityFilter != "" && e.Security != governanceSecurityFilter {
			continue
		}
		if governanceHealthFilter != "" && e.Health != governanceHealthFilter {
			continue
		}
		filtered = append(filtered, e)
	}

	switch governanceFormat {
	case "json":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(filtered); err != nil {
			return fmt.Errorf("json encode: %w", err)
		}
	default:
		printGovernanceTable(filtered)
	}

	return nil
}

// collectGovernanceEntries gathers all governance entries from _root.md watchlist
// tables and from individual room frontmatter flags.
func collectGovernanceEntries(root, indexDir string) ([]index.GovEntry, error) {
	var entries []index.GovEntry
	watchlistRooms := make(map[string]bool) // rooms already in a watchlist (by suffix)

	// First pass: collect from _root.md governance tables.
	walkErr := filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || d.Name() != "_root.md" {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		rowEntries := index.ParseGovernanceTable(string(data))
		for i := range rowEntries {
			rowEntries[i].SourceFile = path
			rel, relErr := filepath.Rel(root, path)
			if relErr == nil {
				rowEntries[i].SourceFile = rel
			}
			entries = append(entries, rowEntries[i])
			watchlistRooms[rowEntries[i].Room] = true
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	// Second pass: scan individual room .md files for elevated frontmatter flags.
	walkErr = filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || d.Name() == "_root.md" || filepath.Ext(d.Name()) != ".md" {
			return nil
		}

		fm, fmErr := index.ParseFrontmatter(path)
		if fmErr != nil || fm == nil {
			return nil
		}

		health := strings.ToLower(fm.ArchitecturalHealth)
		security := strings.ToLower(fm.SecurityTier)

		if health == "" {
			health = "normal"
		}
		if security == "" {
			security = "normal"
		}

		// Only promote if non-normal.
		if health == "normal" && security == "normal" {
			return nil
		}

		// Check if already covered by a watchlist entry (suffix match).
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		for existing := range watchlistRooms {
			if strings.HasSuffix(rel, existing) || strings.HasSuffix(existing, rel) {
				return nil
			}
		}

		entries = append(entries, index.GovEntry{
			Room:       rel,
			Health:     health,
			Security:   security,
			Note:       fm.CommitteeNotes,
			SourceFile: rel,
		})
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	return entries, nil
}

// printGovernanceTable writes an aligned text table to stdout.
func printGovernanceTable(entries []index.GovEntry) {
	if len(entries) == 0 {
		fmt.Println("No governance entries found.")
		return
	}

	// Compute column widths.
	wRoom, wHealth, wSec, wNote := 4, 6, 8, 4 // minimums for header labels
	for _, e := range entries {
		if len(e.Room) > wRoom {
			wRoom = len(e.Room)
		}
		if len(e.Health) > wHealth {
			wHealth = len(e.Health)
		}
		if len(e.Security) > wSec {
			wSec = len(e.Security)
		}
		if len(e.Note) > wNote {
			wNote = len(e.Note)
		}
	}

	format := fmt.Sprintf("%%-%ds  %%-%ds  %%-%ds  %%s\n", wRoom, wHealth, wSec)
	sep := fmt.Sprintf("%s  %s  %s  %s",
		strings.Repeat("-", wRoom),
		strings.Repeat("-", wHealth),
		strings.Repeat("-", wSec),
		strings.Repeat("-", wNote),
	)

	fmt.Printf(format, "ROOM", "HEALTH", "SECURITY", "NOTE")
	fmt.Println(sep)
	for _, e := range entries {
		fmt.Printf(format, e.Room, e.Health, e.Security, e.Note)
	}
	fmt.Printf("\n%d entries\n", len(entries))
}
