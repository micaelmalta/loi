package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/micaelmalta/loi/internal/codetect"
	"github.com/spf13/cobra"
)

var (
	generateScaffold bool
	generateRoom     string
	generateDryRun   bool
)

var generateCmd = &cobra.Command{
	Use:   "generate",
	Short: "Generate or scaffold LOI room index files from codetect symbols",
	Long: `Generate LOI room index files from the codetect symbols database.

Requires .codetect/symbols.db to exist in the project root (populated by the
codetect tool).  Room markdown files are written to docs/index/<room>.md.

Use --dry-run to preview output on stdout without writing files.
Use --room NAME to limit generation to a single room.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !generateScaffold {
			return fmt.Errorf("--scaffold is required")
		}
		return runGenerate(projectRoot, generateRoom, generateDryRun)
	},
}

func init() {
	generateCmd.Flags().BoolVar(&generateScaffold, "scaffold", false,
		"Required flag: scaffold LOI room files from codetect symbols")
	generateCmd.Flags().StringVar(&generateRoom, "room", "",
		"Limit generation to this room name")
	generateCmd.Flags().BoolVar(&generateDryRun, "dry-run", false,
		"Print generated output to stdout instead of writing files")
	rootCmd.AddCommand(generateCmd)
}

// runGenerate is the entry-point called from the cobra command.
func runGenerate(projectRoot, roomFilter string, dryRun bool) error {
	// 1. Verify symbols.db exists.
	dbPath := filepath.Join(projectRoot, ".codetect", "symbols.db")
	if _, err := os.Stat(dbPath); err != nil {
		return fmt.Errorf("symbols.db not found at %s — run codetect first", dbPath)
	}

	// 2. Open DB and query symbols.
	db, err := codetect.OpenDB(dbPath)
	if err != nil {
		return fmt.Errorf("opening symbols.db: %w", err)
	}
	defer db.Close()

	symbolsByFile, err := codetect.QuerySymbols(db)
	if err != nil {
		return fmt.Errorf("querying symbols: %w", err)
	}

	// 3. Get module name.
	moduleName := codetect.GetModuleName(projectRoot)

	// 4. Load existing room files to understand current room → file assignments.
	indexDir := filepath.Join(projectRoot, "docs", "index")
	existingRooms, err := loadExistingRooms(indexDir)
	if err != nil {
		// Non-fatal: if docs/index doesn't exist yet, generate from scratch.
		existingRooms = make(map[string][]string)
	}

	// 5. Build a basename index from symbolsByFile.
	// map[basename][]fullPath — used to look up symbols when existing rooms
	// reference files by basename.
	basenameIndex := make(map[string][]string)
	for path := range symbolsByFile {
		base := filepath.Base(path)
		basenameIndex[base] = append(basenameIndex[base], path)
	}

	// 6. Determine the set of rooms to generate.
	// If existing rooms are found, use them; otherwise group files by directory.
	if len(existingRooms) == 0 {
		// Fall back to directory grouping.
		var allFiles []string
		for path := range symbolsByFile {
			allFiles = append(allFiles, path)
		}
		existingRooms = codetect.GroupFilesByDirectory(allFiles)
	}

	// Apply --room filter.
	if roomFilter != "" {
		filtered := map[string][]string{roomFilter: existingRooms[roomFilter]}
		if len(filtered[roomFilter]) == 0 {
			return fmt.Errorf("room %q not found in index; available: %s",
				roomFilter, roomList(existingRooms))
		}
		existingRooms = filtered
	}

	// 7. Build dep → room mapping for see_also resolution.
	depPathToRoom := buildDepToRoomMap(existingRooms, symbolsByFile, projectRoot, moduleName, basenameIndex)

	// 8. Generate each room.
	for roomName, roomFiles := range existingRooms {
		var fileEntries []string
		var allRoomDeps []string

		for _, baseName := range roomFiles {
			// Look up the full path(s) for this basename.
			paths := basenameIndex[baseName]
			if len(paths) == 0 {
				// Try as a full path directly.
				paths = []string{baseName}
			}

			for _, fullPath := range paths {
				syms := symbolsByFile[fullPath]
				entry, deps := codetect.GenerateFileEntry(fullPath, syms, projectRoot, moduleName)
				fileEntries = append(fileEntries, entry)
				allRoomDeps = append(allRoomDeps, deps...)
			}
		}

		seeAlso := codetect.BuildSeeAlso(roomName, allRoomDeps, depPathToRoom)
		content := codetect.GenerateRoom(roomName, fileEntries, seeAlso)

		if dryRun {
			fmt.Printf("=== %s ===\n%s\n", roomName, content)
			continue
		}

		outPath := filepath.Join(indexDir, roomName+".md")
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			return fmt.Errorf("creating index dir: %w", err)
		}
		if err := os.WriteFile(outPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", outPath, err)
		}
		fmt.Printf("Generated: %s\n", outPath)
	}

	return nil
}

// loadExistingRooms walks indexDir and parses each *.md file (excluding
// _root.md) for `# filename.ext` headings.  Returns map[roomKey][]string where
// roomKey is the filename stem and the slice contains the referenced basenames.
func loadExistingRooms(indexDir string) (map[string][]string, error) {
	result := make(map[string][]string)

	entries, err := os.ReadDir(indexDir)
	if err != nil {
		return nil, fmt.Errorf("reading index dir %s: %w", indexDir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		if entry.Name() == "_root.md" {
			continue
		}

		roomName := strings.TrimSuffix(entry.Name(), ".md")
		path := filepath.Join(indexDir, entry.Name())

		files, err := parseRoomFileHeadings(path)
		if err != nil {
			// Skip unreadable files.
			continue
		}
		result[roomName] = files
	}

	return result, nil
}

// parseRoomFileHeadings reads a room markdown file and returns the list of
// file basenames referenced as `# filename.ext` headings at the h1 level.
func parseRoomFileHeadings(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var files []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "# ") {
			continue
		}
		heading := strings.TrimPrefix(line, "# ")
		heading = strings.TrimSpace(heading)

		// Heuristic: a source file heading has an extension.
		if strings.Contains(heading, ".") && !strings.HasPrefix(heading, "Room:") {
			if !seen[heading] {
				seen[heading] = true
				files = append(files, heading)
			}
		}
	}
	return files, scanner.Err()
}

