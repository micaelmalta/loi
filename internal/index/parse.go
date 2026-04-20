package index

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// SourceExts is the set of source file extensions to track.
var SourceExts = map[string]bool{
	".go": true, ".py": true, ".js": true, ".ts": true, ".tsx": true,
	".jsx": true, ".rb": true, ".rs": true, ".java": true, ".kt": true,
	".swift": true, ".c": true, ".cpp": true, ".h": true, ".hpp": true,
	".cs": true, ".sh": true, ".bash": true, ".zsh": true,
}

// Frontmatter holds parsed YAML frontmatter from any LOI markdown file.
type Frontmatter struct {
	Room                string
	SeeAlso             []string
	ArchitecturalHealth string
	SecurityTier        string
	PatternAliases      []string
	LastValidated       string
	StaleSlice          string
	HotPaths            string
	CommitteeNotes      string
	Raw                 map[string]string // everything not recognized above
}

// ParseFrontmatter reads path and extracts YAML frontmatter.
// Returns nil (not error) if the file has no frontmatter.
// Handles:
//   - scalar values (key: value)
//   - quoted values (key: "value" or key: 'value')
//   - inline list values (key: ["a", "b"])
//   - block list items (  - item under a key: line)
//
// Maps recognized fields to struct fields; everything else goes in Raw.
// Python source: parse_frontmatter() in validate_loi.py
//
//	+ parse_frontmatter_raw() in validate_patterns.py (handles lists)
//	+ parse_room_frontmatter_flags() in governance.py
func ParseFrontmatter(path string) (*Frontmatter, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)

	if !strings.HasPrefix(text, "---") {
		return nil, nil
	}
	// Find closing "---" (must be at position > 3 to skip the opening marker)
	end := strings.Index(text[3:], "---")
	if end == -1 {
		return nil, nil
	}
	// end is offset from text[3:], so absolute index of closing "---" start is end+3
	body := text[3 : end+3]

	fm := &Frontmatter{
		Raw: make(map[string]string),
	}

	var currentKey string
	var currentList []string
	inList := false

	commitList := func() {
		if currentKey == "" {
			return
		}
		fm.setList(currentKey, currentList)
		currentKey = ""
		currentList = nil
		inList = false
	}

	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		stripped := strings.TrimSpace(line)

		if stripped == "" || strings.HasPrefix(stripped, "#") {
			continue
		}

		// Block list item: line starts with optional whitespace then "- "
		if inList && regexp.MustCompile(`^\s+-\s`).MatchString(line) {
			item := strings.TrimSpace(line)
			item = strings.TrimPrefix(item, "- ")
			item = strings.Trim(item, `"'`)
			currentList = append(currentList, item)
			continue
		}

		// Key-value line
		colonIdx := strings.Index(stripped, ":")
		if colonIdx == -1 {
			inList = false
			currentKey = ""
			continue
		}

		// Commit any pending list before starting a new key
		if inList {
			commitList()
		}

		rawKey := stripped[:colonIdx]
		// Only accept word-like keys (letters, digits, underscores, hyphens)
		if !regexp.MustCompile(`^\w[\w_-]*$`).MatchString(rawKey) {
			inList = false
			currentKey = ""
			continue
		}

		rawVal := strings.TrimSpace(stripped[colonIdx+1:])

		// Empty or "[]" => start block list
		if rawVal == "" || rawVal == "[]" {
			currentKey = rawKey
			currentList = []string{}
			inList = true
			continue
		}

		// Inline list: ["a", "b"] or ['a', 'b']
		if strings.HasPrefix(rawVal, "[") {
			inner := strings.Trim(rawVal, "[]")
			parts := strings.Split(inner, ",")
			var items []string
			for _, p := range parts {
				item := strings.TrimSpace(p)
				item = strings.Trim(item, `"'`)
				if item != "" {
					items = append(items, item)
				}
			}
			fm.setList(rawKey, items)
			currentKey = ""
			inList = false
			continue
		}

		// Scalar value — strip surrounding quotes
		val := strings.Trim(rawVal, `"'`)
		fm.setScalar(rawKey, val)
		currentKey = ""
		inList = false
	}

	// Commit any trailing list
	if inList {
		commitList()
	}

	return fm, nil
}

// setScalar assigns a scalar value to the appropriate Frontmatter field.
func (f *Frontmatter) setScalar(key, val string) {
	switch key {
	case "room":
		f.Room = val
	case "architectural_health":
		f.ArchitecturalHealth = val
	case "security_tier":
		f.SecurityTier = val
	case "last_validated":
		f.LastValidated = val
	case "stale_since":
		f.StaleSlice = val
	case "hot_paths":
		f.HotPaths = val
	case "committee_notes":
		f.CommitteeNotes = val
	default:
		f.Raw[key] = val
	}
}

