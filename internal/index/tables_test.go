package index

import (
	"strings"
	"testing"
)

// ---- ParseTables ------------------------------------------------------------

func TestParseTables(t *testing.T) {
	t.Run("TASK rows extracted", func(t *testing.T) {
		text := `
## TASK → LOAD

| Task | Load |
|------|------|
| Issue a UCAN token | auth/ucan.md |
| Rotate a token | auth/rotation.md |
`
		tables := ParseTables(text)
		if len(tables.Task) != 2 {
			t.Fatalf("Task rows: got %d, want 2; rows=%v", len(tables.Task), tables.Task)
		}
		if tables.Task[0][0] != "Issue a UCAN token" {
			t.Errorf("Task[0][0]: got %q, want %q", tables.Task[0][0], "Issue a UCAN token")
		}
		if tables.Task[0][1] != "auth/ucan.md" {
			t.Errorf("Task[0][1]: got %q, want %q", tables.Task[0][1], "auth/ucan.md")
		}
	})

	t.Run("PATTERN rows extracted", func(t *testing.T) {
		text := `
## PATTERN → LOAD

| Pattern | Load |
|---------|------|
| Token rotation | auth/ucan.md |
`
		tables := ParseTables(text)
		if len(tables.Pattern) != 1 {
			t.Fatalf("Pattern rows: got %d, want 1", len(tables.Pattern))
		}
		if tables.Pattern[0][0] != "Token rotation" {
			t.Errorf("Pattern[0][0]: got %q, want %q", tables.Pattern[0][0], "Token rotation")
		}
	})

	t.Run("GOVERNANCE rows extracted", func(t *testing.T) {
		text := `
## 🚨 GOVERNANCE WATCHLIST

| Room | Health | Security | Note |
|------|--------|----------|------|
| auth/ucan.md | warning | high | Some note |
`
		tables := ParseTables(text)
		if len(tables.Governance) != 1 {
			t.Fatalf("Governance rows: got %d, want 1", len(tables.Governance))
		}
		if tables.Governance[0][0] != "auth/ucan.md" {
			t.Errorf("Governance[0][0]: got %q, want %q", tables.Governance[0][0], "auth/ucan.md")
		}
	})

	t.Run("separator and header rows skipped", func(t *testing.T) {
		text := `
## TASK → LOAD

| Task | Load |
|------|------|
| Real task | real.md |
`
		tables := ParseTables(text)
		// Should only have 1 row — the header and separator are skipped
		if len(tables.Task) != 1 {
			t.Errorf("Task rows: got %d, want 1; rows=%v", len(tables.Task), tables.Task)
		}
		if tables.Task[0][0] == "Task" {
			t.Error("header row was included in results")
		}
	})

	t.Run("empty text returns zero-length slices", func(t *testing.T) {
		tables := ParseTables("")
		if len(tables.Task) != 0 {
			t.Errorf("Task: got %d rows, want 0", len(tables.Task))
		}
		if len(tables.Pattern) != 0 {
			t.Errorf("Pattern: got %d rows, want 0", len(tables.Pattern))
		}
		if len(tables.Governance) != 0 {
			t.Errorf("Governance: got %d rows, want 0", len(tables.Governance))
		}
	})

	t.Run("multiple tables in one file", func(t *testing.T) {
		text := `
## TASK → LOAD

| Task | Load |
|------|------|
| Task one | one.md |
| Task two | two.md |

## PATTERN → LOAD

| Pattern | Load |
|---------|------|
| Debounce | watcher.md |

## 🚨 GOVERNANCE WATCHLIST

| Room | Health | Security | Note |
|------|--------|----------|------|
| ops/deploy.md | critical | sensitive | Note |
`
		tables := ParseTables(text)
		if len(tables.Task) != 2 {
			t.Errorf("Task: got %d, want 2", len(tables.Task))
		}
		if len(tables.Pattern) != 1 {
			t.Errorf("Pattern: got %d, want 1", len(tables.Pattern))
		}
		if len(tables.Governance) != 1 {
			t.Errorf("Governance: got %d, want 1", len(tables.Governance))
		}
	})
}

