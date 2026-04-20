package cmd_test

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGovernance_noFlags_allRooms(t *testing.T) {
	root := initGitRepo(t)
	stdout, _, code := runLOI(t, root, "governance")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	// The fixture has auth/session.md flagged in the watchlist.
	if !strings.Contains(stdout, "session") {
		t.Errorf("expected session.md in governance output; got:\n%s", stdout)
	}
}

func TestGovernance_filterBySecurity(t *testing.T) {
	root := initGitRepo(t)
	stdout, _, code := runLOI(t, root, "governance", "--security", "sensitive")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	// session.md is marked sensitive — should appear.
	if !strings.Contains(stdout, "session") {
		t.Errorf("expected session.md in sensitive-filtered output; got:\n%s", stdout)
	}
}

func TestGovernance_formatJSON(t *testing.T) {
	root := initGitRepo(t)
	stdout, _, code := runLOI(t, root, "governance", "--format", "json")
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}
	var arr []any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &arr); err != nil {
		t.Fatalf("governance --format json: not valid JSON array: %v\noutput: %s", err, stdout)
	}
}
