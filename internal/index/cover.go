package index

import (
	"os"
	"path/filepath"
	"strings"
)

// CoverStrategy selects how FindCoveringRooms matches source files to rooms.
type CoverStrategy int

const (
	// CoverBySourcePaths matches using declared "Source paths:" lines in room files.
	// Used by check-stale.
	CoverBySourcePaths CoverStrategy = iota
	// CoverByContent matches by searching room bodies for the filename stem.
	// Used by the watcher's SourceHandler.
	CoverByContent
)

// FindCoveringRooms returns paths of LOI room .md files that cover sourceFile.
// projectRoot is the git root; sourceFile is relative to projectRoot.
// Python source:
//
//	CoverBySourcePaths: find_covering_rooms() in check_stale.py
//	CoverByContent: find_covering_rooms() in watcher.py
func FindCoveringRooms(projectRoot, sourceFile string, strategy CoverStrategy) ([]string, error) {
	indexDir := filepath.Join(projectRoot, "docs", "index")
	info, err := os.Stat(indexDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var covering []string

	err = filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}

		switch strategy {
		case CoverBySourcePaths:
			paths, readErr := ExtractSourcePaths(path)
			if readErr != nil {
				return nil
			}
			for _, sp := range paths {
				if coversBySourcePath(sourceFile, sp) {
					covering = append(covering, path)
					return nil
				}
			}

		case CoverByContent:
			// Skip _root.md files and files starting with "_"
			if strings.HasPrefix(d.Name(), "_") {
				return nil
			}
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil
			}
			content := strings.ToLower(string(data))
			stem := strings.ToLower(strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile)))
			sfLower := strings.ToLower(sourceFile)
			if strings.Contains(content, stem) || strings.Contains(content, sfLower) {
				covering = append(covering, path)
			}
		}

		return nil
	})

	return covering, err
}

// coversBySourcePath checks if sourceFile is covered by the declared source path sp.
// Mirrors the Python: source_file.startswith(sp + "/") or source_file == sp or source_file.startswith(sp)
func coversBySourcePath(sourceFile, sp string) bool {
	return sourceFile == sp ||
		strings.HasPrefix(sourceFile, sp+"/") ||
		strings.HasPrefix(sourceFile, sp)
}
