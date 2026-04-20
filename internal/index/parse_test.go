package index

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp writes content to a temp file and returns its path.
func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTemp: %v", err)
	}
	return path
}

// ---- ParseFrontmatter -------------------------------------------------------

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		expectNil bool
		check     func(t *testing.T, fm *Frontmatter)
	}{
		{
			name:      "nil when no frontmatter",
			content:   "# Just a heading\n\nSome body text.\n",
			expectNil: true,
		},
		{
			name: "scalar fields",
			content: `---
room: auth/ucan.md
architectural_health: warning
security_tier: high
last_validated: 2026-04-10
---
# Body
`,
			check: func(t *testing.T, fm *Frontmatter) {
				if fm == nil {
					t.Fatal("expected non-nil Frontmatter")
				}
				if fm.Room != "auth/ucan.md" {
					t.Errorf("Room: got %q, want %q", fm.Room, "auth/ucan.md")
				}
				if fm.ArchitecturalHealth != "warning" {
					t.Errorf("ArchitecturalHealth: got %q, want %q", fm.ArchitecturalHealth, "warning")
				}
				if fm.SecurityTier != "high" {
					t.Errorf("SecurityTier: got %q, want %q", fm.SecurityTier, "high")
				}
				if fm.LastValidated != "2026-04-10" {
					t.Errorf("LastValidated: got %q, want %q", fm.LastValidated, "2026-04-10")
				}
			},
		},
		{
			name: "quoted values",
			content: `---
room: "auth/ucan.md"
committee_notes: 'Some note here'
---
`,
			check: func(t *testing.T, fm *Frontmatter) {
				if fm == nil {
					t.Fatal("expected non-nil Frontmatter")
				}
				if fm.Room != "auth/ucan.md" {
					t.Errorf("Room: got %q, want %q", fm.Room, "auth/ucan.md")
				}
				if fm.CommitteeNotes != "Some note here" {
					t.Errorf("CommitteeNotes: got %q, want %q", fm.CommitteeNotes, "Some note here")
				}
			},
		},
		{
			name: "inline list values for see_also",
			content: `---
room: auth/ucan.md
see_also: ["auth/tokens.md", "auth/rbac.md"]
---
`,
			check: func(t *testing.T, fm *Frontmatter) {
				if fm == nil {
					t.Fatal("expected non-nil Frontmatter")
				}
				if len(fm.SeeAlso) != 2 {
					t.Fatalf("SeeAlso len: got %d, want 2", len(fm.SeeAlso))
				}
				if fm.SeeAlso[0] != "auth/tokens.md" {
					t.Errorf("SeeAlso[0]: got %q, want %q", fm.SeeAlso[0], "auth/tokens.md")
				}
				if fm.SeeAlso[1] != "auth/rbac.md" {
					t.Errorf("SeeAlso[1]: got %q, want %q", fm.SeeAlso[1], "auth/rbac.md")
				}
			},
		},
		{
			name: "block list for pattern_aliases",
			content: `---
room: auth/ucan.md
pattern_aliases:
  - Token rotation
  - UCAN refresh
---
`,
			check: func(t *testing.T, fm *Frontmatter) {
				if fm == nil {
					t.Fatal("expected non-nil Frontmatter")
				}
				if len(fm.PatternAliases) != 2 {
					t.Fatalf("PatternAliases len: got %d, want 2", len(fm.PatternAliases))
				}
				if fm.PatternAliases[0] != "Token rotation" {
					t.Errorf("PatternAliases[0]: got %q, want %q", fm.PatternAliases[0], "Token rotation")
				}
				if fm.PatternAliases[1] != "UCAN refresh" {
					t.Errorf("PatternAliases[1]: got %q, want %q", fm.PatternAliases[1], "UCAN refresh")
				}
			},
		},
		{
			name: "unknown keys go to Raw",
			content: `---
room: some.md
custom_field: custom_value
another: 42
---
`,
			check: func(t *testing.T, fm *Frontmatter) {
				if fm == nil {
					t.Fatal("expected non-nil Frontmatter")
				}
				if fm.Raw["custom_field"] != "custom_value" {
					t.Errorf("Raw[custom_field]: got %q, want %q", fm.Raw["custom_field"], "custom_value")
				}
				if fm.Raw["another"] != "42" {
					t.Errorf("Raw[another]: got %q, want %q", fm.Raw["another"], "42")
				}
			},
		},
		{
			name: "architectural_health and security_tier",
			content: `---
room: ops/deploy.md
architectural_health: critical
security_tier: sensitive
---
`,
			check: func(t *testing.T, fm *Frontmatter) {
				if fm == nil {
					t.Fatal("expected non-nil Frontmatter")
				}
				if fm.ArchitecturalHealth != "critical" {
					t.Errorf("ArchitecturalHealth: got %q, want %q", fm.ArchitecturalHealth, "critical")
				}
				if fm.SecurityTier != "sensitive" {
					t.Errorf("SecurityTier: got %q, want %q", fm.SecurityTier, "sensitive")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.content)
			fm, err := ParseFrontmatter(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.expectNil {
				if fm != nil {
					t.Errorf("expected nil, got non-nil Frontmatter")
				}
				return
			}
			if tt.check != nil {
				tt.check(t, fm)
			}
		})
	}
}

// ---- Normalize --------------------------------------------------------------

