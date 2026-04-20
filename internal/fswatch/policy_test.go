package fswatch

import (
	"path/filepath"
	"testing"

	"github.com/micaelmalta/loi/internal/claims"
)


func baseCfg(projectRoot string, policy PolicyTier) WatcherConfig {
	return WatcherConfig{
		ProjectRoot:        projectRoot,
		Policy:             policy,
		BlockGovernanceSec: map[string]bool{"sensitive": true, "high": true},
	}
}

func TestCheckPolicy_notifyOnly(t *testing.T) {
	cfg := baseCfg(t.TempDir(), PolicyNotifyOnly)
	allowed, reason := checkPolicy(cfg, []string{"any.md"}, map[string]string{})
	if allowed {
		t.Error("expected blocked")
	}
	if reason == "" {
		t.Error("expected non-empty reason")
	}
}

func TestCheckPolicy_draftOnly(t *testing.T) {
	cfg := baseCfg(t.TempDir(), PolicyDraftOnly)
	allowed, _ := checkPolicy(cfg, []string{"any.md"}, map[string]string{})
	if allowed {
		t.Error("expected blocked")
	}
}

func TestCheckPolicy_docsSafe_pass(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyDocsSafe)
	room := filepath.Join(root, "docs", "index", "auth.md")
	allowed, _ := checkPolicy(cfg, []string{room}, map[string]string{})
	if !allowed {
		t.Error("expected allowed: room is under docs/")
	}
}

func TestCheckPolicy_docsSafe_fail(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyDocsSafe)
	room := filepath.Join(root, "internal", "auth", "auth.go")
	allowed, reason := checkPolicy(cfg, []string{room}, map[string]string{})
	if allowed {
		t.Errorf("expected blocked; reason: %s", reason)
	}
}

func TestCheckPolicy_testsSafe_pass(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyTestsSafe)
	allowed, _ := checkPolicy(cfg, []string{"auth_test.go"}, map[string]string{})
	if !allowed {
		t.Error("expected allowed: test file")
	}
}

func TestCheckPolicy_testsSafe_fail(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyTestsSafe)
	allowed, reason := checkPolicy(cfg, []string{"auth.go"}, map[string]string{})
	if allowed {
		t.Errorf("expected blocked; reason: %s", reason)
	}
}

func TestCheckPolicy_scopedCodeSafe_pass(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyScopedCodeSafe)
	cfg.AllowedScopes = []string{"docs/"}
	room := filepath.Join("docs", "index", "auth.md")
	allowed, _ := checkPolicy(cfg, []string{room}, map[string]string{})
	if !allowed {
		t.Error("expected allowed: room matches scope prefix")
	}
}

func TestCheckPolicy_scopedCodeSafe_fail(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyScopedCodeSafe)
	cfg.AllowedScopes = []string{"docs/"}
	room := filepath.Join("internal", "auth.go")
	allowed, reason := checkPolicy(cfg, []string{room}, map[string]string{})
	if allowed {
		t.Errorf("expected blocked; reason: %s", reason)
	}
}

func TestCheckPolicy_fullAuto_allowedUnlessGovCritical(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyFullAuto)
	allowed, _ := checkPolicy(cfg, []string{"any.md"}, map[string]string{})
	if !allowed {
		t.Error("expected allowed: full-auto with no governance flags")
	}
}

func TestCheckPolicy_blockedByHealthCritical(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyFullAuto)
	allowed, reason := checkPolicy(cfg, []string{"any.md"}, map[string]string{"health": "critical"})
	if allowed {
		t.Errorf("expected blocked by critical health; reason: %s", reason)
	}
}

func TestCheckPolicy_blockedBySecurityTier(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyFullAuto)
	allowed, reason := checkPolicy(cfg, []string{"any.md"}, map[string]string{"security": "sensitive"})
	if allowed {
		t.Errorf("expected blocked by sensitive security tier; reason: %s", reason)
	}
}

func TestCheckPolicy_blockedByClaimConflict(t *testing.T) {
	root := t.TempDir()
	cfg := baseCfg(root, PolicyFullAuto)

	// Inject an edit claim for "auth" in the claims store.
	cs := claims.NewClaimsStore(root)
	if err := cs.AddClaim(claims.Claim{
		ScopeType: "room",
		ScopeID:   "auth",
		Repo:      "test",
		AgentID:   "agent-1",
		SessionID: "sess-1",
		Intent:    "edit",
	}); err != nil {
		t.Fatalf("AddClaim: %v", err)
	}

	roomPath := filepath.Join(root, "docs", "index", "auth.md")
	allowed, reason := checkPolicy(cfg, []string{roomPath}, map[string]string{})
	if allowed {
		t.Errorf("expected blocked by claim conflict; reason: %s", reason)
	}
	_ = reason
}
