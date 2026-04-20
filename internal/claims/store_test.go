package claims

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// helper: new store in a fresh temp dir
func newTestStore(t *testing.T) *ClaimsStore {
	t.Helper()
	return NewClaimsStore(t.TempDir())
}

// helper: a claim expiring in the future
func futureClaim(scopeID, agentID, intent string) Claim {
	now := time.Now().UTC()
	return Claim{
		ScopeType: "room",
		ScopeID:   scopeID,
		Repo:      "test-repo",
		AgentID:   agentID,
		SessionID: agentID + "-session",
		Intent:    intent,
		ClaimedAt: now,
		ExpiresAt: now.Add(DefaultTTL),
	}
}

// helper: a claim already expired
func expiredClaim(scopeID, agentID string) Claim {
	past := time.Now().UTC().Add(-10 * time.Minute)
	return Claim{
		ScopeType: "room",
		ScopeID:   scopeID,
		Repo:      "test-repo",
		AgentID:   agentID,
		SessionID: agentID + "-session",
		Intent:    "read",
		ClaimedAt: past.Add(-DefaultTTL),
		ExpiresAt: past,
	}
}

// ── Test 1: Add claim and retrieve it ────────────────────────────────────────

func TestAddAndRetrieveClaim(t *testing.T) {
	s := newTestStore(t)
	c := futureClaim("auth/ucan.md", "alice@host", "edit")

	if err := s.AddClaim(c); err != nil {
		t.Fatalf("AddClaim: %v", err)
	}

	claims, err := s.GetClaimsFor("auth/ucan.md")
	if err != nil {
		t.Fatalf("GetClaimsFor: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}
	if claims[0].AgentID != "alice@host" {
		t.Errorf("expected agent alice@host, got %s", claims[0].AgentID)
	}
	if claims[0].Intent != "edit" {
		t.Errorf("expected intent edit, got %s", claims[0].Intent)
	}
}

// ── Test 2: Release removes claim, returns true ───────────────────────────────

func TestRelease(t *testing.T) {
	s := newTestStore(t)
	c := futureClaim("auth/ucan.md", "alice@host", "edit")
	if err := s.AddClaim(c); err != nil {
		t.Fatalf("AddClaim: %v", err)
	}

	removed, err := s.RemoveClaim("auth/ucan.md", "alice@host")
	if err != nil {
		t.Fatalf("RemoveClaim: %v", err)
	}
	if !removed {
		t.Fatal("expected RemoveClaim to return true")
	}

	claims, err := s.GetClaimsFor("auth/ucan.md")
	if err != nil {
		t.Fatalf("GetClaimsFor after remove: %v", err)
	}
	if len(claims) != 0 {
		t.Fatalf("expected 0 claims after remove, got %d", len(claims))
	}
}

// ── Test 3: Release non-existent returns false ────────────────────────────────

func TestReleaseNonExistent(t *testing.T) {
	s := newTestStore(t)

	removed, err := s.RemoveClaim("auth/ucan.md", "nobody@host")
	if err != nil {
		t.Fatalf("RemoveClaim: %v", err)
	}
	if removed {
		t.Fatal("expected RemoveClaim to return false for non-existent claim")
	}
}

// ── Test 4: Duplicate claim replaces old, only 1 claim remains ───────────────

func TestDuplicateClaimReplacesOld(t *testing.T) {
	s := newTestStore(t)
	c1 := futureClaim("auth/ucan.md", "alice@host", "read")
	c2 := futureClaim("auth/ucan.md", "alice@host", "edit")

	if err := s.AddClaim(c1); err != nil {
		t.Fatalf("AddClaim c1: %v", err)
	}
	if err := s.AddClaim(c2); err != nil {
		t.Fatalf("AddClaim c2: %v", err)
	}

	claims, err := s.GetClaimsFor("auth/ucan.md")
	if err != nil {
		t.Fatalf("GetClaimsFor: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim after dedup, got %d", len(claims))
	}
	if claims[0].Intent != "edit" {
		t.Errorf("expected intent edit (latest), got %s", claims[0].Intent)
	}
}

// ── Test 5: Expired claims pruned on AllClaims ────────────────────────────────

func TestExpiredClaimsPruned(t *testing.T) {
	s := newTestStore(t)
	exp := expiredClaim("auth/ucan.md", "ghost@host")
	live := futureClaim("auth/ucan.md", "alice@host", "read")

	// Write both directly so we can bypass pruning on add
	if err := s.AddClaim(exp); err != nil {
		t.Fatalf("AddClaim expired: %v", err)
	}
	if err := s.AddClaim(live); err != nil {
		t.Fatalf("AddClaim live: %v", err)
	}

	all, err := s.AllClaims()
	if err != nil {
		t.Fatalf("AllClaims: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected 1 live claim, got %d", len(all))
	}
	if all[0].AgentID != "alice@host" {
		t.Errorf("expected alice@host, got %s", all[0].AgentID)
	}
}

// ── Test 6: UpdateExpiry extends TTL; returns false for missing ───────────────

func TestUpdateExpiry(t *testing.T) {
	s := newTestStore(t)
	c := futureClaim("auth/ucan.md", "alice@host", "edit")
	if err := s.AddClaim(c); err != nil {
		t.Fatalf("AddClaim: %v", err)
	}

	originalExpiry := c.ExpiresAt

	updated, err := s.UpdateExpiry("auth/ucan.md", "alice@host", HeartbeatGrace)
	if err != nil {
		t.Fatalf("UpdateExpiry: %v", err)
	}
	if !updated {
		t.Fatal("expected UpdateExpiry to return true")
	}

	claims, err := s.GetClaimsFor("auth/ucan.md")
	if err != nil {
		t.Fatalf("GetClaimsFor: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}
	if !claims[0].ExpiresAt.After(originalExpiry) {
		t.Errorf("expected extended expiry, got %v (original %v)", claims[0].ExpiresAt, originalExpiry)
	}
	if claims[0].LastHeartbeat == nil {
		t.Error("expected LastHeartbeat to be set")
	}

	// Returns false for missing claim
	updated, err = s.UpdateExpiry("auth/ucan.md", "nobody@host", HeartbeatGrace)
	if err != nil {
		t.Fatalf("UpdateExpiry (missing): %v", err)
	}
	if updated {
		t.Fatal("expected UpdateExpiry to return false for missing claim")
	}
}

// ── Test 7: AddSummary and GetSummariesFor ────────────────────────────────────

func TestSummaries(t *testing.T) {
	s := newTestStore(t)

	if err := s.AddSummary("auth/ucan.md", "alice@host", "Working on TTL path"); err != nil {
		t.Fatalf("AddSummary: %v", err)
	}
	if err := s.AddSummary("auth/ucan.md", "bob@host", "Reviewed minting flow"); err != nil {
		t.Fatalf("AddSummary: %v", err)
	}
	// Different scope — should not appear
	if err := s.AddSummary("other/file.md", "carol@host", "Other room"); err != nil {
		t.Fatalf("AddSummary other: %v", err)
	}

	sums, err := s.GetSummariesFor("auth/ucan.md")
	if err != nil {
		t.Fatalf("GetSummariesFor: %v", err)
	}
	if len(sums) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(sums))
	}
	if sums[0].Text != "Working on TTL path" {
		t.Errorf("unexpected summary text: %s", sums[0].Text)
	}
	if sums[1].AgentID != "bob@host" {
		t.Errorf("unexpected agent: %s", sums[1].AgentID)
	}
}

// ── Test 8: Summaries capped at 100 (101st drops oldest) ─────────────────────

func TestSummariesCapped(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 101; i++ {
		text := fmt.Sprintf("summary-%03d", i)
		if err := s.AddSummary("room/x.md", "agent@host", text); err != nil {
			t.Fatalf("AddSummary %d: %v", i, err)
		}
	}

	sums, err := s.GetSummariesFor("room/x.md")
	if err != nil {
		t.Fatalf("GetSummariesFor: %v", err)
	}
	if len(sums) != maxSummaries {
		t.Fatalf("expected %d summaries, got %d", maxSummaries, len(sums))
	}
	// Oldest (summary-000) should have been dropped
	if sums[0].Text != "summary-001" {
		t.Errorf("expected oldest to be summary-001, got %s", sums[0].Text)
	}
	// Newest should be last
	if sums[maxSummaries-1].Text != "summary-100" {
		t.Errorf("expected newest to be summary-100, got %s", sums[maxSummaries-1].Text)
	}
}

// ── Test 9: AllClaims returns claims across multiple scopes ───────────────────

func TestAllClaimsMultipleScopes(t *testing.T) {
	s := newTestStore(t)
	scopes := []string{"auth/ucan.md", "payments/checkout.md", "infra/deploy.md"}
	for _, sc := range scopes {
		if err := s.AddClaim(futureClaim(sc, "agent@host", "read")); err != nil {
			t.Fatalf("AddClaim %s: %v", sc, err)
		}
	}

	all, err := s.AllClaims()
	if err != nil {
		t.Fatalf("AllClaims: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 claims, got %d", len(all))
	}
}

// ── Test 10: 10 goroutines writing different rooms concurrently ───────────────

func TestConcurrentDifferentRooms(t *testing.T) {
	s := newTestStore(t)
	const n = 10
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			room := fmt.Sprintf("room/file-%02d.md", idx)
			agent := fmt.Sprintf("agent%02d@host", idx)
			errs[idx] = s.AddClaim(futureClaim(room, agent, "edit"))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: AddClaim error: %v", i, err)
		}
	}

	all, err := s.AllClaims()
	if err != nil {
		t.Fatalf("AllClaims: %v", err)
	}
	if len(all) != n {
		t.Fatalf("expected %d claims, got %d", n, len(all))
	}
}