// TestNormalize verifies the normalization matches the Python spec:
// lowercase, replace non-word chars with space, collapse whitespace, trim.
// Note: underscore (_) is a word char (\w) so it is preserved.
// Punctuation like comma and exclamation are replaced by space then collapsed.
func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		// Basic lowercase
		{"Token rotation", "token rotation"},
		{"ALL CAPS NOW", "all caps now"},
		// Punctuation replaced by space, then collapsed
		{"Hello, World!", "hello world"},
		// Underscore is a \w char — preserved
		{"with_underscore", "with_underscore"},
		// Leading/trailing whitespace trimmed
		{"  trimmed  ", "trimmed"},
		// Empty string
		{"", ""},
		// Alphanumeric unchanged
		{"abc123", "abc123"},
		// Multiple spaces collapsed
		{"a  b   c", "a b c"},
		// Mixed punctuation
		{"foo!bar@baz", "foo bar baz"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := Normalize(tt.input)
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestNormalizeIdempotent verifies Normalize is idempotent.
func TestNormalizeIdempotent(t *testing.T) {
	inputs := []string{
		"Token rotation without service restart",
		"Hello, World!",
		"ALL CAPS",
		"with_underscore",
		"",
	}
	for _, input := range inputs {
		once := Normalize(input)
		twice := Normalize(once)
		if once != twice {
			t.Errorf("Normalize not idempotent for %q: first=%q, second=%q", input, once, twice)
		}
	}
}

// ---- CountEntries -----------------------------------------------------------

func TestCountEntries(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "zero when no entries",
			content: "# Just a heading\n\nSome body text without entries.\n",
			want:    0,
		},
		{
			name: "counts filename.ext headings",
			content: `# auth.go

DOES: Handles authentication.

# token.go

DOES: Manages tokens.

# middleware.go

DOES: Request middleware.
`,
			want: 3,
		},
		{
			name: "does not count headings without extension",
			content: `# Overview

Some text.

# auth.go

DOES: Handles auth.

## Sub-heading

# plain-heading
`,
			want: 1,
		},
		{
			name:    "empty file",
			content: "",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.content)
			got, err := CountEntries(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("CountEntries: got %d, want %d", got, tt.want)
			}
		})
	}
}

// ---- ExtractSourcePaths -----------------------------------------------------

func TestExtractSourcePaths(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "no match returns empty",
			content: "# Room\n\nNo source paths here.\n",
			want:    nil,
		},
		{
			name:    "single path",
			content: "Source paths: internal/auth\n",
			want:    []string{"internal/auth"},
		},
		{
			name:    "trailing slash stripped",
			content: "Source paths: internal/auth/\n",
			want:    []string{"internal/auth"},
		},
		{
			name:    "comma-separated paths",
			content: "Source paths: internal/auth, internal/token, pkg/middleware\n",
			want:    []string{"internal/auth", "internal/token", "pkg/middleware"},
		},
		{
			name:    "case-insensitive match",
			content: "SOURCE PATHS: internal/auth\n",
			want:    []string{"internal/auth"},
		},
		{
			name:    "singular form",
			content: "Source path: internal/auth\n",
			want:    []string{"internal/auth"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.content)
			got, err := ExtractSourcePaths(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ExtractSourcePaths: got %v, want %v", got, tt.want)
			}
			for i, w := range tt.want {
				if got[i] != w {
					t.Errorf("ExtractSourcePaths[%d]: got %q, want %q", i, got[i], w)
				}
			}
		})
	}
}

// ---- ExtractMDLinks ---------------------------------------------------------

func TestExtractMDLinks(t *testing.T) {
	tests := []struct {
		name    string
		content string
		wantLen int
		wantAny []string
	}{
		{
			name:    "finds md paths in table cells",
			content: "| Issue token | auth/ucan.md |\n| Rotate key  | auth/rotation.md |\n",
			wantLen: 2,
			wantAny: []string{"auth/ucan.md", "auth/rotation.md"},
		},
		{
			name:    "no md links",
			content: "Just plain text here.\n",
			wantLen: 0,
		},
		{
			name:    "single md link",
			content: "See also: auth/tokens.md for more.\n",
			wantLen: 1,
			wantAny: []string{"auth/tokens.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTemp(t, tt.content)
			got, err := ExtractMDLinks(path)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("ExtractMDLinks: got %v (len=%d), want len=%d", got, len(got), tt.wantLen)
			}
			for _, w := range tt.wantAny {
				found := false
				for _, g := range got {
					if g == w {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("ExtractMDLinks: missing %q in %v", w, got)
				}
			}
		})
	}
}

// ---- UpdateFrontmatterField -------------------------------------------------

func TestUpdateFrontmatterField(t *testing.T) {
	t.Run("updates existing key", func(t *testing.T) {
		content := `---
room: auth/ucan.md
architectural_health: normal
---
# Body
`
		path := writeTemp(t, content)
		changed, err := UpdateFrontmatterField(path, "architectural_health", "warning")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Error("expected changed=true")
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "architectural_health: warning") {
			t.Errorf("file does not contain updated value; got:\n%s", data)
		}
	})

	t.Run("inserts missing key", func(t *testing.T) {
		content := `---
room: auth/ucan.md
---
# Body
`
		path := writeTemp(t, content)
		changed, err := UpdateFrontmatterField(path, "architectural_health", "warning")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !changed {
			t.Error("expected changed=true")
		}
		data, _ := os.ReadFile(path)
		if !strings.Contains(string(data), "architectural_health: warning") {
			t.Errorf("file does not contain inserted key; got:\n%s", data)
		}
	})

	t.Run("no-op when value unchanged", func(t *testing.T) {
		content := `---
room: auth/ucan.md
architectural_health: warning
---
# Body
`
		path := writeTemp(t, content)
		changed, err := UpdateFrontmatterField(path, "architectural_health", "warning")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if changed {
			t.Error("expected changed=false when value is already set")
		}
	})
}
