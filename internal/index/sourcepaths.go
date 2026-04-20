package index

import (
	"os"
	"path/filepath"
	"strings"
)

// ExtractSourcePathsFromRooms walks all .md files under indexDir (recursively,
// skipping _root.md files) and collects every declared "Source paths:" value.
// Returns the union of all source paths found across all room files.
// Python source: inline logic in validate_loi.py source-coverage check.
func ExtractSourcePathsFromRooms(indexDir string) ([]string, error) {
	seen := make(map[string]bool)

	err := filepath.WalkDir(indexDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if filepath.Ext(d.Name()) != ".md" {
			return nil
		}
		if strings.HasPrefix(d.Name(), "_") {
			return nil
		}
		paths, readErr := ExtractSourcePaths(path)
		if readErr != nil {
			return nil
		}
		for _, p := range paths {
			seen[p] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(seen))
	for p := range seen {
		result = append(result, p)
	}
	return result, nil
}