// ---- ParseGovernanceTable ---------------------------------------------------

func TestParseGovernanceTable(t *testing.T) {
	t.Run("backtick stripping", func(t *testing.T) {
		text := `
## GOVERNANCE WATCHLIST

| Room | Health | Security | Note |
|------|--------|----------|------|
| ` + "`auth/ucan.md`" + ` | ` + "`warning`" + ` | ` + "`high`" + ` | Some note |
`
		entries := ParseGovernanceTable(text)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1", len(entries))
		}
		if entries[0].Room != "auth/ucan.md" {
			t.Errorf("Room: got %q, want %q", entries[0].Room, "auth/ucan.md")
		}
		if entries[0].Health != "warning" {
			t.Errorf("Health: got %q, want %q", entries[0].Health, "warning")
		}
		if entries[0].Security != "high" {
			t.Errorf("Security: got %q, want %q", entries[0].Security, "high")
		}
	})

	t.Run("lowercase normalization", func(t *testing.T) {
		text := `
## GOVERNANCE WATCHLIST

| Room | Health | Security | Note |
|------|--------|----------|------|
| ops/deploy.md | CRITICAL | SENSITIVE | Note |
`
		entries := ParseGovernanceTable(text)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1", len(entries))
		}
		if entries[0].Health != "critical" {
			t.Errorf("Health: got %q, want %q", entries[0].Health, "critical")
		}
		if entries[0].Security != "sensitive" {
			t.Errorf("Security: got %q, want %q", entries[0].Security, "sensitive")
		}
	})

	t.Run("heading variants with emoji prefix", func(t *testing.T) {
		text := `
## 🚨 GOVERNANCE WATCHLIST

| Room | Health | Security | Note |
|------|--------|----------|------|
| auth/ucan.md | warning | high | note |
`
		entries := ParseGovernanceTable(text)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1 (emoji prefix should work)", len(entries))
		}
	})

	t.Run("no GOVERNANCE section returns empty", func(t *testing.T) {
		text := `
## TASK → LOAD

| Task | Load |
|------|------|
| Do thing | thing.md |
`
		entries := ParseGovernanceTable(text)
		if len(entries) != 0 {
			t.Errorf("entries: got %d, want 0", len(entries))
		}
	})

	t.Run("note field preserved", func(t *testing.T) {
		text := `
## GOVERNANCE WATCHLIST

| Room | Health | Security | Committee Note |
|------|--------|----------|----------------|
| auth/ucan.md | warning | high | "Fixed shell quoting" |
`
		entries := ParseGovernanceTable(text)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1", len(entries))
		}
		if entries[0].Note == "" {
			t.Error("Note should be non-empty")
		}
	})
}

// ---- DiffTables -------------------------------------------------------------

