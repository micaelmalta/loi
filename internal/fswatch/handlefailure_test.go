package fswatch

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/micaelmalta/loi/internal/notify"
)

// fakeBackend captures all sent events for inspection.
type fakeBackend struct {
	mu     sync.Mutex
	events []notify.NotifyEvent
}

func (f *fakeBackend) Send(e notify.NotifyEvent) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.events = append(f.events, e)
	return nil
}

func (f *fakeBackend) last() (notify.NotifyEvent, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.events) == 0 {
		return notify.NotifyEvent{}, false
	}
	return f.events[len(f.events)-1], true
}

// initTempRepo creates a temporary git repository and returns its path.
func initTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}
	run("git", "init", "-b", "main")
	run("git", "config", "user.email", "test@test.com")
	run("git", "config", "user.name", "Test")
	// Create an initial commit so HEAD exists.
	placeholder := filepath.Join(dir, ".gitkeep")
	os.WriteFile(placeholder, []byte{}, 0o644)
	run("git", "add", ".gitkeep")
	run("git", "commit", "-m", "init")
	return dir
}

// writeRoomInRepo writes a room fixture file in repo and returns the path.
func writeRoomInRepo(t *testing.T, repoRoot, name, health, security string) string {
	t.Helper()
	content := "---\nroom: " + name + "\narchitectural_health: " + health + "\nsecurity_tier: " + security + "\nsee_also: []\n---\n\n# dummy.go\n\nDOES: test.\n"
	path := filepath.Join(repoRoot, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write room: %v", err)
	}
	return path
}

func TestHandleTestFailure_marksRoomsConflicted(t *testing.T) {
	root := initTempRepo(t)
	backend := &fakeBackend{}

	room := writeRoomInRepo(t, root, "auth.md", "normal", "normal")

	cfg := WatcherConfig{
		ProjectRoot: root,
		Backend:     backend,
	}
	handleTestFailure(cfg, []string{room}, "test output here")

	// Room file should now have architectural_health: conflicted.
	data, err := os.ReadFile(room)
	if err != nil {
		t.Fatalf("read room: %v", err)
	}
	if !strings.Contains(string(data), "architectural_health: conflicted") {
		t.Errorf("expected conflicted health in room file; got:\n%s", data)
	}
}

func TestHandleTestFailure_sendsConflictEvent(t *testing.T) {
	root := initTempRepo(t)
	backend := &fakeBackend{}
	room := writeRoomInRepo(t, root, "auth.md", "normal", "normal")

	cfg := WatcherConfig{
		ProjectRoot: root,
		Backend:     backend,
	}
	handleTestFailure(cfg, []string{room}, "some failures")

	evt, ok := backend.last()
	if !ok {
		t.Fatal("expected at least one event sent")
	}
	if evt.Type != "conflict.detected" {
		t.Errorf("event type: got %q, want %q", evt.Type, "conflict.detected")
	}
	if len(evt.Rooms) == 0 {
		t.Error("expected rooms in event")
	}
}

func TestHandleTestFailure_noRooms_noOp(t *testing.T) {
	root := initTempRepo(t)
	backend := &fakeBackend{}

	cfg := WatcherConfig{
		ProjectRoot: root,
		Backend:     backend,
	}
	handleTestFailure(cfg, nil, "")

	// A conflict.detected event is still sent (for observability), but no rooms committed.
	evt, ok := backend.last()
	if !ok {
		t.Fatal("expected conflict.detected event even with no rooms")
	}
	if evt.Type != "conflict.detected" {
		t.Errorf("event type: got %q, want %q", evt.Type, "conflict.detected")
	}
}
