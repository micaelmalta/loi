package index

import (
	"fmt"
	"regexp"
	"strings"
)

// TableRow is a slice of trimmed cell strings from one markdown table row.
type TableRow []string

// Tables holds parsed TASK, PATTERN, and GOVERNANCE table rows from one file.
type Tables struct {
	Task       []TableRow
	Pattern    []TableRow
	Governance []TableRow
}

var (
	taskHeadingRe       = regexp.MustCompile(`(?i)^#+\s+TASK`)
	patternHeadingRe    = regexp.MustCompile(`(?i)^#+\s+PATTERN`)
	governanceHeadingRe = regexp.MustCompile(`(?i)^#+.*\bGOVERNANCE\b`)
	anyHeadingRe        = regexp.MustCompile(`^#+\s+`)
	separatorCellRe     = regexp.MustCompile(`^[-:]+$`)
)

// ParseTables parses TASK/PATTERN/GOVERNANCE markdown tables from text.
// Detects section headings:
//
//	TASK:       /^#+\s+TASK/i
//	PATTERN:    /^#+\s+PATTERN/i
//	GOVERNANCE: /^#+.*\bGOVERNANCE\b/i
//
// Skips separator rows (all cells match /^[-:]+$/).
// Skips header row (first data row after a separator).
// Python source: parse_tables() in diff_tables.py
func ParseTables(text string) Tables {
	type sectionType int
	const (
		sectionNone sectionType = iota
		sectionTask
		sectionPattern
		sectionGovernance
	)

	var tables Tables
	currentSection := sectionNone
	headerSeen := false

	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)

		// Detect section headings
		if taskHeadingRe.MatchString(stripped) {
			currentSection = sectionTask
			headerSeen = false
			continue
		}
		if patternHeadingRe.MatchString(stripped) {
			currentSection = sectionPattern
			headerSeen = false
			continue
		}
		if governanceHeadingRe.MatchString(stripped) {
			currentSection = sectionGovernance
			headerSeen = false
			continue
		}
		// Any other heading ends the current section
		if anyHeadingRe.MatchString(stripped) {
			currentSection = sectionNone
			continue
		}

		if currentSection == sectionNone {
			continue
		}

		if !strings.HasPrefix(stripped, "|") {
			continue
		}

		cells := parseTableRowCells(line)
		if len(cells) == 0 {
			continue
		}

		// Skip separator rows
		isSep := true
		for _, c := range cells {
			if !separatorCellRe.MatchString(c) {
				isSep = false
				break
			}
		}
		if isSep {
			headerSeen = true
			continue
		}

		// Skip the header row (first data row after we see any separator, or first data row)
		if !headerSeen {
			headerSeen = true
			continue
		}

		row := TableRow(cells)
		switch currentSection {
		case sectionTask:
			tables.Task = append(tables.Task, row)
		case sectionPattern:
			tables.Pattern = append(tables.Pattern, row)
		case sectionGovernance:
			tables.Governance = append(tables.Governance, row)
		}
	}

	return tables
}

// parseTableRowCells splits a markdown table line into trimmed, non-empty cells.
func parseTableRowCells(line string) []string {
	parts := strings.Split(line, "|")
	var cells []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cells = append(cells, trimmed)
		}
	}
	return cells
}

// GovEntry is one row from a GOVERNANCE WATCHLIST table.
type GovEntry struct {
	Room       string
	Health     string
	Security   string
	Note       string
	Repo       string // set by aggregation layer, not parsed from table
	SourceFile string // path of the _root.md this came from
}