func TestDiffTables(t *testing.T) {
	t.Run("added row detected", func(t *testing.T) {
		old := Tables{}
		new := Tables{
			Task: []TableRow{{"New task", "new.md"}},
		}
		diff := DiffTables(old, new)
		if len(diff.Task.Added) != 1 {
			t.Fatalf("Task.Added: got %d, want 1", len(diff.Task.Added))
		}
		if diff.Task.Added[0][0] != "New task" {
			t.Errorf("Added[0][0]: got %q, want %q", diff.Task.Added[0][0], "New task")
		}
	})

	t.Run("removed row detected", func(t *testing.T) {
		old := Tables{
			Task: []TableRow{{"Old task", "old.md"}},
		}
		new := Tables{}
		diff := DiffTables(old, new)
		if len(diff.Task.Removed) != 1 {
			t.Fatalf("Task.Removed: got %d, want 1", len(diff.Task.Removed))
		}
		if diff.Task.Removed[0][0] != "Old task" {
			t.Errorf("Removed[0][0]: got %q, want %q", diff.Task.Removed[0][0], "Old task")
		}
	})

	t.Run("changed row detected", func(t *testing.T) {
		old := Tables{
			Task: []TableRow{{"Existing task", "old.md"}},
		}
		new := Tables{
			Task: []TableRow{{"Existing task", "new.md"}},
		}
		diff := DiffTables(old, new)
		if len(diff.Task.Changed) != 1 {
			t.Fatalf("Task.Changed: got %d, want 1", len(diff.Task.Changed))
		}
		pair := diff.Task.Changed[0]
		if pair[0][1] != "old.md" {
			t.Errorf("Changed old[1]: got %q, want %q", pair[0][1], "old.md")
		}
		if pair[1][1] != "new.md" {
			t.Errorf("Changed new[1]: got %q, want %q", pair[1][1], "new.md")
		}
	})

	t.Run("unchanged row not in diff", func(t *testing.T) {
		row := TableRow{"Same task", "same.md"}
		old := Tables{Task: []TableRow{row}}
		new := Tables{Task: []TableRow{row}}
		diff := DiffTables(old, new)
		if len(diff.Task.Added) != 0 || len(diff.Task.Removed) != 0 || len(diff.Task.Changed) != 0 {
			t.Errorf("expected no changes for identical row; got added=%d removed=%d changed=%d",
				len(diff.Task.Added), len(diff.Task.Removed), len(diff.Task.Changed))
		}
	})

	t.Run("empty old produces only adds", func(t *testing.T) {
		new := Tables{
			Pattern: []TableRow{{"Token rotation", "auth/ucan.md"}},
		}
		diff := DiffTables(Tables{}, new)
		if len(diff.Pattern.Added) != 1 {
			t.Errorf("Pattern.Added: got %d, want 1", len(diff.Pattern.Added))
		}
		if len(diff.Pattern.Removed) != 0 {
			t.Errorf("Pattern.Removed: got %d, want 0", len(diff.Pattern.Removed))
		}
	})

	t.Run("empty new produces only removes", func(t *testing.T) {
		old := Tables{
			Pattern: []TableRow{{"Token rotation", "auth/ucan.md"}},
		}
		diff := DiffTables(old, Tables{})
		if len(diff.Pattern.Removed) != 1 {
			t.Errorf("Pattern.Removed: got %d, want 1", len(diff.Pattern.Removed))
		}
		if len(diff.Pattern.Added) != 0 {
			t.Errorf("Pattern.Added: got %d, want 0", len(diff.Pattern.Added))
		}
	})
}

// ---- FormatDiff -------------------------------------------------------------

func TestFormatDiff(t *testing.T) {
	t.Run("empty string on zero changes", func(t *testing.T) {
		diff := DiffTables(Tables{}, Tables{})
		out := FormatDiff(diff)
		if out != "" {
			t.Errorf("FormatDiff on empty diff: got %q, want empty string", out)
		}
	})

	t.Run("added row uses + prefix", func(t *testing.T) {
		diff := TableDiff{
			Task: TableChanges{
				Added: []TableRow{{"Issue or rotate a UCAN token", "auth/ucan.md"}},
			},
		}
		out := FormatDiff(diff)
		if !strings.Contains(out, "+ added:") {
			t.Errorf("FormatDiff: missing '+ added:' in output:\n%s", out)
		}
		if !strings.Contains(out, "TASK changes") {
			t.Errorf("FormatDiff: missing 'TASK changes' section header:\n%s", out)
		}
	})

	t.Run("removed row uses - prefix", func(t *testing.T) {
		diff := TableDiff{
			Task: TableChanges{
				Removed: []TableRow{{"Old task", "old.md"}},
			},
		}
		out := FormatDiff(diff)
		if !strings.Contains(out, "- removed:") {
			t.Errorf("FormatDiff: missing '- removed:' in output:\n%s", out)
		}
	})

	t.Run("changed row uses ~ prefix", func(t *testing.T) {
		diff := TableDiff{
			Pattern: TableChanges{
				Changed: [][2]TableRow{
					{{"Token rotation", "auth/old.md"}, {"Token rotation", "auth/ucan.md"}},
				},
			},
		}
		out := FormatDiff(diff)
		if !strings.Contains(out, "~ changed:") {
			t.Errorf("FormatDiff: missing '~ changed:' in output:\n%s", out)
		}
		if !strings.Contains(out, "PATTERN changes") {
			t.Errorf("FormatDiff: missing 'PATTERN changes' section header:\n%s", out)
		}
	})

	t.Run("section headers present only for non-empty sections", func(t *testing.T) {
		diff := TableDiff{
			Task: TableChanges{
				Added: []TableRow{{"New task", "new.md"}},
			},
			// Pattern and Governance are empty
		}
		out := FormatDiff(diff)
		if !strings.Contains(out, "TASK changes") {
			t.Errorf("FormatDiff: missing TASK section")
		}
		if strings.Contains(out, "PATTERN changes") {
			t.Errorf("FormatDiff: unexpected PATTERN section in output")
		}
		if strings.Contains(out, "GOVERNANCE changes") {
			t.Errorf("FormatDiff: unexpected GOVERNANCE section in output")
		}
	})
}

