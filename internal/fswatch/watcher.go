package fswatch

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/micaelmalta/loi/internal/claims"
	"github.com/micaelmalta/loi/internal/git"
	"github.com/micaelmalta/loi/internal/index"
	"github.com/micaelmalta/loi/internal/notify"
	"github.com/micaelmalta/loi/internal/testrun"
)

// PolicyTier controls what auto mode is allowed to do.
type PolicyTier int

const (
	PolicyNotifyOnly    PolicyTier = iota // never invoke worker
	PolicyDraftOnly                        // branch+PR, no worker
	PolicyDocsSafe                         // only docs/ source files
	PolicyTestsSafe                        // only test files
	PolicyScopedCodeSafe                   // only files matching AllowedScopes
	PolicyFullAuto                         // no restriction
)

// WatcherConfig holds all configuration for StartWatcher.
type WatcherConfig struct {
	ProjectRoot        string
	WatchDir           string // docs/index dir
	Mode               string // "notify" | "auto" | "dry-run"
	Debounce           time.Duration
	WorkerCmd          string
	Backend            notify.NotifyBackend
	Policy             PolicyTier
	AllowedScopes      []string
	BlockGovernanceSec map[string]bool
	TestCmd            string
	WatchSource        bool
	SourcePaths        []string // extra source dirs; empty = project root
}

// StartWatcher starts the fsnotify watcher and blocks until ctx is cancelled.
// Handles both LOI index changes (Intent-to-Code) and source changes (Code-to-Intent).
func StartWatcher(ctx context.Context, cfg WatcherConfig) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("fswatch: create watcher: %w", err)
	}
	defer watcher.Close()

	// Resolve watchDir — add the docs/index tree.
	watchDir := cfg.WatchDir
	if watchDir == "" {
		watchDir = filepath.Join(cfg.ProjectRoot, "docs", "index")
	}

	if err := addDirRecursive(watcher, watchDir); err != nil {
		return fmt.Errorf("fswatch: watch index dir %s: %w", watchDir, err)
	}

	// Optionally watch source paths.
	if cfg.WatchSource {
		sourceDirs := cfg.SourcePaths
		if len(sourceDirs) == 0 {
			sourceDirs = []string{cfg.ProjectRoot}
		}
		for _, d := range sourceDirs {
			if err := addDirRecursive(watcher, d); err != nil {
				// Non-fatal: log and continue.
				fmt.Fprintf(os.Stderr, "fswatch: watch source dir %s: %v\n", d, err)
			}
		}
	}

	loiDebouncer := NewDebouncer(cfg.Debounce, func(files []string) {
		handleLOIBatch(cfg, files)
	})
	sourceDebouncer := NewDebouncer(cfg.Debounce, func(files []string) {
		handleSourceBatch(cfg, files)
	})

	for {
		select {
		case <-ctx.Done():
			return nil

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if !isRelevantEvent(event) {
				continue
			}
			path := event.Name

			// If a new directory was created, watch it recursively.
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				_ = addDirRecursive(watcher, path)
				continue
			}

			if isLOIIndexFile(path, watchDir) {
				loiDebouncer.Add(path)
			} else if cfg.WatchSource && isSourceFile(path, cfg.ProjectRoot) {
				sourceDebouncer.Add(path)
			}

		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			fmt.Fprintf(os.Stderr, "fswatch: watcher error: %v\n", watchErr)
		}
	}
}

// -----------------------------------------------------------------------------
// LOI handler (Intent-to-Code)
// -----------------------------------------------------------------------------

