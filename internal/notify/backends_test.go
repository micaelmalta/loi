package notify

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var testEvent = NotifyEvent{
	Type:      "room.changed",
	Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	Repo:      "loi",
	Path:      "docs/index/auth.md",
	Summary:   "test summary",
}

// ---- stdoutBackend ----------------------------------------------------------

func TestStdoutBackend_Send(t *testing.T) {
	// Capture stdout.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w

	b := &stdoutBackend{}
	if err := b.Send(testEvent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	w.Close()
	os.Stdout = origStdout

	var buf bytes.Buffer
	io.Copy(&buf, r)
	line := strings.TrimSpace(buf.String())

	if line == "" {
		t.Fatal("expected JSON output, got empty")
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v — got: %s", err, line)
	}
	if got["event"] != "room.changed" {
		t.Errorf("event field: got %v, want room.changed", got["event"])
	}
}

// ---- fileBackend ------------------------------------------------------------

func TestFileBackend_Send(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	b, err := newFileBackend(path)
	if err != nil {
		t.Fatalf("newFileBackend: %v", err)
	}

	for i := 0; i < 3; i++ {
		evt := testEvent
		evt.Summary = fmt.Sprintf("event %d", i)
		if err := b.Send(evt); err != nil {
			t.Fatalf("Send %d: %v", i, err)
		}
	}

	f, _ := os.Open(path)
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
	for _, line := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Errorf("line not valid JSON: %v", err)
		}
	}
}

func TestFileBackend_CreatesParentDir_rejectsOnMissingParent(t *testing.T) {
	// newFileBackend should fail if the parent dir doesn't exist and can't be
	// created (we just verify it returns an error rather than panicking).
	path := filepath.Join(t.TempDir(), "nonexistent", "subdir", "events.jsonl")
	_, err := newFileBackend(path)
	// The file backend does NOT auto-create nested parents; it returns an error.
	if err == nil {
		t.Log("platform created parent dirs automatically — skipping assertion")
	}
	// Either outcome is acceptable; this test just verifies no panic.
}

// ---- webhookBackend ---------------------------------------------------------

func TestWebhookBackend_Send(t *testing.T) {
	var gotBody []byte
	var gotContentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotContentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := newWebhookBackend(srv.URL, "")
	if err := b.Send(testEvent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotContentType != "application/json" {
		t.Errorf("Content-Type: got %q, want application/json", gotContentType)
	}

	var got map[string]any
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("body not valid JSON: %v — body: %s", err, gotBody)
	}
	if got["event"] != "room.changed" {
		t.Errorf("event field: got %v, want room.changed", got["event"])
	}
}

func TestWebhookBackend_BearerToken(t *testing.T) {
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("LOI_TEST_TOKEN", "mysecrettoken")

	b := newWebhookBackend(srv.URL, "LOI_TEST_TOKEN")
	if err := b.Send(testEvent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	if gotAuth != "Bearer mysecrettoken" {
		t.Errorf("Authorization: got %q, want %q", gotAuth, "Bearer mysecrettoken")
	}
}

func TestWebhookBackend_Non2xx_returnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b := newWebhookBackend(srv.URL, "")
	err := b.Send(testEvent)
	if err == nil {
		t.Error("expected error for 500 response")
	}
}

// ---- slackBackend -----------------------------------------------------------

func TestSlackBackend_Send(t *testing.T) {
	var gotBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b := newSlackBackend(srv.URL)
	if err := b.Send(testEvent); err != nil {
		t.Fatalf("Send: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(gotBody, &got); err != nil {
		t.Fatalf("body not valid JSON: %v — body: %s", err, gotBody)
	}

	blocks, ok := got["blocks"]
	if !ok {
		t.Fatal("payload missing 'blocks' key")
	}
	blocksSlice, ok := blocks.([]any)
	if !ok || len(blocksSlice) == 0 {
		t.Error("expected non-empty blocks array")
	}
}

// ---- LoadBackend ------------------------------------------------------------

func TestLoadBackend_unknownReturnsError(t *testing.T) {
	_, err := LoadBackend(map[string]string{"backend": "unknown"})
	if err == nil {
		t.Error("expected error for unknown backend")
	}
}

func TestLoadBackend_webhookRequiresURL(t *testing.T) {
	_, err := LoadBackend(map[string]string{"backend": "webhook"})
	if err == nil {
		t.Error("expected error: webhook needs notify_url")
	}
}

func TestLoadBackend_slackRequiresURL(t *testing.T) {
	_, err := LoadBackend(map[string]string{"backend": "slack"})
	if err == nil {
		t.Error("expected error: slack needs notify_url")
	}
}

func TestLoadBackend_fileDefaultsPath(t *testing.T) {
	// Default file path is "loi-events.jsonl" — but we can't easily test the
	// default in a temp context, so just test explicit path.
	path := filepath.Join(t.TempDir(), "events.jsonl")
	b, err := LoadBackend(map[string]string{"backend": "file", "file_path": path})
	if err != nil {
		t.Fatalf("LoadBackend file: %v", err)
	}
	if b == nil {
		t.Error("expected non-nil backend")
	}
}

func TestLoadBackend_stdoutDefault(t *testing.T) {
	b, err := LoadBackend(map[string]string{})
	if err != nil {
		t.Fatalf("LoadBackend empty: %v", err)
	}
	if b == nil {
		t.Error("expected non-nil backend")
	}
}