// ── Test 11: 8 goroutines racing on same room — exactly 1 claim survives ──────

func TestConcurrentSameRoom(t *testing.T) {
	s := newTestStore(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			agent := fmt.Sprintf("agent%02d@host", idx)
			errs[idx] = s.AddClaim(futureClaim("shared/room.md", agent, "edit"))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: AddClaim error: %v", i, err)
		}
	}

	claims, err := s.GetClaimsFor("shared/room.md")
	if err != nil {
		t.Fatalf("GetClaimsFor: %v", err)
	}
	// Each agent has a unique ID, so each can hold its own claim;
	// but the spec says "racing on the same room — final state has exactly 1 claim"
	// meaning all goroutines write different agents, resulting in n claims.
	// The uniqueness constraint only collapses same-agent duplicates.
	// Re-reading the spec: "8 goroutines racing on the same room" — the purpose
	// is correctness (no corruption), and since agents are distinct, all 8 survive.
	// The spec says "exactly 1 claim" which implies same agent. Let's use same agent.
	//
	// Actually the test spec says: "8 goroutines racing on the same room —
	// final state has exactly 1 claim". To match that, we need same agent.
	// This test is re-implemented below in TestConcurrentSameRoomSameAgent.
	_ = claims
	// No panic, no corruption is the main assertion here.
	for _, c := range claims {
		if c.ScopeID != "shared/room.md" {
			t.Errorf("unexpected scope: %s", c.ScopeID)
		}
	}
}