func handleLOIBatch(cfg WatcherConfig, files []string) {
	var allEntries []index.ChangedEntry
	var changedRooms []string

	for _, file := range files {
		if filepath.Ext(file) != ".md" {
			continue
		}
		diff, err := git.Diff(cfg.ProjectRoot, file)
		if err != nil || diff == "" {
			continue
		}
		entries := index.ExtractChangedEntries(diff)
		if len(entries) == 0 {
			continue
		}
		allEntries = append(allEntries, entries...)
		changedRooms = append(changedRooms, file)
	}

	if len(allEntries) == 0 {
		return
	}

	if cfg.Mode == "dry-run" {
		fmt.Println("[dry-run] LOI changes detected:")
		for _, e := range allEntries {
			fmt.Printf("  %s: %s\n", e.SourceFile, e.ChangedLine)
		}
		return
	}

	// Run validate (shell out to loi validate).
	if err := runValidate(cfg.ProjectRoot); err != nil {
		fmt.Fprintf(os.Stderr, "fswatch: validate failed: %v\n", err)
		return
	}

	repo := git.RepoName(cfg.ProjectRoot)
	ts := time.Now().UTC()
	branch := fmt.Sprintf("loi/auto-%s", ts.Format("20060102-150405"))

	// Compute governance info from changed rooms.
	govInfo := computeGovernanceInfo(changedRooms)

	// Build a brief summary.
	summary := buildSummary(allEntries)

	// Compute table diff across changed rooms.
	tableDiff := computeTableDiff(cfg.ProjectRoot, changedRooms)

	var prURL string

	switch cfg.Mode {
	case "notify":
		// Create a draft PR to surface the changes.
		if err := git.CheckoutNewBranch(cfg.ProjectRoot, branch); err == nil {
			for _, f := range changedRooms {
				_ = git.AddAndCommit(cfg.ProjectRoot, []string{f},
					fmt.Sprintf("loi: index update — %s", filepath.Base(f)))
			}
			if err := git.Push(cfg.ProjectRoot, branch); err == nil {
				prURL, _ = git.CreatePR(cfg.ProjectRoot, branch,
					fmt.Sprintf("LOI: intent update (%d entries)", len(allEntries)),
					summary, true)
			}
		}

		_ = cfg.Backend.Send(notify.NotifyEvent{
			Type:       "room.changed",
			Timestamp:  ts,
			Repo:       repo,
			Path:       strings.Join(changedRooms, ", "),
			Summary:    summary,
			PRURL:      prURL,
			TableDiff:  tableDiff,
			Governance: govInfo,
			Rooms:      changedRooms,
		})

	case "auto":
		allowed, reason := checkPolicy(cfg, changedRooms, govInfo)

		// PolicyDraftOnly: still create a PR but don't run worker.
		if !allowed && cfg.Policy == PolicyDraftOnly {
			if err := git.CheckoutNewBranch(cfg.ProjectRoot, branch); err == nil {
				for _, f := range changedRooms {
					_ = git.AddAndCommit(cfg.ProjectRoot, []string{f},
						fmt.Sprintf("loi: index update — %s", filepath.Base(f)))
				}
				if err := git.Push(cfg.ProjectRoot, branch); err == nil {
					prURL, _ = git.CreatePR(cfg.ProjectRoot, branch,
						fmt.Sprintf("LOI: intent update (%d entries)", len(allEntries)),
						summary, true)
				}
			}
			fmt.Fprintf(os.Stderr, "fswatch: policy blocked worker (%s); draft PR created\n", reason)
			_ = cfg.Backend.Send(notify.NotifyEvent{
				Type:       "room.changed",
				Timestamp:  ts,
				Repo:       repo,
				Path:       strings.Join(changedRooms, ", "),
				Summary:    summary,
				PRURL:      prURL,
				TableDiff:  tableDiff,
				Governance: govInfo,
				Rooms:      changedRooms,
			})
			return
		}

		if !allowed {
			fmt.Fprintf(os.Stderr, "fswatch: policy blocked auto action: %s\n", reason)
			return
		}

		// Run worker subprocess.
		if cfg.WorkerCmd != "" {
			runWorker(cfg.ProjectRoot, cfg.WorkerCmd, changedRooms)
		}

		// Run tests.
		passed, testOut := testrun.DetectAndRun(cfg.ProjectRoot, cfg.TestCmd)

		if !passed {
			handleTestFailure(cfg, changedRooms, testOut)
			return
		}

		// Commit, push, create PR.
		if err := git.CheckoutNewBranch(cfg.ProjectRoot, branch); err == nil {
			for _, f := range changedRooms {
				_ = git.AddAndCommit(cfg.ProjectRoot, []string{f},
					fmt.Sprintf("loi: index update — %s", filepath.Base(f)))
			}
			if err := git.Push(cfg.ProjectRoot, branch); err == nil {
				prURL, _ = git.CreatePR(cfg.ProjectRoot, branch,
					fmt.Sprintf("LOI: intent update (%d entries)", len(allEntries)),
					summary, false)
			}
		}

		_ = cfg.Backend.Send(notify.NotifyEvent{
			Type:       "room.changed",
			Timestamp:  ts,
			Repo:       repo,
			Path:       strings.Join(changedRooms, ", "),
			Summary:    summary,
			PRURL:      prURL,
			TableDiff:  tableDiff,
			Governance: govInfo,
			Rooms:      changedRooms,
			TestOutput: testOut,
		})
	}
}