// ---- ExtractChangedEntries --------------------------------------------------

func TestExtractChangedEntries(t *testing.T) {
	t.Run("DOES: change detected", func(t *testing.T) {
		diff := `diff --git a/docs/index/auth/ucan.md b/docs/index/auth/ucan.md
--- a/docs/index/auth/ucan.md
+++ b/docs/index/auth/ucan.md
@@ -10,6 +10,7 @@
 # token.go
+DOES: Issues UCAN tokens with expiry and delegation chain validation.
`
		entries := ExtractChangedEntries(diff)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1; %v", len(entries), entries)
		}
		if entries[0].SourceFile != "token.go" {
			t.Errorf("SourceFile: got %q, want %q", entries[0].SourceFile, "token.go")
		}
		if !strings.Contains(entries[0].ChangedLine, "DOES:") {
			t.Errorf("ChangedLine should contain DOES:, got %q", entries[0].ChangedLine)
		}
	})

	t.Run("SYMBOLS: change detected", func(t *testing.T) {
		diff := `@@ -5,4 +5,5 @@
 # auth.go
+SYMBOLS: IssueToken, RevokeToken, ValidateToken
`
		entries := ExtractChangedEntries(diff)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1; %v", len(entries), entries)
		}
		if !strings.Contains(entries[0].ChangedLine, "SYMBOLS:") {
			t.Errorf("ChangedLine should contain SYMBOLS:, got %q", entries[0].ChangedLine)
		}
	})

	t.Run("table row with path detected", func(t *testing.T) {
		diff := `@@ -20,3 +20,4 @@
+| auth/ucan.go | Handles UCAN token lifecycle |
`
		entries := ExtractChangedEntries(diff)
		if len(entries) != 1 {
			t.Fatalf("entries: got %d, want 1; %v", len(entries), entries)
		}
		if entries[0].SourceFile != "auth/ucan.go" {
			t.Errorf("SourceFile: got %q, want %q", entries[0].SourceFile, "auth/ucan.go")
		}
	})

	t.Run("unrelated lines ignored", func(t *testing.T) {
		diff := `@@ -1,3 +1,4 @@
 # Overview
+This is just a description paragraph with no intent fields.
-Old description removed.
`
		entries := ExtractChangedEntries(diff)
		if len(entries) != 0 {
			t.Errorf("expected 0 entries from unrelated lines, got %d: %v", len(entries), entries)
		}
	})

	t.Run("multiple intent field changes", func(t *testing.T) {
		diff := `@@ -5,4 +5,7 @@
 # middleware.go
+DOES: Handles HTTP middleware chain.
+TYPE: middleware
+INTERFACE: func(next http.Handler) http.Handler
`
		entries := ExtractChangedEntries(diff)
		if len(entries) != 3 {
			t.Fatalf("entries: got %d, want 3; %v", len(entries), entries)
		}
	})
}