// ParseGovernanceTable parses GOVERNANCE WATCHLIST rows from text.
// Strips backticks from Room/Health/Security cells.
// Lowercases Health and Security values.
// Python source: parse_governance_table() in governance.py
func ParseGovernanceTable(text string) []GovEntry {
	inSection := false
	headerSeen := false
	var entries []GovEntry

	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)

		if governanceHeadingRe.MatchString(stripped) {
			inSection = true
			headerSeen = false
			continue
		}
		if inSection && anyHeadingRe.MatchString(stripped) && !governanceHeadingRe.MatchString(stripped) {
			inSection = false
			continue
		}
		if !inSection {
			continue
		}
		if !strings.HasPrefix(stripped, "|") {
			continue
		}

		cells := parseTableRowCells(line)
		if len(cells) == 0 {
			continue
		}

		// Skip separator lines
		isSep := true
		for _, c := range cells {
			if !separatorCellRe.MatchString(c) {
				isSep = false
				break
			}
		}
		if isSep {
			headerSeen = true
			continue
		}
		if !headerSeen {
			headerSeen = true
			continue
		}

		room := ""
		health := "normal"
		security := "normal"
		note := ""

		if len(cells) > 0 {
			room = strings.Trim(cells[0], "`")
		}
		if len(cells) > 1 {
			health = strings.ToLower(strings.Trim(cells[1], "`"))
		}
		if len(cells) > 2 {
			security = strings.ToLower(strings.Trim(cells[2], "`"))
		}
		if len(cells) > 3 {
			note = cells[3]
		}

		if room != "" {
			entries = append(entries, GovEntry{
				Room:     room,
				Health:   health,
				Security: security,
				Note:     note,
			})
		}
	}

	return entries
}

// PatternRow is one PATTERN table row.
type PatternRow struct {
	Pattern    string
	TargetPath string // the Load cell value
	RawLine    string
}

// ExtractPatternRows parses PATTERN table rows from a _root.md file.
// Pattern is the first cell, TargetPath is the second.
// Python source: extract_pattern_rows() in validate_patterns.py
func ExtractPatternRows(text string) []PatternRow {
	inPattern := false
	var rows []PatternRow

	for _, line := range strings.Split(text, "\n") {
		stripped := strings.TrimSpace(line)

		if patternHeadingRe.MatchString(stripped) {
			inPattern = true
			continue
		}
		if inPattern && anyHeadingRe.MatchString(stripped) && !patternHeadingRe.MatchString(stripped) {
			inPattern = false
			continue
		}
		if !inPattern {
			continue
		}
		if !strings.HasPrefix(stripped, "|") {
			continue
		}

		cells := parseTableRowCells(line)
		if len(cells) < 2 {
			continue
		}

		label := cells[0]
		target := cells[1]

		// Skip separator and header rows
		if separatorCellRe.MatchString(label) {
			continue
		}
		lower := strings.ToLower(label)
		if lower == "pattern" || lower == "---" {
			continue
		}

		rows = append(rows, PatternRow{
			Pattern:    label,
			TargetPath: target,
			RawLine:    stripped,
		})
	}

	return rows
}

// ChangedEntry is one changed intent field from a git diff.
type ChangedEntry struct {
	SourceFile  string
	ChangedLine string
}

var intentFieldPrefixes = []string{"DOES:", "SYMBOLS:", "TYPE:", "INTERFACE:", "PATTERNS:"}

// ExtractChangedEntries parses a git diff for changed LOI intent fields.
// Looks for added lines (+) containing DOES:, SYMBOLS:, TYPE:, INTERFACE:, PATTERNS:
// or added table rows with file paths.
// Python source: extract_changed_entries() in watcher.py
func ExtractChangedEntries(diff string) []ChangedEntry {
	var entries []ChangedEntry
	var currentFile string

	headingRe := regexp.MustCompile(`^[+ ]# (\S+\.\w+)`)
	tableRowRe := regexp.MustCompile(`.*\.\w+$`)

	for _, line := range strings.Split(diff, "\n") {
		// Track current file heading from context or added lines
		if !strings.HasPrefix(line, "+") || strings.HasPrefix(line, "+++") {
			if m := headingRe.FindStringSubmatch(line); m != nil {
				currentFile = m[1]
			}
			continue
		}

		added := line[1:] // strip leading "+"

		// Check for intent fields
		matched := false
		for _, field := range intentFieldPrefixes {
			if strings.Contains(added, field) && currentFile != "" {
				entries = append(entries, ChangedEntry{
					SourceFile:  currentFile,
					ChangedLine: strings.TrimSpace(added),
				})
				matched = true
				break
			}
		}
		if matched {
			continue
		}

		// Check for added table rows with file paths
		trimmedAdded := strings.TrimSpace(added)
		if strings.HasPrefix(trimmedAdded, "|") && strings.Contains(trimmedAdded, "|") {
			cells := parseTableRowCells(trimmedAdded)
			if len(cells) >= 2 {
				fileCell := cells[0]
				content := strings.Join(cells[1:], " | ")
				if tableRowRe.MatchString(fileCell) && fileCell != "FILE" {
					entries = append(entries, ChangedEntry{
						SourceFile:  fileCell,
						ChangedLine: truncate(content, 120),
					})
				}
			}
		}
	}

	return entries
}

