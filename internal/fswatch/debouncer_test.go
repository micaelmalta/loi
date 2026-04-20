package fswatch

import (
	"sync"
	"testing"
	"time"
)

func TestDebouncer_firesAfterDelay(t *testing.T) {
	delay := 30 * time.Millisecond
	var mu sync.Mutex
	var got []string
	var calls int

	d := NewDebouncer(delay, func(files []string) {
		mu.Lock()
		got = files
		calls++
		mu.Unlock()
	})

	d.Add("a.md")
	d.Add("b.md")
	d.Add("c.md")

	time.Sleep(delay * 3)

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("callback calls: got %d, want 1", calls)
	}
	if len(got) != 3 {
		t.Errorf("files len: got %d, want 3 — files: %v", len(got), got)
	}
}

func TestDebouncer_resetOnNewAdd(t *testing.T) {
	delay := 40 * time.Millisecond
	var mu sync.Mutex
	var calls int

	d := NewDebouncer(delay, func(files []string) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	d.Add("a.md")
	time.Sleep(delay / 2) // before window expires
	d.Add("b.md")
	time.Sleep(delay / 2) // still within new window
	d.Add("c.md")

	time.Sleep(delay * 3) // let final window expire

	mu.Lock()
	defer mu.Unlock()
	if calls != 1 {
		t.Errorf("callback calls: got %d, want 1 (timer should reset)", calls)
	}
}

func TestDebouncer_emptyBatchNotFired(t *testing.T) {
	delay := 20 * time.Millisecond
	var mu sync.Mutex
	var calls int

	_ = NewDebouncer(delay, func(files []string) {
		mu.Lock()
		calls++
		mu.Unlock()
	})

	time.Sleep(delay * 3)

	mu.Lock()
	defer mu.Unlock()
	if calls != 0 {
		t.Errorf("callback calls: got %d, want 0 (no adds)", calls)
	}
}