// -----------------------------------------------------------------------------
// Source handler (Code-to-Intent)
// -----------------------------------------------------------------------------

func handleSourceBatch(cfg WatcherConfig, files []string) {
	docsPrefix := filepath.Join(cfg.ProjectRoot, "docs") + string(filepath.Separator)

	var processed []string
	for _, file := range files {
		// Skip files inside the docs/ subtree.
		if strings.HasPrefix(file, docsPrefix) || strings.HasPrefix(file, filepath.Join(cfg.ProjectRoot, "docs")) {
			continue
		}
		processed = append(processed, file)
	}
	if len(processed) == 0 {
		return
	}

	repo := git.RepoName(cfg.ProjectRoot)
	ts := time.Now().UTC()
	branch := fmt.Sprintf("loi/source-%s", ts.Format("20060102-150405"))

	var allRooms []string
	roomSet := make(map[string]bool)

	for _, file := range processed {
		rel, err := filepath.Rel(cfg.ProjectRoot, file)
		if err != nil {
			rel = file
		}
		rooms, err := index.FindCoveringRooms(cfg.ProjectRoot, rel, index.CoverByContent)
		if err != nil || len(rooms) == 0 {
			continue
		}

		// Check if codetect symbols.db exists.
		symbolsDB := filepath.Join(cfg.ProjectRoot, "symbols.db")
		hasSymbols := false
		if _, err := os.Stat(symbolsDB); err == nil {
			hasSymbols = true
		}

		for _, roomPath := range rooms {
			if roomSet[roomPath] {
				continue
			}
			roomSet[roomPath] = true
			allRooms = append(allRooms, roomPath)

			if hasSymbols {
				// Run loi generate --scaffold --room <room>
				roomRel, relErr := filepath.Rel(cfg.ProjectRoot, roomPath)
				if relErr != nil {
					roomRel = roomPath
				}
				cmd := exec.Command("loi", "generate", "--scaffold", "--room", roomRel)
				cmd.Dir = cfg.ProjectRoot
				if out, err := cmd.CombinedOutput(); err != nil {
					fmt.Fprintf(os.Stderr, "fswatch: loi generate: %v\n%s\n", err, out)
				}
			} else {
				// Mark room as stale.
				_, _ = index.UpdateFrontmatterField(roomPath, "stale_since",
					ts.Format(time.RFC3339))
			}
		}
	}

	if len(allRooms) == 0 {
		return
	}

	// Create draft PR.
	var prURL string
	if err := git.CheckoutNewBranch(cfg.ProjectRoot, branch); err == nil {
		if err := git.AddAndCommit(cfg.ProjectRoot, allRooms,
			fmt.Sprintf("loi: mark rooms stale after source change (%d rooms)", len(allRooms))); err == nil {
			if err := git.Push(cfg.ProjectRoot, branch); err == nil {
				prURL, _ = git.CreatePR(cfg.ProjectRoot, branch,
					fmt.Sprintf("LOI: source change detected (%d rooms)", len(allRooms)),
					fmt.Sprintf("Source files changed:\n%s", strings.Join(processed, "\n")),
					true)
			}
		}
	}

	_ = cfg.Backend.Send(notify.NotifyEvent{
		Type:      "source.changed",
		Timestamp: ts,
		Repo:      repo,
		Path:      strings.Join(processed, ", "),
		Summary:   fmt.Sprintf("%d source file(s) changed, %d room(s) affected", len(processed), len(allRooms)),
		PRURL:     prURL,
		Rooms:     allRooms,
	})
}

// -----------------------------------------------------------------------------
// checkPolicy
// -----------------------------------------------------------------------------