// truncate returns s truncated to at most n characters.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// TableDiff holds the result of diffing two Tables.
type TableDiff struct {
	Task       TableChanges
	Pattern    TableChanges
	Governance TableChanges
}

// TableChanges holds added/removed/changed rows for one table type.
type TableChanges struct {
	Added   []TableRow
	Removed []TableRow
	Changed [][2]TableRow // [0]=old, [1]=new
}

// DiffTables computes a semantic diff between two Tables.
// Keyed by first cell (primary key). Detects added, removed, changed rows.
// Python source: diff_tables() in diff_tables.py
func DiffTables(old, new Tables) TableDiff {
	return TableDiff{
		Task:       diffRowSets(old.Task, new.Task),
		Pattern:    diffRowSets(old.Pattern, new.Pattern),
		Governance: diffRowSets(old.Governance, new.Governance),
	}
}

func rowKey(r TableRow) string {
	if len(r) == 0 {
		return ""
	}
	return r[0]
}

func rowsToMap(rows []TableRow) map[string]TableRow {
	m := make(map[string]TableRow, len(rows))
	for _, r := range rows {
		k := rowKey(r)
		m[k] = r
	}
	return m
}

func rowEqual(a, b TableRow) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func diffRowSets(old, new []TableRow) TableChanges {
	oldMap := rowsToMap(old)
	newMap := rowsToMap(new)

	var changes TableChanges

	for k, newRow := range newMap {
		if oldRow, exists := oldMap[k]; !exists {
			changes.Added = append(changes.Added, newRow)
		} else if !rowEqual(oldRow, newRow) {
			changes.Changed = append(changes.Changed, [2]TableRow{oldRow, newRow})
		}
	}

	for k, oldRow := range oldMap {
		if _, exists := newMap[k]; !exists {
			changes.Removed = append(changes.Removed, oldRow)
		}
	}

	return changes
}

func formatRow(r TableRow) string {
	return strings.Join(r, " -> ")
}

// FormatDiff formats a TableDiff as a human-readable string.
// Returns "" if no changes.
// Python source: format_diff() in diff_tables.py
// Example output:
//
//	TASK changes
//	  + added: Issue or rotate a UCAN token -> auth/ucan.md
//	  - removed: Old task -> old.md
//	PATTERN changes
//	  ~ changed: Token rotation -> auth/ucan.md (was auth/old.md)
func FormatDiff(d TableDiff) string {
	type section struct {
		name    string
		changes TableChanges
	}
	sections := []section{
		{"TASK", d.Task},
		{"PATTERN", d.Pattern},
		{"GOVERNANCE", d.Governance},
	}

	var lines []string
	for _, s := range sections {
		c := s.changes
		if len(c.Added) == 0 && len(c.Removed) == 0 && len(c.Changed) == 0 {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s changes", s.name))
		for _, r := range c.Added {
			lines = append(lines, fmt.Sprintf("  + added: %s", formatRow(r)))
		}
		for _, r := range c.Removed {
			lines = append(lines, fmt.Sprintf("  - removed: %s", formatRow(r)))
		}
		for _, pair := range c.Changed {
			old, newR := pair[0], pair[1]
			// Show what changed: key is same, but rest differs
			if len(old) >= 2 && len(newR) >= 2 {
				lines = append(lines, fmt.Sprintf("  ~ changed: %s -> %s (was %s)", old[0], newR[1], old[1]))
			} else {
				lines = append(lines, fmt.Sprintf("  ~ changed: %s (was %s)", formatRow(newR), formatRow(old)))
			}
		}
	}

	return strings.Join(lines, "\n")
}