// setList assigns a list value to the appropriate Frontmatter field.
func (f *Frontmatter) setList(key string, items []string) {
	switch key {
	case "see_also":
		f.SeeAlso = items
	case "pattern_aliases":
		f.PatternAliases = items
	default:
		// Flatten list back into a comma-separated scalar for Raw
		if len(items) > 0 {
			f.Raw[key] = strings.Join(items, ", ")
		} else {
			f.Raw[key] = ""
		}
	}
}

// UpdateFrontmatterField atomically updates (or inserts) a key in YAML frontmatter.
// If the key exists: regex-replaces the value on that line.
// If absent: inserts before the closing "---".
// Writes atomically via tmp file + os.Rename.
// Returns true if the file was changed.
// Python source: update_room_health() + mark_room_stale() in watcher.py
func UpdateFrontmatterField(path, key, value string) (bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	text := string(data)

	if !strings.HasPrefix(text, "---") {
		return false, nil
	}

	// Find "\n---" which marks the end of frontmatter
	closingIdx := strings.Index(text[3:], "\n---")
	if closingIdx == -1 {
		return false, nil
	}
	// closingIdx is relative to text[3:], so absolute position of the "\n" is closingIdx+3
	fmStart := 3
	fmEnd := closingIdx + 3 // index of "\n---" in text

	fm := text[fmStart:fmEnd]
	rest := text[fmEnd+4:] // skip "\n---"

	keyPattern := regexp.MustCompile(fmt.Sprintf(`(?m)(^%s\s*:\s*)\S+`, regexp.QuoteMeta(key)))

	var newFM string
	if keyPattern.MatchString(fm) {
		newFM = keyPattern.ReplaceAllString(fm, "${1}"+value)
	} else {
		// Insert before closing "---"
		trimmed := strings.TrimRight(fm, "\n")
		newFM = trimmed + "\n" + key + ": " + value + "\n"
	}

	newText := "---" + newFM + "\n---" + rest
	if newText == text {
		return false, nil
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(newText), 0o644); err != nil {
		return false, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return false, err
	}
	return true, nil
}

// PatternMeta holds per-pattern validation metadata.
type PatternMeta struct {
	Name             string
	FirstIntroduced  string
	LastValidated    string
	ValidationSource string
}

var patternMetaBlockRe = regexp.MustCompile(`(?m)pattern_metadata\s*:\s*\n((?:\s+.*\n?)*)`)
var patternMetaKVRe = regexp.MustCompile(`^\s+(\w[\w_-]*):\s*(.*)`)
var patternMetaItemRe = regexp.MustCompile(`^\s+-\s+name:\s*(.*)`)

// ParsePatternMetadataBlock reads optional pattern_metadata: YAML block from a room file.
// This block appears in the room body (not frontmatter) and has the shape:
//
//	pattern_metadata:
//	  - name: Token rotation without service restart
//	    first_introduced: 2026-04-09
//	    last_validated: 2026-04-18
//	    validation_source: refresh_token_test.go
//
// Returns map[normalizedName]PatternMeta.
// Python source: parse_pattern_metadata_block() in validate_patterns.py
func ParsePatternMetadataBlock(path string) (map[string]PatternMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)

	m := patternMetaBlockRe.FindStringSubmatch(text)
	if m == nil {
		return map[string]PatternMeta{}, nil
	}
	block := m[1]

	result := make(map[string]PatternMeta)
	var current *PatternMeta

	commit := func() {
		if current != nil && current.Name != "" {
			key := Normalize(current.Name)
			result[key] = *current
		}
	}

	for _, line := range strings.Split(block, "\n") {
		if im := patternMetaItemRe.FindStringSubmatch(line); im != nil {
			commit()
			current = &PatternMeta{Name: strings.TrimSpace(im[1])}
			continue
		}
		if current != nil {
			if kv := patternMetaKVRe.FindStringSubmatch(line); kv != nil {
				val := strings.TrimSpace(kv[2])
				switch kv[1] {
				case "first_introduced":
					current.FirstIntroduced = val
				case "last_validated":
					current.LastValidated = val
				case "validation_source":
					current.ValidationSource = val
				}
			} else if strings.TrimSpace(line) == "" {
				continue
			} else {
				// Non-indented or unrecognized — end of block
				break
			}
		}
	}
	commit()

	return result, nil
}

var proposalMetaBlockRe = regexp.MustCompile(`(?m)^proposal_metadata\s*:\s*\n((?:[ \t]+.*\n?)*)`)
var proposalMetaItemRe = regexp.MustCompile(`^\s+-\s+(.*)`)
var proposalMetaKVRe = regexp.MustCompile(`^\s+(\w[\w_-]*):\s*(.*)`)