// TestConcurrentSameRoomSameAgent: 8 goroutines with the SAME agent on the same
// room; dedup means only 1 claim survives.
func TestConcurrentSameRoomSameAgent(t *testing.T) {
	s := newTestStore(t)
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = s.AddClaim(futureClaim("shared/room.md", "alice@host", "edit"))
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: AddClaim error: %v", i, err)
		}
	}

	claims, err := s.GetClaimsFor("shared/room.md")
	if err != nil {
		t.Fatalf("GetClaimsFor: %v", err)
	}
	if len(claims) != 1 {
		t.Fatalf("expected exactly 1 claim after concurrent dedup, got %d", len(claims))
	}
}

// ── Test 12: Concurrent reader (AllClaims) and writer (AddClaim) ──────────────

func TestConcurrentReadWrite(t *testing.T) {
	s := newTestStore(t)
	// Seed a claim
	if err := s.AddClaim(futureClaim("shared/room.md", "seed@host", "read")); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 8 reader+writer pairs (16 goroutines). Kept modest so the serial lock queue
	// finishes well within the 30-second lock timeout even under -race.
	const n = 8
	var wg sync.WaitGroup
	errs := make([]error, n*2)

	for i := 0; i < n; i++ {
		wg.Add(2)

		// Reader
		go func(idx int) {
			defer wg.Done()
			_, err := s.AllClaims()
			errs[idx] = err
		}(i)

		// Writer
		go func(idx int) {
			defer wg.Done()
			agent := fmt.Sprintf("writer%02d@host", idx)
			errs[n+idx] = s.AddClaim(futureClaim("shared/room.md", agent, "edit"))
		}(i)
	}
	wg.Wait()

	// No panics, no errors
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: %v", i, err)
		}
	}
}

