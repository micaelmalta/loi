---
room: core/index
see_also:
  - core/claims.md
  - core/notify.md
  - ../runtime.md
architectural_health: normal
security_tier: normal
hot_paths: cover.go, parse.go, tables.go
---

# LOI Room: core/index

Source paths: internal/index/

## Entries

# cover.go

DOES: Finds LOI room `.md` files that cover a given source file using either declared "Source paths:" lines or filename-stem search in the room body. Also provides helpers to extract markdown links, task file refs, governance table rows, gitignore-excluded dirs, and source dirs.
SYMBOLS:
- FindCoveringRooms(projectRoot, sourceFile string, strategy CoverStrategy) ([]string, error)
- ExtractSourcePaths(path string) ([]string, error)
- ExtractMDLinks(path string) ([]string, error)
- ExtractTaskFileRefs(path string) ([]string, error)
- CountEntries(path string) (int, error)
- ExtractChangedEntries(diff string) []ChangedEntry
- ParseGovernanceTable(text string) []GovEntry
- ParseGitignoreDirs(projectRoot string) (map[string]bool, error)
- FindSourceDirs(projectRoot string, excluded map[string]bool) ([]string, error)
- Types: CoverStrategy, ChangedEntry, GovEntry
PATTERNS: strategy-pattern
USE WHEN: Determining which room files are stale after a source change (CoverBySourcePaths) or locating rooms by filename stem during watcher dispatch (CoverByContent).

---

# parse.go

DOES: Parses and atomically updates YAML frontmatter in LOI `.md` files, handling scalar, quoted, inline-list, and block-list YAML forms. Also parses `proposal_metadata:` blocks and normalizes pattern names for semantic comparison.
SYMBOLS:
- ParseFrontmatter(path string) (*Frontmatter, error)
- UpdateFrontmatterField(path, key, value string) (bool, error)
- ParseProposalMetadata(path string) (map[string]interface{}, error)
- Types: Frontmatter
TYPE: Frontmatter { Room, SeeAlso, ArchitecturalHealth, SecurityTier, PatternAliases, LastValidated, StaleSlice, HotPaths, CommitteeNotes, Raw }
USE WHEN: Reading or patching any frontmatter field in a room file; extracting proposal provenance metadata.

---

# parse_test.go

DOES: Tests ParseFrontmatter (scalar, list, inline-list, quoted, missing frontmatter) and UpdateFrontmatterField (key replacement, key insertion, atomic write semantics).

---

# sourcepaths.go

DOES: Walks all non-underscore `.md` files under an index directory and returns the union of every declared "Source paths:" value across all room files; used by validate to check source coverage.
SYMBOLS:
- ExtractSourcePathsFromRooms(indexDir string) ([]string, error)

---

# tables.go

DOES: Parses TASK, PATTERN, and GOVERNANCE markdown tables from text by detecting section headings; diffs two `Tables` values at the row level; formats diffs as human-readable strings with +/-/~ prefixes. Also parses GOVERNANCE WATCHLIST rows into `GovEntry` slices.
SYMBOLS:
- ParseTables(text string) Tables
- DiffTables(old, new Tables) TableDiff
- FormatDiff(d TableDiff) string
- ParseGovernanceTable(text string) []GovEntry
- Types: TableRow, Tables, TableDiff, GovEntry
TYPE: Tables { Task, Pattern, Governance []TableRow }
PATTERNS: markdown-table-parser, diff-output
USE WHEN: Attaching structured change summaries to watcher notifications; detecting intent-field changes in a git diff; validating PATTERN table entries.

---

# tables_test.go

DOES: Tests ParseTables (section detection, header/separator skipping), DiffTables (added/removed/changed rows), and FormatDiff output.

---
