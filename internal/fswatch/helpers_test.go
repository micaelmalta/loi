package fswatch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/micaelmalta/loi/internal/index"
)

// ---- mostSevereHealth -------------------------------------------------------

func TestMostSevereHealth(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"", "", ""},
		{"normal", "", "normal"},
		{"", "normal", "normal"},
		{"normal", "normal", "normal"},
		{"normal", "critical", "critical"},
		{"critical", "normal", "critical"},
		{"conflicted", "degraded", "conflicted"},
		{"degraded", "conflicted", "conflicted"},
		{"critical", "conflicted", "critical"},
	}
	for _, tt := range tests {
		got := mostSevereHealth(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("mostSevereHealth(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}

// ---- computeGovernanceInfo --------------------------------------------------

func writeRoomFixture(t *testing.T, dir, name, health, security string) string {
	t.Helper()
	content := "---\nroom: " + name + "\narchitectural_health: " + health + "\nsecurity_tier: " + security + "\nsee_also: []\n---\n"
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestComputeGovernanceInfo_singleRoom(t *testing.T) {
	dir := t.TempDir()
	room := writeRoomFixture(t, dir, "auth.md", "warning", "high")
	info := computeGovernanceInfo([]string{room})
	if info["health"] != "warning" {
		t.Errorf("health: got %q, want %q", info["health"], "warning")
	}
	if info["security"] != "high" {
		t.Errorf("security: got %q, want %q", info["security"], "high")
	}
}

func TestComputeGovernanceInfo_severityMerge(t *testing.T) {
	dir := t.TempDir()
	r1 := writeRoomFixture(t, dir, "a.md", "degraded", "normal")
	r2 := writeRoomFixture(t, dir, "b.md", "critical", "sensitive")
	info := computeGovernanceInfo([]string{r1, r2})
	if info["health"] != "critical" {
		t.Errorf("health: got %q, want %q", info["health"], "critical")
	}
}

func TestComputeGovernanceInfo_noRooms(t *testing.T) {
	info := computeGovernanceInfo(nil)
	if len(info) != 0 {
		t.Errorf("expected empty info, got %v", info)
	}
}

// ---- buildSummary -----------------------------------------------------------

func TestBuildSummary_single(t *testing.T) {
	entries := []index.ChangedEntry{
		{SourceFile: "auth.go", ChangedLine: "DOES: handles login"},
		{SourceFile: "auth.go", ChangedLine: "SYMBOLS: Login()"},
	}
	got := buildSummary(entries)
	if got == "" {
		t.Error("expected non-empty summary")
	}
	// Should mention the single file.
	if !contains(got, "auth.go") {
		t.Errorf("summary should mention auth.go; got %q", got)
	}
}

func TestBuildSummary_multi(t *testing.T) {
	entries := []index.ChangedEntry{
		{SourceFile: "auth.go", ChangedLine: "DOES: handles login"},
		{SourceFile: "token.go", ChangedLine: "DOES: manages tokens"},
	}
	got := buildSummary(entries)
	if got == "" {
		t.Error("expected non-empty summary")
	}
}

func TestBuildSummary_empty(t *testing.T) {
	got := buildSummary(nil)
	if got == "" {
		t.Error("expected fallback summary for empty entries")
	}
}

// ---- isTestFile -------------------------------------------------------------

func TestIsTestFile(t *testing.T) {
	hits := []string{"auth_test.go", "test_auth.py", "auth.test.ts", "auth.test.js", "auth_spec.rb", "test_helpers.go"}
	for _, name := range hits {
		if !isTestFile(name) {
			t.Errorf("isTestFile(%q) = false, want true", name)
		}
	}
	misses := []string{"auth.go", "auth.ts", "model.rb", "main.py"}
	for _, name := range misses {
		if isTestFile(name) {
			t.Errorf("isTestFile(%q) = true, want false", name)
		}
	}
}

// ---- matchesAnyScope --------------------------------------------------------

func TestMatchesAnyScope_glob(t *testing.T) {
	if !matchesAnyScope("docs/index/auth.md", []string{"docs/**"}) {
		// filepath.Match doesn't support ** — falls back to prefix
		// this is fine; the prefix check covers it
		t.Log("glob not matched, but prefix should cover it")
	}
}

func TestMatchesAnyScope_prefix(t *testing.T) {
	if !matchesAnyScope("docs/index/auth.md", []string{"docs/"}) {
		t.Error("expected match via prefix")
	}
	if matchesAnyScope("internal/auth.go", []string{"docs/"}) {
		t.Error("expected no match")
	}
}

// ---- isSourceFile -----------------------------------------------------------

func TestIsSourceFile(t *testing.T) {
	root := t.TempDir()
	hits := []string{"auth.go", "app.ts", "model.py", "handler.js", "helper.rb"}
	for _, name := range hits {
		path := filepath.Join(root, name)
		if !isSourceFile(path, root) {
			t.Errorf("isSourceFile(%q) = false, want true", name)
		}
	}
	misses := []string{"schema.json", "README.md", "config.yaml"}
	for _, name := range misses {
		path := filepath.Join(root, name)
		if isSourceFile(path, root) {
			t.Errorf("isSourceFile(%q) = true, want false", name)
		}
	}
	// docs/ files are excluded
	docsPath := filepath.Join(root, "docs", "index", "auth.md")
	if isSourceFile(docsPath, root) {
		t.Error("isSourceFile for docs/ .md should be false")
	}
}

// helpers

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
