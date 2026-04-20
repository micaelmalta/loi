package datadog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"encoding/json"
)

// newFastClient returns a Client pointing at srv with a very short HTTP timeout.
func newFastClient(srv *httptest.Server) *Client {
	c := NewClient("k", "a").withBaseURL(srv.URL)
	c.httpClient = &http.Client{Timeout: 2 * time.Second}
	return c
}

func TestPoll_firesCallbackOnThresholdBreach(t *testing.T) {
	srv := serveFixture(t, http.StatusOK, fixtureResponse("cpu", "service:auth", 90.0))
	defer srv.Close()

	var fired int32
	cfg := PollConfig{
		Query:     "avg:cpu{*}",
		Interval:  20 * time.Millisecond,
		Window:    time.Second,
		Threshold: 80.0,
		Operator:  ">",
		OnAlert: func(s Series, rooms []string) {
			atomic.AddInt32(&fired, 1)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	Poll(ctx, cfg, newFastClient(srv)) //nolint:errcheck

	if atomic.LoadInt32(&fired) == 0 {
		t.Error("expected callback to fire at least once")
	}
}

func TestPoll_noCallbackBelowThreshold(t *testing.T) {
	srv := serveFixture(t, http.StatusOK, fixtureResponse("cpu", "service:auth", 30.0))
	defer srv.Close()

	var fired int32
	cfg := PollConfig{
		Query:     "avg:cpu{*}",
		Interval:  20 * time.Millisecond,
		Window:    time.Second,
		Threshold: 80.0,
		Operator:  ">",
		OnAlert: func(s Series, rooms []string) {
			atomic.AddInt32(&fired, 1)
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	Poll(ctx, cfg, newFastClient(srv)) //nolint:errcheck

	if atomic.LoadInt32(&fired) != 0 {
		t.Errorf("expected no callback below threshold, got %d", fired)
	}
}

func TestPoll_stopOnContextCancel(t *testing.T) {
	srv := serveFixture(t, http.StatusOK, ddQueryResponse{})
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cfg := PollConfig{
		Interval: 10 * time.Millisecond,
		Window:   time.Second,
		OnAlert:  func(Series, []string) {},
	}

	done := make(chan error, 1)
	go func() {
		done <- Poll(ctx, cfg, newFastClient(srv))
	}()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error on cancel, got %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Poll did not return after context cancel")
	}
}

func TestPoll_stopOnAuthFailure(t *testing.T) {
	srv := serveFixture(t, http.StatusForbidden, nil)
	defer srv.Close()

	cfg := PollConfig{
		Interval: 10 * time.Millisecond,
		Window:   time.Second,
		OnAlert:  func(Series, []string) {},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := Poll(ctx, cfg, newFastClient(srv))
	if err == nil {
		t.Error("expected error on auth failure")
	}
}

func TestPoll_scopeToRoomMapping(t *testing.T) {
	// Build a minimal LOI index in a temp dir so FindCoveringRooms can work.
	root := t.TempDir()
	indexDir := filepath.Join(root, "docs", "index", "auth")
	if err := os.MkdirAll(indexDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	roomContent := "---\nroom: auth/login.md\narchitectural_health: normal\nsecurity_tier: normal\nsee_also: []\n---\n\n# LOI Room: auth/login\n\nSource paths: internal/auth\n\n## Entries\n\n# login.go\n\nDOES: Handles auth.\nSYMBOLS:\n- Login() error\n"
	if err := os.WriteFile(filepath.Join(indexDir, "login.md"), []byte(roomContent), 0o644); err != nil {
		t.Fatalf("write room: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(fixtureResponse("cpu", "service:auth", 95.0))
	}))
	defer srv.Close()

	var capturedRooms []string
	cfg := PollConfig{
		Query:       "avg:cpu{*}",
		Interval:    20 * time.Millisecond,
		Window:      time.Second,
		Threshold:   80.0,
		Operator:    ">",
		ProjectRoot: root,
		OnAlert: func(s Series, rooms []string) {
			capturedRooms = rooms
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	Poll(ctx, cfg, newFastClient(srv)) //nolint:errcheck

	// The scope "service:auth" → pathHint "auth" → should match the auth room.
	if len(capturedRooms) == 0 {
		t.Error("expected at least one covering room for scope service:auth")
	}
}

func TestBreaches(t *testing.T) {
	tests := []struct {
		value, threshold float64
		op               string
		want             bool
	}{
		{90, 80, ">", true},
		{80, 80, ">", false},
		{80, 80, ">=", true},
		{79, 80, ">=", false},
		{50, 80, "<", true},
		{80, 80, "<", false},
		{80, 80, "<=", true},
		{81, 80, "<=", false},
	}
	for _, tt := range tests {
		got := breaches(tt.value, tt.threshold, tt.op)
		if got != tt.want {
			t.Errorf("breaches(%v, %v, %q) = %v, want %v", tt.value, tt.threshold, tt.op, got, tt.want)
		}
	}
}
