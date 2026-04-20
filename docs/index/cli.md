---
room: cli
see_also:
  - core/index.md
  - core/claims.md
  - core/notify.md
  - core/datadog.md
  - runtime.md
architectural_health: normal
security_tier: normal
hot_paths: generate.go, validate.go, watch.go, datadog_watch.go
---

# LOI Room: cli

Source paths: cmd/, main.go

## Entries

# check_stale.go

DOES: Implements `loi check-stale`, a pre-commit hook command that reads staged files, finds LOI room files covering them that are not also staged, and warns or exits 1. Setting `LOI_STALE_BLOCK=0` demotes the exit to a warning.
SYMBOLS:
- runCheckStale(cmd *cobra.Command, args []string) error
- isUnderIndexDir(file, indexPrefix string) bool
DEPENDS: internal/index, internal/git
PATTERNS: cobra-command, pre-commit-hook, git-integration

---

# claim.go

DOES: Registers six subcommands — `loi claim`, `loi heartbeat`, `loi release`, `loi status`, `loi summary`, and `loi claims` — all delegating to `internal/claims.ClaimsStore` for advisory room coordination.
SYMBOLS:
- (cobra.Command init blocks; no exported funcs beyond cobra registration)
DEPENDS: internal/claims, internal/git
PATTERNS: cobra-command, delegation
USE WHEN: Adding or modifying claim lifecycle subcommands; business logic lives in internal/claims.

---

# diff_tables.go

DOES: Implements `loi diff-tables <filepath> [--from HEAD~1] [--to HEAD]`, fetching two file revisions via `git.Show` then diffing their TASK/PATTERN/GOVERNANCE tables and printing the result.
SYMBOLS:
- runDiffTables(cmd *cobra.Command, args []string) error
DEPENDS: internal/index, internal/git
PATTERNS: cobra-command, diff-output

---

# generate.go

DOES: Implements `loi generate --scaffold [--room NAME] [--dry-run]`, reading `.codetect/symbols.db`, querying symbols by file, grouping into rooms, and writing LOI room markdown via `codetect.GenerateFileEntry` and `codetect.GenerateRoom`.
SYMBOLS:
- runGenerate(projectRoot, roomFilter string, dryRun bool) error
- loadExistingRooms(indexDir string) (map[string][]string, error)
- parseRoomFileHeadings(path string) ([]string, error)
- buildDepToRoomMap(rooms map[string][]string, symbolsByFile map[string][]Symbol, projectRoot, moduleName string, basenameIndex map[string][]string) map[string]string
- roomList(rooms map[string][]string) string
DEPENDS: internal/codetect
PATTERNS: cobra-command, scaffold-generation, symbol-driven
USE WHEN: Generating or refreshing room files from the codetect symbol database rather than editing room files manually.

---

# governance.go

DOES: Implements `loi governance [roots...] [--security] [--health] [--format text|json]`, collecting governance entries from `_root.md` watchlist tables and room frontmatter, sorting by combined severity.
SYMBOLS:
- runGovernance(cmd *cobra.Command, args []string) error
- collectGovernanceEntries(root, indexDir string) ([]index.GovEntry, error)
- printGovernanceTable(entries []index.GovEntry)
DEPENDS: internal/index
PATTERNS: cobra-command, governance-reporting

---

# main.go

DOES: Three-line binary entry point that imports `cmd` and calls `cmd.Execute()`.
SYMBOLS:
- main()
PATTERNS: entry-point

---

# proposals.go

DOES: Implements `loi proposals [--target-room] [--grader-version] [--failure-reason] [--validate]`, listing or validating AI-generated proposal `.md` files under `docs/index/proposals/`.
SYMBOLS:
- runProposals(cmd *cobra.Command, args []string) error
- findProposalFiles(indexDir string) ([]string, error)
DEPENDS: internal/index
PATTERNS: cobra-command, proposal-lifecycle

---

# root.go

DOES: Defines the root `cobra.Command`, `Execute()` entry point called from `main.go`, and `gitRepoRoot()` which resolves the git repo root stored in the `projectRoot` global via `PersistentPreRunE`.
SYMBOLS:
- Execute()
- gitRepoRoot() (string, error)
DEPENDS: github.com/spf13/cobra
PATTERNS: cobra-root, persistent-pre-run
USE WHEN: Adding global flags or pre-run hooks that must apply to every subcommand.

