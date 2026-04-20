package codetect

import (
	"bufio"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Symbol mirrors the codetect symbols table schema.
type Symbol struct {
	Name      string
	Kind      string
	Path      string
	Line      int
	Pattern   string
	Scope     string
	Signature string
}

var (
	// FunctionKinds is the set of symbol kinds treated as callable functions.
	FunctionKinds = map[string]bool{"function": true}

	// GroupedKinds maps a kind to its display group heading.
	GroupedKinds = map[string]string{
		"struct":    "Types",
		"interface": "Interfaces",
	}

	// SkipKinds are symbol kinds that are not useful in LOI room entries.
	SkipKinds = map[string]bool{
		"field": true, "constant": true, "variable": true, "receiver": true,
		"parameter": true, "package": true, "packageName": true, "anonMember": true,
		"local": true, "database": true, "index": true, "table": true,
		"talias": true,
	}
)

// OpenDB opens the codetect symbols.db in immutable read-only mode.
// URI mode: "file:path?immutable=1"
func OpenDB(dbPath string) (*sql.DB, error) {
	uri := fmt.Sprintf("file:%s?immutable=1", dbPath)
	db, err := sql.Open("sqlite", uri)
	if err != nil {
		return nil, fmt.Errorf("codetect: open db %s: %w", dbPath, err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("codetect: ping db %s: %w", dbPath, err)
	}
	return db, nil
}

// QuerySymbols queries all symbols, groups them by source file path, and
// deduplicates by (normalizedName, kind, line).
//
// Normalization: if the name contains a "." separator, strip the package
// prefix — keep only the part after the last dot.
//
// Returns map[filePath][]Symbol.
func QuerySymbols(db *sql.DB) (map[string][]Symbol, error) {
	rows, err := db.Query(
		`SELECT name, kind, path, line,
		        COALESCE(pattern, ''),
		        COALESCE(scope, ''),
		        COALESCE(signature, '')
		 FROM symbols
		 ORDER BY path, line`,
	)
	if err != nil {
		return nil, fmt.Errorf("codetect: query symbols: %w", err)
	}
	defer rows.Close()

	type dedupeKey struct {
		name string
		kind string
		line int
	}

	result := make(map[string][]Symbol)
	seen := make(map[string]map[dedupeKey]bool) // per-file seen set

	for rows.Next() {
		var s Symbol
		if err := rows.Scan(&s.Name, &s.Kind, &s.Path, &s.Line,
			&s.Pattern, &s.Scope, &s.Signature); err != nil {
			return nil, fmt.Errorf("codetect: scan symbol row: %w", err)
		}

		// Normalize name: strip package prefix on "." separator.
		normalized := s.Name
		if idx := strings.LastIndex(s.Name, "."); idx >= 0 {
			normalized = s.Name[idx+1:]
		}

		if seen[s.Path] == nil {
			seen[s.Path] = make(map[dedupeKey]bool)
		}
		k := dedupeKey{name: normalized, kind: s.Kind, line: s.Line}
		if seen[s.Path][k] {
			continue
		}
		seen[s.Path][k] = true

		// Store with normalized name for display.
		s.Name = normalized
		result[s.Path] = append(result[s.Path], s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("codetect: iterating symbol rows: %w", err)
	}
	return result, nil
}

// GetModuleName reads go.mod in projectRoot and returns the module path, or ""
// if not found or not parseable.
func GetModuleName(projectRoot string) string {
	gomod := filepath.Join(projectRoot, "go.mod")
	f, err := os.Open(gomod)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if mod, ok := strings.CutPrefix(line, "module "); ok {
			return strings.TrimSpace(mod)
		}
	}
	return ""
}

// ParseGoImports extracts import paths from a Go source file.
// Handles both:
//   - single: import "path"
//   - block:  import ( "path1" \n "path2" )
func ParseGoImports(filePath string) ([]string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("codetect: open %s: %w", filePath, err)
	}
	defer f.Close()

	var imports []string
	inBlock := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if !inBlock {
			// Single-line import: import "path"
			if strings.HasPrefix(line, `import "`) {
				path := extractQuoted(line)
				if path != "" {
					imports = append(imports, path)
				}
				continue
			}
			// Start of import block
			if line == "import (" || strings.HasPrefix(line, "import (") {
				inBlock = true
				continue
			}
		} else {
			if line == ")" {
				inBlock = false
				continue
			}
			// Each line inside a block may be `"path"` or `alias "path"`.
			path := extractQuoted(line)
			if path != "" {
				imports = append(imports, path)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("codetect: scanning %s: %w", filePath, err)
	}
	return imports, nil
}

// extractQuoted returns the content of the first double-quoted string found in
// line, or "" if none.
func extractQuoted(line string) string {
	start := strings.Index(line, `"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(line[start+1:], `"`)
	if end < 0 {
		return ""
	}
	return line[start+1 : start+1+end]
}

// ClassifyImports classifies import paths as same-repo (packages within the
// module) or external.  Stdlib entries (no dot in the first path segment,
// purely lowercase first segment) are silently skipped.
//
// sameRepo paths have the module prefix stripped (e.g.
// "github.com/org/repo/internal/foo" → "internal/foo").
func ClassifyImports(imports []string, moduleName string) (sameRepo, external []string) {
	for _, imp := range imports {
		if isStdlib(imp) {
			continue
		}
		if moduleName != "" && strings.HasPrefix(imp, moduleName) {
			rel := strings.TrimPrefix(imp, moduleName)
			rel = strings.TrimPrefix(rel, "/")
			sameRepo = append(sameRepo, rel)
		} else {
			external = append(external, imp)
		}
	}
	return sameRepo, external
}

// isStdlib returns true when the import path is a Go standard-library package:
// the first path segment contains no dot and is purely lowercase ASCII.
func isStdlib(imp string) bool {
	seg := imp
	if before, _, ok := strings.Cut(imp, "/"); ok {
		seg = before
	}
	if strings.Contains(seg, ".") {
		return false
	}
	for _, r := range seg {
		if r >= 'A' && r <= 'Z' {
			return false
		}
	}
	return true
}

// ReadFuncSignature reads the function signature starting at line (1-based) in
// the file at filepath (relative to projectRoot).  It collects source lines
// until it encounters a "{", strips the leading "func " prefix, and returns a
// cleaned single-line signature.  Returns "" on any error.
func ReadFuncSignature(projectRoot, filePath string, line int) string {
	abs := filePath
	if !filepath.IsAbs(filePath) {
		abs = filepath.Join(projectRoot, filePath)
	}

	f, err := os.Open(abs)
	if err != nil {
		return ""
	}
	defer f.Close()

	var (
		sigParts []string
		current  int
		found    bool
	)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		current++
		if current < line {
			continue
		}
		text := scanner.Text()
		sigParts = append(sigParts, strings.TrimSpace(text))
		if strings.Contains(text, "{") {
			found = true
			break
		}
	}

	if !found || len(sigParts) == 0 {
		return ""
	}

	sig := strings.Join(sigParts, " ")
	// Remove the body opening: everything from " {" onward.
	if idx := strings.Index(sig, "{"); idx >= 0 {
		sig = strings.TrimSpace(sig[:idx])
	}
	// Strip "func " prefix.
	sig = strings.TrimPrefix(sig, "func ")
	return strings.TrimSpace(sig)
}

const (
	maxGroupedDisplay = 3  // max names shown inline before "+N more"
	maxFunctions      = 20 // cap on function entries per file
)

// BuildSymbolsLines builds the SYMBOLS entry lines for a file.
//
//   - Functions (FunctionKinds): one line per function, "- sig" where sig is
//     read from the source via ReadFuncSignature (falls back to Name).
//   - Grouped kinds (GroupedKinds): collected per group, printed as
//     "- Types: A, B, C (+N more)".
//   - SkipKinds are ignored.
func BuildSymbolsLines(symbols []Symbol, projectRoot string) []string {
	grouped := make(map[string][]string) // group heading → names
	var funcLines []string

	for _, s := range symbols {
		if SkipKinds[s.Kind] {
			continue
		}
		if FunctionKinds[s.Kind] {
			if len(funcLines) < maxFunctions {
				sig := ReadFuncSignature(projectRoot, s.Path, s.Line)
				if sig == "" {
					sig = s.Name
				}
				funcLines = append(funcLines, "- "+sig)
			}
			continue
		}
		if heading, ok := GroupedKinds[s.Kind]; ok {
			grouped[heading] = append(grouped[heading], s.Name)
		}
	}

	var lines []string
	lines = append(lines, funcLines...)

	// Emit grouped kinds in a stable order.
	for _, heading := range []string{"Types", "Interfaces"} {
		names, ok := grouped[heading]
		if !ok {
			continue
		}
		var display string
		if len(names) <= maxGroupedDisplay {
			display = strings.Join(names, ", ")
		} else {
			display = fmt.Sprintf("%s (+%d more)",
				strings.Join(names[:maxGroupedDisplay], ", "),
				len(names)-maxGroupedDisplay)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", heading, display))
	}
	return lines
}

// GenerateFileEntry generates one LOI entry block for a source file.
// Returns (entryMarkdown, sameRepoDeps).
//
// Format mirrors generate_loi.py's generate_file_entry():
//
//	# filename.go
//	<!-- LLM-FILL: DOES -->
//	SYMBOLS:
//	- sig1
//	DEPENDS: dep1, dep2
//	<!-- LLM-FILL: PATTERNS, USE WHEN, EMITS, CONSUMERS -->
func GenerateFileEntry(filePath string, symbols []Symbol, projectRoot, moduleName string) (string, []string) {
	basename := filepath.Base(filePath)
	symbolLines := BuildSymbolsLines(symbols, projectRoot)

	// Resolve imports for DEPENDS.
	var sameRepoDeps []string
	var externalDeps []string

	if strings.HasSuffix(filePath, ".go") {
		abs := filePath
		if !filepath.IsAbs(filePath) {
			abs = filepath.Join(projectRoot, filePath)
		}
		imports, err := ParseGoImports(abs)
		if err == nil {
			sameRepoDeps, externalDeps = ClassifyImports(imports, moduleName)
		}
	}

	// Build DEPENDS list: same-repo first, then external (capped to keep it concise).
	var allDeps []string
	allDeps = append(allDeps, sameRepoDeps...)
	if len(allDeps) == 0 {
		allDeps = append(allDeps, externalDeps...)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s\n", basename)
	fmt.Fprintln(&sb, "<!-- LLM-FILL: DOES -->")

	if len(symbolLines) > 0 {
		fmt.Fprintln(&sb, "SYMBOLS:")
		for _, l := range symbolLines {
			fmt.Fprintln(&sb, l)
		}
	}

	if len(allDeps) > 0 {
		fmt.Fprintf(&sb, "DEPENDS: %s\n", strings.Join(allDeps, ", "))
	}

	fmt.Fprintln(&sb, "<!-- LLM-FILL: PATTERNS, USE WHEN, EMITS, CONSUMERS -->")

	return sb.String(), sameRepoDeps
}

// GenerateRoom produces a complete room markdown file.
// seeAlso is a slice of relative paths like ["../other/room.md"].
func GenerateRoom(roomName string, fileEntries []string, seeAlso []string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "# Room: %s\n\n", roomName)

	if len(seeAlso) > 0 {
		fmt.Fprintln(&sb, "see_also:")
		for _, s := range seeAlso {
			fmt.Fprintf(&sb, "  - %s\n", s)
		}
		fmt.Fprintln(&sb)
	}

	for _, entry := range fileEntries {
		fmt.Fprintln(&sb, entry)
	}

	return sb.String()
}

// BuildSeeAlso infers see_also links from cross-room DEPENDS references.
// roomName is the current room; allRoomDeps is the union of all dep paths
// found in the room; depPathToRoom maps import/dep paths to room names.
// Returns a slice of relative Markdown paths like ["../other/room.md"].
func BuildSeeAlso(roomName string, allRoomDeps []string, depPathToRoom map[string]string) []string {
	seen := make(map[string]bool)
	var result []string

	for _, dep := range allRoomDeps {
		targetRoom, ok := depPathToRoom[dep]
		if !ok || targetRoom == roomName {
			continue
		}
		ref := "../" + targetRoom + ".md"
		if !seen[ref] {
			seen[ref] = true
			result = append(result, ref)
		}
	}
	return result
}

// GroupFilesByDirectory groups source file paths by their parent directory,
// using the directory as the default room name.  The room name is the
// directory base name (or "root" for top-level files).
func GroupFilesByDirectory(filePaths []string) map[string][]string {
	groups := make(map[string][]string)
	for _, p := range filePaths {
		dir := filepath.Dir(p)
		roomName := filepath.Base(dir)
		if roomName == "." || roomName == "" {
			roomName = "root"
		}
		groups[roomName] = append(groups[roomName], p)
	}
	return groups
}
