package notify

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// fileBackend appends JSON lines to a file. Thread-safe.
type fileBackend struct {
	mu   sync.Mutex
	path string
}

// newFileBackend returns a fileBackend that writes to path.
// It creates the file (and any missing parent directories) if they do not exist.
func newFileBackend(path string) (*fileBackend, error) {
	// Open/create once to validate the path is writable.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("notify/file: open %q: %w", path, err)
	}
	f.Close()
	return &fileBackend{path: path}, nil
}

// Send marshals e as JSON and appends it to the log file as a single line.
func (b *fileBackend) Send(e NotifyEvent) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("notify/file: marshal: %w", err)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	f, err := os.OpenFile(b.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("notify/file: open %q: %w", b.path, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, string(data)); err != nil {
		return fmt.Errorf("notify/file: write %q: %w", b.path, err)
	}
	return nil
}