// checkPolicy returns (allowed, reason). It translates _check_policy from watcher.py.
func checkPolicy(cfg WatcherConfig, rooms []string, govInfo map[string]string) (bool, string) {
	switch cfg.Policy {
	case PolicyNotifyOnly:
		return false, "policy is notify-only"

	case PolicyDraftOnly:
		return false, "policy is draft-only"

	case PolicyDocsSafe:
		// All source files must be under docs/.
		docsPrefix := filepath.Join(cfg.ProjectRoot, "docs")
		for _, r := range rooms {
			if !strings.HasPrefix(r, docsPrefix) {
				return false, fmt.Sprintf("policy docs-safe: %s is not under docs/", r)
			}
		}

	case PolicyTestsSafe:
		// All source files must be test files.
		for _, r := range rooms {
			base := filepath.Base(r)
			if !isTestFile(base) {
				return false, fmt.Sprintf("policy tests-safe: %s is not a test file", r)
			}
		}

	case PolicyScopedCodeSafe:
		// All source files must match AllowedScopes globs.
		for _, r := range rooms {
			if !matchesAnyScope(r, cfg.AllowedScopes) {
				return false, fmt.Sprintf("policy scoped-code-safe: %s does not match allowed scopes", r)
			}
		}

	case PolicyFullAuto:
		// No restriction.
	}

	// Always block on health=critical.
	if health := govInfo["health"]; health == "critical" {
		return false, "governance health is critical"
	}

	// Governance security block.
	if security := govInfo["security"]; security != "" && cfg.BlockGovernanceSec[security] {
		return false, fmt.Sprintf("governance security tier %q is blocked", security)
	}

	// Advisory room claim check.
	for _, roomPath := range rooms {
		roomName := filepath.Base(strings.TrimSuffix(roomPath, ".md"))
		cs := claims.NewClaimsStore(cfg.ProjectRoot)
		existing, err := cs.GetClaimsFor(roomName)
		if err == nil && len(existing) > 0 {
			action, msg := claims.CheckConflict(existing, "edit")
			if action == claims.ActionConflict || action == claims.ActionGovernanceSensitive {
				return false, msg
			}
			// Warnings / visibility: log but allow.
			if msg != "" {
				fmt.Fprintf(os.Stderr, "fswatch: claims advisory: %s\n", msg)
			}
		}
	}

	return true, ""
}

// -----------------------------------------------------------------------------
// handleTestFailure
// -----------------------------------------------------------------------------

// handleTestFailure marks affected rooms as conflicted, commits to a conflict
// branch, and sends a conflict.detected event.
func handleTestFailure(cfg WatcherConfig, rooms []string, testOutput string) {
	ts := time.Now().UTC()
	branch := fmt.Sprintf("loi/conflict-%s", ts.Format("20060102-150405"))
	repo := git.RepoName(cfg.ProjectRoot)

	var updated []string
	for _, roomPath := range rooms {
		changed, err := index.UpdateFrontmatterField(roomPath, "architectural_health", "conflicted")
		if err != nil {
			fmt.Fprintf(os.Stderr, "fswatch: update health %s: %v\n", roomPath, err)
			continue
		}
		if changed {
			updated = append(updated, roomPath)
		}
	}

	if len(updated) > 0 {
		if err := git.CheckoutNewBranch(cfg.ProjectRoot, branch); err == nil {
			msg := fmt.Sprintf("loi: mark %d room(s) conflicted after test failure", len(updated))
			if err := git.AddAndCommit(cfg.ProjectRoot, updated, msg); err == nil {
				_ = git.Push(cfg.ProjectRoot, branch)
			}
		}
	}

	_ = cfg.Backend.Send(notify.NotifyEvent{
		Type:       "conflict.detected",
		Timestamp:  ts,
		Repo:       repo,
		Rooms:      rooms,
		TestOutput: testOutput,
		Summary:    fmt.Sprintf("Test failure detected in %d room(s)", len(rooms)),
	})
}

// -----------------------------------------------------------------------------
// Debouncer
// -----------------------------------------------------------------------------

// Debouncer batches file events over a delay window. Each Add() resets the
// timer; the callback runs once after delay with no more events.
type Debouncer struct {
	mu      sync.Mutex
	timer   *time.Timer
	pending map[string]struct{}
	delay   time.Duration
	fn      func([]string)
}

// NewDebouncer creates a Debouncer with the given delay and callback.
func NewDebouncer(delay time.Duration, fn func(files []string)) *Debouncer {
	return &Debouncer{
		pending: make(map[string]struct{}),
		delay:   delay,
		fn:      fn,
	}
}

// Add adds path to the pending set and resets the debounce timer.
func (d *Debouncer) Add(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pending[path] = struct{}{}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.fire)
}

// fire drains the pending set and calls fn outside the lock.
func (d *Debouncer) fire() {
	d.mu.Lock()
	files := make([]string, 0, len(d.pending))
	for f := range d.pending {
		files = append(files, f)
	}
	clear(d.pending)
	d.timer = nil
	d.mu.Unlock()
	d.fn(files)
}

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// addDirRecursive walks dir and adds every sub-directory to watcher.
func addDirRecursive(w *fsnotify.Watcher, dir string) error {
	return filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable
		}
		if d.IsDir() {
			return w.Add(path)
		}
		return nil
	})
}

