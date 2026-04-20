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

var validatePatternsLevel int

var validatePatternsCmd = &cobra.Command{
	Use:   "validate-patterns",
	Short: "Validate PATTERN table entries against their target rooms",
	Long: `Validate-patterns checks every PATTERN table row in all _root.md files under
docs/index/ and verifies that the declared pattern phrase is semantically
supported by the content of its target room.

Level 1 (default): checks that a normalized form of the pattern appears in
the target room body.

Level 2: additionally checks pattern_aliases frontmatter (alias-only support
warning) and last_validated metadata (stale validation warning if >14 days).

Exit codes:
  0 — no errors
  1 — one or more errors found`,
	RunE: runValidatePatterns,
}

func init() {
	validatePatternsCmd.Flags().IntVar(&validatePatternsLevel, "level", 1, "Validation level: 1 (default) or 2 (extended)")
	rootCmd.AddCommand(validatePatternsCmd)
}

func runValidatePatterns(cmd *cobra.Command, args []string) error {
	indexDir := filepath.Join(projectRoot, "docs", "index")

	if _, err := os.Stat(indexDir); os.IsNotExist(err) {
		return fmt.Errorf("docs/index/ not found under %s", projectRoot)
	}

	type patternError struct {
		rootFile string
		pattern  string
		target   string
		msg      string
		isWarn   bool
	}

	var errs []patternError
	var warns []patternError

	// Walk all _root.md files under docs/index/.
	walkErr := filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() || d.Name() != "_root.md" {
			return nil
		}

		data, readErr := os.ReadFile(path)
		if readErr != nil {
			warns = append(warns, patternError{
				rootFile: path,
				msg:      fmt.Sprintf("read _root.md: %v", readErr),
				isWarn:   true,
			})
			return nil
		}

		rows := index.ExtractPatternRows(string(data))
		if len(rows) == 0 {
			return nil
		}

		for _, row := range rows {
			targetPath := resolvePatternTarget(indexDir, path, row.TargetPath)
			if targetPath == "" {
				errs = append(errs, patternError{
					rootFile: path,
					pattern:  row.Pattern,
					target:   row.TargetPath,
					msg:      fmt.Sprintf("missing target room: %s", row.TargetPath),
				})
				continue
			}

			roomData, roomErr := os.ReadFile(targetPath)
			if roomErr != nil {
				errs = append(errs, patternError{
					rootFile: path,
					pattern:  row.Pattern,
					target:   row.TargetPath,
					msg:      fmt.Sprintf("cannot read target room %s: %v", targetPath, roomErr),
				})
				continue
			}
			roomBody := string(roomData)

			normPattern := index.Normalize(row.Pattern)
			normBody := index.Normalize(roomBody)

			if !strings.Contains(normBody, normPattern) {
				errs = append(errs, patternError{
					rootFile: path,
					pattern:  row.Pattern,
					target:   row.TargetPath,
					msg:      fmt.Sprintf("weak semantic support: pattern %q not found in room body %s", row.Pattern, row.TargetPath),
				})
			}

			if validatePatternsLevel < 2 {
				continue
			}

			// Level 2: alias-only support check.
			fm, fmErr := index.ParseFrontmatter(targetPath)
			if fmErr == nil && fm != nil {
				for _, alias := range fm.PatternAliases {
					if strings.Contains(normBody, index.Normalize(alias)) {
						warns = append(warns, patternError{
							rootFile: path,
							pattern:  row.Pattern,
							target:   row.TargetPath,
							msg:      fmt.Sprintf("alias-only support: alias %q found in room body but not main pattern phrase", alias),
							isWarn:   true,
						})
					}
				}
			}

			// Level 2: stale validation check.
			meta, metaErr := index.ParsePatternMetadataBlock(targetPath)
			if metaErr == nil {
				key := index.Normalize(row.Pattern)
				if pm, ok := meta[key]; ok && pm.LastValidated != "" {
					// Try ISO date parse: 2006-01-02 or RFC3339.
					validatedAt, parseErr := parseFlexibleDate(pm.LastValidated)
					if parseErr == nil {
						age := time.Since(validatedAt)
						if age > 14*24*time.Hour {
							warns = append(warns, patternError{
								rootFile: path,
								pattern:  row.Pattern,
								target:   row.TargetPath,
								msg: fmt.Sprintf("stale validation: last_validated %s is %d days ago (>14)",
									pm.LastValidated, int(age.Hours()/24)),
								isWarn: true,
							})
						}
					}
				}
			}
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk index: %w", walkErr)
	}

	// Print warnings.
	for _, w := range warns {
		rel := relOrAbs(projectRoot, w.rootFile)
		if w.pattern != "" {
			fmt.Fprintf(os.Stderr, "WARN: [%s] pattern %q -> %s: %s\n", rel, w.pattern, w.target, w.msg)
		} else {
			fmt.Fprintf(os.Stderr, "WARN: [%s] %s\n", rel, w.msg)
		}
	}

	// Print errors.
	for _, e := range errs {
		rel := relOrAbs(projectRoot, e.rootFile)
		fmt.Fprintf(os.Stderr, "ERROR: [%s] pattern %q -> %s: %s\n", rel, e.pattern, e.target, e.msg)
	}

	// Summary.
	fmt.Printf("Pattern validation: %d errors, %d warnings\n", len(errs), len(warns))

	if len(errs) > 0 {
		os.Exit(1)
	}
	return nil
}

// resolvePatternTarget resolves the target path of a pattern row to an
// absolute filesystem path. It tries two locations:
//  1. indexDir/targetPath
//  2. filepath.Dir(rootmd)/targetPath
//
// Returns "" if neither resolves to an existing file.
func resolvePatternTarget(indexDir, rootmd, targetPath string) string {
	candidates := []string{
		filepath.Join(indexDir, targetPath),
		filepath.Join(filepath.Dir(rootmd), targetPath),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// parseFlexibleDate tries to parse s as "2006-01-02" or time.RFC3339.
func parseFlexibleDate(s string) (time.Time, error) {
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}