---

# setup_hook.go

DOES: Implements `loi setup-hook [--mode pre-push|pre-commit-stale] [--force]`, writing shell hook scripts from `cmd/hooks/` samples into the repository's `.git/hooks/` directory.
SYMBOLS:
- runSetupHook(cmd *cobra.Command, args []string) error
PATTERNS: cobra-command, git-hook-installation

---

# validate.go

DOES: Implements `loi validate [--changed-rooms] [--ci]`, checking campus `_root.md`, building `_root.md` files, room frontmatter, entry counts (>150 = warning), file references, and source coverage; collects results in `ValidationResult`.
SYMBOLS:
- runValidate(cmd *cobra.Command, args []string) error
- listRoomFiles(dir string) ([]string, error)
- collectMDs(dir string) ([]string, error)
- isCoveredByPrefix(sd string, covered map[string]bool) bool
- relOrAbs(root, path string) string
- printValidationResult(r *ValidationResult, ci bool)
TYPE: ValidationResult { Errors, Warnings []string; TotalRooms, TotalEntries int }
DEPENDS: internal/index, internal/git
PATTERNS: cobra-command, structural-validation, ci-integration
USE WHEN: Running structural LOI validation; for semantic pattern validation use validate_patterns.go.
DISAMBIGUATION: There is also a private `runValidate(projectRoot string)` in `internal/fswatch/watcher.go` (`runtime.md`) that shells out to the `loi validate` binary after LOI index changes. If the question is about what triggers validation during the watch loop, load `runtime.md` instead.

---

# validate_patterns.go

DOES: Implements `loi validate-patterns [--level 1|2]`, verifying every PATTERN entry in `_root.md` is semantically present in its target room body; level 2 additionally checks `pattern_aliases` and `last_validated` freshness.
SYMBOLS:
- runValidatePatterns(cmd *cobra.Command, args []string) error
DEPENDS: internal/index
PATTERNS: cobra-command, semantic-validation
USE WHEN: Validating pattern semantics; for structural room integrity use validate.go.

---

# datadog_watch.go

DOES: Implements `loi datadog-watch --query <expr> --threshold <n> [--operator] [--interval] [--worker-cmd claude] [--dry-run] [--notify-backend ...]`, reading DD_API_KEY and DD_APPLICATION_KEY from env, constructing a `datadog.PollConfig`, and delegating to `datadog.Poll`. On alert: writes a proposal file, pipes a focused intent-review prompt to `--worker-cmd` (default `claude`) via stdin, creates a draft PR, and sends a `datadog.alert` notify event. `--dry-run` prints alert details without git/notify/LLM ops.
SYMBOLS:
- runDatadogWatch(cmd *cobra.Command, args []string) error
- onDatadogAlert(series datadog.Series, rooms []string, backend notify.NotifyBackend)
- runAlertWorker(workerCmd string, series datadog.Series, rooms []string, proposalPath string)
- writeProposal(ts time.Time, metricSlug string, series datadog.Series, targetRoom string) string
- buildAlertPRBody(series datadog.Series, rooms []string, ts time.Time) string
DEPENDS: internal/datadog, internal/git, internal/notify
PATTERNS: cobra-command, delegation, proposal-lifecycle
USE WHEN: Modifying Datadog alert behaviour, proposal file format, or PR creation logic; polling core lives in internal/datadog.

---

# watch.go

DOES: Implements `loi watch [--mode notify|auto|dry-run] [--policy] [--notify-backend] ...`, parsing all flags into `fswatch.WatcherConfig` and delegating to `fswatch.StartWatcher`.
SYMBOLS:
- runWatch(cmd *cobra.Command, args []string) error
- resolveWatchPath(watchPath, defaultRoot string) (string, string, error)
- parsePolicy(s string) (fswatch.PolicyTier, error)
DEPENDS: internal/fswatch, internal/notify
PATTERNS: cobra-command, filesystem-watch, delegation
USE WHEN: Modifying watch flags or startup logic; watcher core logic lives in internal/fswatch.

---