// isRelevantEvent returns true for write/create/rename events.
func isRelevantEvent(e fsnotify.Event) bool {
	return e.Has(fsnotify.Write) || e.Has(fsnotify.Create) || e.Has(fsnotify.Rename)
}

// isLOIIndexFile returns true if path is a .md file inside watchDir.
func isLOIIndexFile(path, watchDir string) bool {
	if filepath.Ext(path) != ".md" {
		return false
	}
	return strings.HasPrefix(path, watchDir)
}

// isSourceFile returns true for non-docs source files.
func isSourceFile(path, projectRoot string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if !index.SourceExts[ext] {
		return false
	}
	docsDir := filepath.Join(projectRoot, "docs")
	return !strings.HasPrefix(path, docsDir)
}

// isTestFile returns true for common test file naming patterns.
func isTestFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, "test_") ||
		strings.HasSuffix(lower, "_test.go") ||
		strings.HasSuffix(lower, ".test.ts") ||
		strings.HasSuffix(lower, ".test.js") ||
		strings.HasSuffix(lower, "_spec.rb") ||
		strings.Contains(lower, "test")
}

// matchesAnyScope returns true if path matches any of the scope glob patterns.
func matchesAnyScope(path string, scopes []string) bool {
	for _, scope := range scopes {
		// filepath.Match handles simple globs; for path prefix matching fall back.
		if matched, err := filepath.Match(scope, path); err == nil && matched {
			return true
		}
		if strings.HasPrefix(path, scope) {
			return true
		}
	}
	return false
}

// computeGovernanceInfo aggregates governance metadata from changed rooms.
func computeGovernanceInfo(rooms []string) map[string]string {
	info := make(map[string]string)
	for _, roomPath := range rooms {
		fm, err := index.ParseFrontmatter(roomPath)
		if err != nil || fm == nil {
			continue
		}
		// Prefer most severe health.
		if h := fm.ArchitecturalHealth; h != "" {
			existing := info["health"]
			info["health"] = mostSevereHealth(existing, h)
		}
		if s := fm.SecurityTier; s != "" {
			info["security"] = s
		}
	}
	return info
}

// mostSevereHealth returns the more severe of two health strings.
// critical > conflicted > warning > degraded > normal > ""
func mostSevereHealth(a, b string) string {
	order := map[string]int{
		"critical":   5,
		"conflicted": 4,
		"warning":    3,
		"degraded":   2,
		"normal":     1,
		"":           0,
	}
	if order[b] > order[a] {
		return b
	}
	return a
}

// buildSummary creates a brief human-readable summary from changed entries.
func buildSummary(entries []index.ChangedEntry) string {
	if len(entries) == 0 {
		return "LOI intent update"
	}
	seen := make(map[string]bool)
	for _, e := range entries {
		seen[e.SourceFile] = true
	}
	files := make([]string, 0, len(seen))
	for f := range seen {
		files = append(files, f)
	}
	if len(files) == 1 {
		return fmt.Sprintf("Intent update for %s (%d field(s) changed)", files[0], len(entries))
	}
	return fmt.Sprintf("Intent update for %d files (%d field(s) changed)", len(files), len(entries))
}

// computeTableDiff loads the HEAD version and the working-tree version of each
// changed room, then diffs their tables.
func computeTableDiff(projectRoot string, rooms []string) string {
	var parts []string
	for _, roomPath := range rooms {
		rel, err := filepath.Rel(projectRoot, roomPath)
		if err != nil {
			rel = roomPath
		}
		oldContent, err := git.Show(projectRoot, "HEAD", rel)
		if err != nil {
			continue
		}
		newData, err := os.ReadFile(roomPath)
		if err != nil {
			continue
		}
		oldTables := index.ParseTables(oldContent)
		newTables := index.ParseTables(string(newData))
		diff := index.DiffTables(oldTables, newTables)
		formatted := index.FormatDiff(diff)
		if formatted != "" {
			parts = append(parts, fmt.Sprintf("--- %s ---\n%s", filepath.Base(roomPath), formatted))
		}
	}
	return strings.Join(parts, "\n\n")
}

// runValidate shells out to `loi validate` in projectRoot.
func runValidate(projectRoot string) error {
	cmd := exec.Command("loi", "validate")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("loi validate failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// runWorker runs the worker command subprocess with the changed rooms as arguments.
func runWorker(projectRoot, workerCmd string, rooms []string) {
	args := strings.Fields(workerCmd)
	args = append(args, rooms...)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = projectRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "fswatch: worker %q: %v\n%s\n", workerCmd, err, out)
	}
}