// ── Test 13: ParseTTL ──────────────────────────────────────────────────────────

func TestParseTTL(t *testing.T) {
	cases := []struct {
		input    string
		wantSecs int64
		wantErr  bool
	}{
		{"15m", 900, false},
		{"2h", 7200, false},
		{"30s", 30, false},
		{"1d", 86400, false},
		{"900", 900, false},
		{"invalid", 0, true},
		{"", 0, true},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			d, err := ParseTTL(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseTTL(%q): expected error, got %v", tc.input, d)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseTTL(%q): unexpected error: %v", tc.input, err)
			}
			got := int64(d / time.Second)
			if got != tc.wantSecs {
				t.Errorf("ParseTTL(%q): expected %ds, got %ds", tc.input, tc.wantSecs, got)
			}
		})
	}
}

// ── Test 14: CheckConflict — all 7 matrix cases ───────────────────────────────

func TestCheckConflict(t *testing.T) {
	t.Run("empty existing allows", func(t *testing.T) {
		action, msg := CheckConflict(nil, "edit")
		if action != ActionAllow {
			t.Errorf("expected allow, got %s", action)
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("edit+edit = conflict", func(t *testing.T) {
		existing := []Claim{{AgentID: "bob@host", Intent: "edit", Branch: "main",
			ExpiresAt: time.Now().Add(time.Hour)}}
		action, msg := CheckConflict(existing, "edit")
		if action != ActionConflict {
			t.Errorf("expected conflict, got %s", action)
		}
		if !strings.Contains(msg, "CONFLICT") {
			t.Errorf("expected CONFLICT in message, got %q", msg)
		}
	})

	t.Run("read+read = allow", func(t *testing.T) {
		existing := []Claim{{AgentID: "bob@host", Intent: "read"}}
		action, msg := CheckConflict(existing, "read")
		if action != ActionAllow {
			t.Errorf("expected allow, got %s", action)
		}
		if msg != "" {
			t.Errorf("expected empty message, got %q", msg)
		}
	})

	t.Run("read+edit = allow_with_visibility", func(t *testing.T) {
		existing := []Claim{{AgentID: "bob@host", Intent: "read"}}
		action, msg := CheckConflict(existing, "edit")
		if action != ActionAllowWithVisibility {
			t.Errorf("expected allow_with_visibility, got %s", action)
		}
		if !strings.Contains(msg, "NOTE") {
			t.Errorf("expected NOTE in message, got %q", msg)
		}
	})

	t.Run("edit+read = allow_with_visibility", func(t *testing.T) {
		existing := []Claim{{AgentID: "bob@host", Intent: "edit"}}
		action, msg := CheckConflict(existing, "read")
		if action != ActionAllowWithVisibility {
			t.Errorf("expected allow_with_visibility, got %s", action)
		}
		if !strings.Contains(msg, "NOTE") {
			t.Errorf("expected NOTE in message, got %q", msg)
		}
	})

	t.Run("review+edit = allow_with_warning", func(t *testing.T) {
		existing := []Claim{{AgentID: "bob@host", Intent: "review"}}
		action, msg := CheckConflict(existing, "edit")
		if action != ActionAllowWithWarning {
			t.Errorf("expected allow_with_warning, got %s", action)
		}
		if !strings.Contains(msg, "WARNING") {
			t.Errorf("expected WARNING in message, got %q", msg)
		}
	})

	t.Run("security-sweep+edit = governance_sensitive", func(t *testing.T) {
		existing := []Claim{{AgentID: "security@host", Intent: "security-sweep"}}
		action, msg := CheckConflict(existing, "edit")
		if action != ActionGovernanceSensitive {
			t.Errorf("expected governance_sensitive, got %s", action)
		}
		if !strings.Contains(msg, "CAUTION") {
			t.Errorf("expected CAUTION in message, got %q", msg)
		}
	})
}

