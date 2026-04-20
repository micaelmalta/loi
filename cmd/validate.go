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

// ValidationResult holds the outcome of a validate run.
type ValidationResult struct {
	Errors       []string
	Warnings     []string
	TotalRooms   int
	TotalEntries int
}

var (
	validateChangedRooms bool
	validateCI           bool
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the LOI index structure and room files",
	Long: `Validate checks the docs/index/ hierarchy for structural issues.

It verifies campus and building _root.md files, checks room frontmatter,
entry counts, file references, and optionally source coverage.

Exit codes:
  0 — no errors (warnings may be present)
  1 — errors found, or warnings found in --ci mode`,
	RunE: runValidate,
}

func init() {
	validateCmd.Flags().BoolVar(&validateChangedRooms, "changed-rooms", false, "Only validate rooms changed in HEAD")
	validateCmd.Flags().BoolVar(&validateCI, "ci", false, "Treat warnings as errors (exit 1 on any warning)")
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	result := &ValidationResult{}

	indexDir := filepath.Join(projectRoot, "docs", "index")
	campusRoot := filepath.Join(indexDir, "_root.md")

	// Step 1: campus _root.md must exist.
	if _, err := os.Stat(campusRoot); os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, "ERROR: docs/index/_root.md not found")
		os.Exit(1)
	} else if err != nil {
		return fmt.Errorf("stat %s: %w", campusRoot, err)
	}

	// Step 2: campus body checks.
	campusData, err := os.ReadFile(campusRoot)
	if err != nil {
		return fmt.Errorf("read campus root: %w", err)
	}
	campusText := string(campusData)
	if !strings.Contains(campusText, "TASK") {
		result.Warnings = append(result.Warnings, "campus _root.md: missing TASK section")
	}
	if !strings.Contains(campusText, "LOAD") {
		result.Warnings = append(result.Warnings, "campus _root.md: missing LOAD section")
	}
	if !strings.Contains(campusText, "Buildings") && !strings.Contains(campusText, "Subdomain") {
		result.Warnings = append(result.Warnings, "campus _root.md: missing 'Buildings' or 'Subdomain' section")
	}

	// Step 3: collect changed rooms list (if --changed-rooms).
	var changedSet map[string]bool
	if validateChangedRooms {
		changed, diffErr := git.DiffNameOnly(projectRoot, "HEAD")
		if diffErr != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("git diff failed: %v", diffErr))
		}
		var indexMDs []string
		for _, f := range changed {
			if strings.Contains(f, "docs/index") && strings.HasSuffix(f, ".md") {
				indexMDs = append(indexMDs, f)
			}
		}
		if len(indexMDs) == 0 {
			fmt.Fprintln(os.Stderr, "WARN: --changed-rooms: no changed docs/index .md files in HEAD")
			printValidationResult(result, validateCI)
			return nil
		}
		changedSet = make(map[string]bool, len(indexMDs))
		for _, f := range indexMDs {
			changedSet[filepath.Join(projectRoot, f)] = true
		}
	}

	// Step 4: walk buildings (non-hidden subdirs of docs/index/).
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		return fmt.Errorf("readdir %s: %w", indexDir, err)
	}

	// allLinked accumulates all .md paths referenced from any router.
	allLinked := make(map[string]bool)
	allLinked[campusRoot] = true

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		buildingDir := filepath.Join(indexDir, e.Name())
		buildingRoot := filepath.Join(buildingDir, "_root.md")

		allLinked[buildingRoot] = true

		// Building _root.md must exist.
		if _, statErr := os.Stat(buildingRoot); os.IsNotExist(statErr) {
			result.Errors = append(result.Errors,
				fmt.Sprintf("building %s: missing _root.md", e.Name()))
			continue
		} else if statErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("building %s: stat _root.md: %v", e.Name(), statErr))
			continue
		}

		// Count non-_root room files in building.
		roomFiles, listErr := listRoomFiles(buildingDir)
		if listErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("building %s: list rooms: %v", e.Name(), listErr))
		} else if len(roomFiles) == 1 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("building %s: only 1 room file — consider promoting to flat layout", e.Name()))
		}

		// Building router must have TASK + LOAD.
		routerData, readErr := os.ReadFile(buildingRoot)
		if readErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("building %s: read _root.md: %v", e.Name(), readErr))
		} else {
			rt := string(routerData)
			if !strings.Contains(rt, "TASK") {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("building %s/_root.md: missing TASK section", e.Name()))
			}
			if !strings.Contains(rt, "LOAD") {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("building %s/_root.md: missing LOAD section", e.Name()))
			}
		}

		// Collect .md links from router.
		links, linkErr := index.ExtractMDLinks(buildingRoot)
		if linkErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("building %s: extract links: %v", e.Name(), linkErr))
		}
		for _, lnk := range links {
			// Resolve relative to building dir.
			resolved := lnk
			if !filepath.IsAbs(lnk) {
				resolved = filepath.Join(buildingDir, lnk)
			}
			allLinked[resolved] = true
		}
	}

	// Step 5: collect all .md files under docs/index/ and warn about unreferenced ones.
	allMDs, err := collectMDs(indexDir)
	if err != nil {
		return fmt.Errorf("collect md files: %w", err)
	}
	for _, md := range allMDs {
		if !allLinked[md] {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("unreferenced index file: %s", relOrAbs(projectRoot, md)))
		}
	}

	// Step 6: validate each room file.
	for _, md := range allMDs {
		// Skip _root.md files — those are router files, not rooms.
		if filepath.Base(md) == "_root.md" {
			continue
		}
		// If --changed-rooms: skip rooms not in the changed set.
		if validateChangedRooms && !changedSet[md] {
			continue
		}

		result.TotalRooms++

		// Parse frontmatter.
		fm, fmErr := index.ParseFrontmatter(md)
		if fmErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: parse frontmatter: %v", relOrAbs(projectRoot, md), fmErr))
		} else if fm == nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: missing frontmatter", relOrAbs(projectRoot, md)))
		} else {
			if fm.Room == "" {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: frontmatter missing 'room' field", relOrAbs(projectRoot, md)))
			}
			if len(fm.SeeAlso) == 0 {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: frontmatter missing 'see_also' field", relOrAbs(projectRoot, md)))
			}
		}

		// Entry count.
		count, countErr := index.CountEntries(md)
		if countErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: count entries: %v", relOrAbs(projectRoot, md), countErr))
		} else {
			result.TotalEntries += count
			if count > 150 {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("%s: %d entries exceeds 150 — consider splitting", relOrAbs(projectRoot, md), count))
			}
		}

		// File reference checks.
		refs, refErr := index.ExtractTaskFileRefs(md)
		if refErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("%s: extract file refs: %v", relOrAbs(projectRoot, md), refErr))
		} else {
			for _, ref := range refs {
				// Resolve relative to project root.
				pattern := filepath.Join(projectRoot, ref)
				matches, globErr := filepath.Glob(pattern)
				if globErr != nil {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s: invalid glob %q: %v", relOrAbs(projectRoot, md), ref, globErr))
				} else if len(matches) == 0 {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s: file ref not found: %s", relOrAbs(projectRoot, md), ref))
				}
			}
		}
	}

	// Step 7: source coverage check (only when not --changed-rooms).
	if !validateChangedRooms {
		excluded, gitignoreErr := index.ParseGitignoreDirs(projectRoot)
		if gitignoreErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("parse .gitignore: %v", gitignoreErr))
			excluded = map[string]bool{}
		}
		// Add standard exclusions.
		for _, x := range []string{"vendor", "node_modules", "dist", "build", ".git", "docs"} {
			excluded[x] = true
		}

		sourceDirs, sdErr := index.FindSourceDirs(projectRoot, excluded)
		if sdErr != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("find source dirs: %v", sdErr))
		} else {
			coveredPaths, covErr := index.ExtractSourcePathsFromRooms(indexDir)
			if covErr != nil {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("extract source paths from rooms: %v", covErr))
			} else {
				coveredSet := make(map[string]bool, len(coveredPaths))
				for _, p := range coveredPaths {
					coveredSet[p] = true
				}
				for _, sd := range sourceDirs {
					if !coveredSet[sd] && !isCoveredByPrefix(sd, coveredSet) {
						result.Warnings = append(result.Warnings,
							fmt.Sprintf("source dir not covered by any room: %s", sd))
					}
				}
			}
		}
	}

	printValidationResult(result, validateCI)

	hasErrors := len(result.Errors) > 0
	hasWarnings := len(result.Warnings) > 0
	if hasErrors || (validateCI && hasWarnings) {
		os.Exit(1)
	}
	return nil
}

