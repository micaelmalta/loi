---
room: runtime
see_also:
  - core/index.md
  - core/claims.md
  - core/notify.md
architectural_health: normal
security_tier: normal
hot_paths: watcher.go, symbols.go
---

# LOI Room: runtime

Source paths: internal/fswatch/, internal/git/, internal/testrun/, internal/codetect/

## Entries

# detect.go

DOES: Auto-detects and runs the project test suite with a 300-second timeout. Detection probes in order: `pytest` → `python3 -m pytest` → `go test ./...` (requires go.mod) → `npm test` (requires package.json). Returns `(passed bool, output string)`.
SYMBOLS:
- DetectAndRun(projectRoot, testCmd string) (bool, string)
PATTERNS: multi-runtime-test-detection
USE WHEN: Running tests after an auto-mode LOI worker execution inside the file watcher.

---

# git.go

DOES: Thin wrapper around `git` and `gh` CLI commands; all public functions delegate to an internal `run(dir, args...)` helper that executes commands in a given directory and returns trimmed stdout.
SYMBOLS:
- Root() (string, error)
- RepoName(projectRoot string) string
- Branch(projectRoot string) string
- Diff(projectRoot, path string) (string, error)
- Show(projectRoot, ref, filePath string) (string, error)
- DiffNameOnly(projectRoot string, extraArgs ...string) ([]string, error)
- StagedFiles(projectRoot string) ([]string, error)
- CreatePR(projectRoot, branch, title, body string, draft bool) (string, error)
- CheckoutNewBranch(projectRoot, branch string) error
- AddAndCommit(projectRoot string, files []string, message string) error
- Push(projectRoot, branch string) error
- CurrentBranch(projectRoot string) (string, error)
PATTERNS: git-shell-delegation
USE WHEN: Any package needing git operations (diff, branch, commit, push) or GitHub PR creation via `gh`.

---

# symbols.go

DOES: SQLite-backed codetect symbol indexing and Go source analysis. Opens `symbols.db` in immutable mode, queries and deduplicates symbols by file, parses Go imports, classifies same-repo vs external deps, and generates LOI room markdown with `<!-- LLM-FILL -->` scaffolding markers.
SYMBOLS:
- OpenDB(dbPath string) (*sql.DB, error)
- QuerySymbols(db *sql.DB) (map[string][]Symbol, error)
- GetModuleName(projectRoot string) string
- ParseGoImports(filePath string) ([]string, error)
- ClassifyImports(imports []string, moduleName string) (sameRepo, external []string)
- ReadFuncSignature(projectRoot, filePath string, line int) string
- BuildSymbolsLines(symbols []Symbol, projectRoot string) []string
- GenerateFileEntry(filePath string, symbols []Symbol, projectRoot, moduleName string) (string, []string)
- GenerateRoom(roomName string, fileEntries []string, seeAlso []string) string
- BuildSeeAlso(roomName string, allRoomDeps []string, depPathToRoom map[string]string) []string
- GroupFilesByDirectory(filePaths []string) map[string][]string
- Types: Symbol
TYPE: Symbol { Name, Kind, Path, Line, Pattern, Scope, Signature string }
DEPENDS: modernc.org/sqlite
PATTERNS: sqlite-immutable-read, loi-entry-generation
USE WHEN: Generating or scaffolding LOI room entries from a codetect `symbols.db` index.

---

# watcher.go

DOES: Blocks until context is cancelled, watching `docs/index/` for LOI markdown changes (intent-to-code flow) and optionally source directories for code changes (code-to-intent flow). Debounces events, gates actions through a `PolicyTier`, then creates branches/commits/PRs and sends notifications via the configured backend.
SYMBOLS:
- StartWatcher(ctx context.Context, cfg WatcherConfig) error
- NewDebouncer(delay time.Duration, fn func([]string)) *Debouncer
- Debouncer.Add(path string)
- Types: PolicyTier, WatcherConfig, Debouncer
TYPE: WatcherConfig { ProjectRoot, WatchDir, Mode string; Debounce time.Duration; WorkerCmd string; Backend notify.NotifyBackend; Policy PolicyTier; AllowedScopes []string; BlockGovernanceSec map[string]bool; TestCmd string; WatchSource bool; SourcePaths []string }
TYPE: Debouncer { mu sync.Mutex; timer *time.Timer; pending map[string]struct{}; delay time.Duration; fn func([]string) }
DEPENDS: internal/claims, internal/git, internal/index, internal/notify (+2 more)
PATTERNS: file-watcher-debounce, policy-tier-gating, bidirectional-sync
USE WHEN: Running the Level 7 background daemon that auto-commits LOI changes or marks rooms stale on source edits.
DISAMBIGUATION: There is also a `runValidate` cobra command handler in `cmd/validate.go` (`cli.md`) — that is the public CLI entrypoint. The `runValidate` in this file is a private helper that subprocess-invokes `loi validate` from within the watcher event loop.

---