// ParseProposalMetadata reads a proposal_metadata: block from a proposal file.
// Returns map[string]interface{} with string scalars and []string for list fields.
// Python source: parse_proposal_metadata() in proposals.py
func ParseProposalMetadata(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := string(data)

	m := proposalMetaBlockRe.FindStringSubmatch(text)
	if m == nil {
		return nil, nil
	}
	block := m[1]

	meta := make(map[string]interface{})
	var currentListKey string
	var currentList []string

	commitList := func() {
		if currentListKey != "" {
			meta[currentListKey] = currentList
			currentListKey = ""
			currentList = nil
		}
	}

	for _, line := range strings.Split(block, "\n") {
		if im := proposalMetaItemRe.FindStringSubmatch(line); im != nil && currentListKey != "" {
			currentList = append(currentList, strings.TrimSpace(im[1]))
			continue
		}
		if kv := proposalMetaKVRe.FindStringSubmatch(line); kv != nil {
			commitList()
			key := kv[1]
			val := strings.TrimSpace(kv[2])
			val = strings.Trim(val, `"'`)
			if val == "" || val == "[]" {
				currentListKey = key
				currentList = []string{}
			} else {
				meta[key] = val
			}
		} else if strings.TrimSpace(line) == "" {
			commitList()
		}
	}
	commitList()

	if len(meta) == 0 {
		return nil, nil
	}
	return meta, nil
}

var mdLinkRe = regexp.MustCompile(`[\w./-]+\.md`)

// ExtractMDLinks extracts markdown paths ending in .md from a file.
// Python source: extract_md_links() in validate_loi.py
func ExtractMDLinks(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return mdLinkRe.FindAllString(string(data), -1), nil
}

var sourcePathsRe = regexp.MustCompile(`(?i)Source paths?:\s*(.+)`)

// ExtractSourcePaths extracts declared source paths from a room file.
// Looks for lines matching "Source paths?: (.+)" (case-insensitive).
// Splits on comma, strips trailing slashes.
// Python source: extract_source_paths() in check_stale.py
func ExtractSourcePaths(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, m := range sourcePathsRe.FindAllStringSubmatch(string(data), -1) {
		for _, part := range strings.Split(m[1], ",") {
			cleaned := strings.TrimRight(strings.TrimSpace(part), "/")
			if cleaned != "" {
				paths = append(paths, cleaned)
			}
		}
	}
	return paths, nil
}

var entryHeadingRe = regexp.MustCompile(`(?m)^# \S+\.\w+`)

// CountEntries counts `# filename.ext` headings in a room file.
// Python source: count_entries() in validate_loi.py
func CountEntries(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return len(entryHeadingRe.FindAllString(string(data), -1)), nil
}

var nonWordRe = regexp.MustCompile(`[^\w\s]`)
var whitespaceRe = regexp.MustCompile(`\s+`)

// Normalize lowercases, strips non-word chars, collapses whitespace.
// Python source: normalize() in validate_patterns.py
func Normalize(text string) string {
	text = strings.ToLower(text)
	text = nonWordRe.ReplaceAllString(text, " ")
	text = whitespaceRe.ReplaceAllString(text, " ")
	return strings.TrimSpace(text)
}

// FindSourceDirs walks root and returns relative dir paths containing source files.
// Skips dirs in excluded set and dirs starting with ".".
// Python source: find_source_dirs() in validate_loi.py
func FindSourceDirs(root string, excluded map[string]bool) ([]string, error) {
	seen := make(map[string]bool)

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if d.IsDir() {
			name := d.Name()
			if strings.HasPrefix(name, ".") || excluded[name] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if SourceExts[ext] {
			rel, relErr := filepath.Rel(root, filepath.Dir(path))
			if relErr != nil {
				return nil
			}
			if rel == "." {
				rel = ""
			}
			seen[rel] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	return dirs, nil
}

// ParseGitignoreDirs extracts directory-pattern entries from .gitignore (lines ending with /).
// Python source: parse_gitignore_dirs() in validate_loi.py
func ParseGitignoreDirs(projectRoot string) (map[string]bool, error) {
	path := filepath.Join(projectRoot, ".gitignore")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}

	dirs := make(map[string]bool)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasSuffix(line, "/") {
			// Take the last path segment, strip slashes
			name := strings.Trim(line, "/")
			if idx := strings.LastIndex(name, "/"); idx >= 0 {
				name = name[idx+1:]
			}
			if name != "" {
				dirs[name] = true
			}
		}
	}
	return dirs, nil
}

var taskFileRefRe = regexp.MustCompile(`^[\w./-]+\.\w+$`)

// ExtractTaskFileRefs extracts file paths and glob patterns from TASK table Load cells.
// Looks for cells matching ^[\w./-]+\.\w+$ with "/" inside, or containing "*" and "/".
// Python source: extract_task_file_refs() in validate_loi.py
func ExtractTaskFileRefs(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var refs []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cells := strings.Split(line, "|")
		for _, cell := range cells {
			cell = strings.TrimFunc(cell, unicode.IsSpace)
			if cell == "" {
				continue
			}
			if taskFileRefRe.MatchString(cell) && strings.Contains(cell, "/") {
				refs = append(refs, cell)
			} else if strings.Contains(cell, "*") && strings.Contains(cell, "/") {
				refs = append(refs, cell)
			}
		}
	}
	return refs, nil
}