// listRoomFiles returns non-_root.md .md files directly in dir (non-recursive).
func listRoomFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var rooms []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" || e.Name() == "_root.md" {
			continue
		}
		rooms = append(rooms, filepath.Join(dir, e.Name()))
	}
	return rooms, nil
}

// collectMDs recursively collects all .md files under dir.
func collectMDs(dir string) ([]string, error) {
	var mds []string
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && filepath.Ext(d.Name()) == ".md" {
			mds = append(mds, path)
		}
		return nil
	})
	return mds, err
}

// isCoveredByPrefix returns true if sd is covered by any prefix in covered.
func isCoveredByPrefix(sd string, covered map[string]bool) bool {
	for p := range covered {
		if sd == p || strings.HasPrefix(sd, p+"/") {
			return true
		}
	}
	return false
}

// relOrAbs returns the path relative to root, falling back to the absolute path.
func relOrAbs(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// printValidationResult writes the validation report to stdout/stderr.
func printValidationResult(r *ValidationResult, ci bool) {
	for _, e := range r.Errors {
		fmt.Fprintf(os.Stderr, "ERROR: %s\n", e)
	}
	for _, w := range r.Warnings {
		if ci {
			fmt.Fprintf(os.Stderr, "WARN(ci): %s\n", w)
		} else {
			fmt.Fprintf(os.Stderr, "WARN: %s\n", w)
		}
	}

	status := "OK"
	if len(r.Errors) > 0 {
		status = "FAIL"
	} else if len(r.Warnings) > 0 && ci {
		status = "FAIL"
	} else if len(r.Warnings) > 0 {
		status = "WARN"
	}

	fmt.Printf("Validation result: %s\n", status)
	fmt.Printf("  Rooms checked:   %d\n", r.TotalRooms)
	fmt.Printf("  Total entries:   %d\n", r.TotalEntries)
	fmt.Printf("  Errors:          %d\n", len(r.Errors))
	fmt.Printf("  Warnings:        %d\n", len(r.Warnings))
}