// buildDepToRoomMap builds a mapping from dep/import path → room name, used
// for computing see_also cross-references.
func buildDepToRoomMap(
	rooms map[string][]string,
	symbolsByFile map[string][]Symbol,
	projectRoot, moduleName string,
	basenameIndex map[string][]string,
) map[string]string {
	result := make(map[string]string)

	for roomName, roomFiles := range rooms {
		for _, baseName := range roomFiles {
			paths := basenameIndex[baseName]
			if len(paths) == 0 {
				paths = []string{baseName}
			}
			for _, fullPath := range paths {
				// Map the package import path to this room.
				if strings.HasSuffix(fullPath, ".go") {
					abs := fullPath
					if !filepath.IsAbs(fullPath) {
						abs = filepath.Join(projectRoot, fullPath)
					}
					dir := filepath.Dir(abs)
					rel, err := filepath.Rel(projectRoot, dir)
					if err == nil && moduleName != "" {
						importPath := moduleName + "/" + rel
						result[importPath] = roomName
						result[rel] = roomName
					}
				}
			}
		}
	}
	return result
}

// roomList returns a comma-separated list of room names for error messages.
func roomList(rooms map[string][]string) string {
	var names []string
	for k := range rooms {
		names = append(names, k)
	}
	return strings.Join(names, ", ")
}

// Symbol is a local type alias so generate.go can reference codetect.Symbol
// without the package qualifier in buildDepToRoomMap, where it only uses the
// map key.  This avoids an import cycle — generate.go already imports codetect.
type Symbol = codetect.Symbol
