package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/micaelmalta/loi/internal/fswatch"
	"github.com/micaelmalta/loi/internal/notify"
	"github.com/spf13/cobra"
)

var (
	watchPath               string
	watchMode               string
	watchDebounce           float64
	watchWorkerCmd          string
	watchNotifyBackend      string
	watchNotifyURL          string
	watchNotifyFile         string
	watchNotifyTokenEnv     string
	watchSource             bool
	watchSourcePaths        string
	watchTestCmd            string
	watchPolicy             string
	watchAllowedScopes      string
	watchBlockGovSec        string
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch LOI index (and optionally source) for changes and react automatically",
	Long: `watch monitors the docs/index/ directory for changes to LOI room files
and optionally source directories for code changes.

In notify mode (default) it creates draft PRs and sends events.
In auto mode it also invokes a worker command and runs the test suite.
In dry-run mode it only prints what it would do.

Ctrl+C stops the watcher gracefully.`,
	RunE: runWatch,
}

func init() {
	watchCmd.Flags().StringVar(&watchPath, "watch-path", "", "Project root or docs/index/ dir (default: current dir)")
	watchCmd.Flags().StringVar(&watchMode, "mode", "notify", "Watch mode: notify|auto|dry-run")
	watchCmd.Flags().Float64Var(&watchDebounce, "debounce", 5.0, "Debounce window in seconds")
	watchCmd.Flags().StringVar(&watchWorkerCmd, "worker-cmd", "claude", "Worker command to run in auto mode")
	watchCmd.Flags().StringVar(&watchNotifyBackend, "notify-backend", "stdout", "Notification backend: stdout|file|webhook|slack")
	watchCmd.Flags().StringVar(&watchNotifyURL, "notify-url", "", "URL for webhook or Slack backend")
	watchCmd.Flags().StringVar(&watchNotifyFile, "notify-file", "loi-events.jsonl", "File path for file backend")
	watchCmd.Flags().StringVar(&watchNotifyTokenEnv, "notify-token-env", "", "Env var name for bearer token (webhook backend)")
	watchCmd.Flags().BoolVar(&watchSource, "watch-source", true, "Watch source files for Code-to-Intent changes")
	watchCmd.Flags().StringVar(&watchSourcePaths, "source-paths", "", "Comma-separated extra source directories to watch")
	watchCmd.Flags().StringVar(&watchTestCmd, "test-cmd", "", "Explicit test command (auto-detected if empty)")
	watchCmd.Flags().StringVar(&watchPolicy, "policy", "full-auto",
		"Policy tier: notify-only|draft-only|docs-safe|tests-safe|scoped-code-safe|full-auto")
	watchCmd.Flags().StringVar(&watchAllowedScopes, "allowed-scopes", "", "Comma-separated glob patterns for scoped-code-safe policy")
	watchCmd.Flags().StringVar(&watchBlockGovSec, "block-governance-security", "sensitive",
		"Comma-separated security tiers to block in auto mode")

	rootCmd.AddCommand(watchCmd)
}

func runWatch(cmd *cobra.Command, args []string) error {
	// 1. Resolve watch path.
	root, watchDir, err := resolveWatchPath(watchPath, projectRoot)
	if err != nil {
		return err
	}

	// 2. Build notify backend.
	backendConfig := map[string]string{
		"backend":        watchNotifyBackend,
		"notify_url":     watchNotifyURL,
		"file_path":      watchNotifyFile,
		"auth_token_env": watchNotifyTokenEnv,
	}
	backend, err := notify.LoadBackend(backendConfig)
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}

	// 3. Validate worker-cmd exists in PATH for auto mode.
	if watchMode == "auto" && watchWorkerCmd != "" {
		if _, lookErr := exec.LookPath(strings.Fields(watchWorkerCmd)[0]); lookErr != nil {
			return fmt.Errorf("watch: worker-cmd %q not found in PATH: %w", watchWorkerCmd, lookErr)
		}
	}

	// 4. Parse policy.
	policy, err := parsePolicy(watchPolicy)
	if err != nil {
		return err
	}

	// 5. Parse allowed scopes.
	var allowedScopes []string
	if watchAllowedScopes != "" {
		for _, s := range strings.Split(watchAllowedScopes, ",") {
			if t := strings.TrimSpace(s); t != "" {
				allowedScopes = append(allowedScopes, t)
			}
		}
	}

	// 6. Parse blocked governance security tiers.
	blockGovSec := make(map[string]bool)
	for _, tier := range strings.Split(watchBlockGovSec, ",") {
		if t := strings.TrimSpace(tier); t != "" {
			blockGovSec[t] = true
		}
	}

	// 7. Parse source paths.
	var sourcePaths []string
	if watchSourcePaths != "" {
		for _, p := range strings.Split(watchSourcePaths, ",") {
			if t := strings.TrimSpace(p); t != "" {
				abs := t
				if !filepath.IsAbs(abs) {
					abs = filepath.Join(root, t)
				}
				sourcePaths = append(sourcePaths, abs)
			}
		}
	}

	// 8. Build WatcherConfig.
	cfg := fswatch.WatcherConfig{
		ProjectRoot:        root,
		WatchDir:           watchDir,
		Mode:               watchMode,
		Debounce:           time.Duration(watchDebounce * float64(time.Second)),
		WorkerCmd:          watchWorkerCmd,
		Backend:            backend,
		Policy:             policy,
		AllowedScopes:      allowedScopes,
		BlockGovernanceSec: blockGovSec,
		TestCmd:            watchTestCmd,
		WatchSource:        watchSource,
		SourcePaths:        sourcePaths,
	}

	// 9. Print startup info.
	fmt.Printf("loi watch starting\n")
	fmt.Printf("  project root:  %s\n", root)
	fmt.Printf("  watch dir:     %s\n", watchDir)
	fmt.Printf("  mode:          %s\n", watchMode)
	fmt.Printf("  backend:       %s\n", watchNotifyBackend)
	fmt.Printf("  policy:        %s\n", watchPolicy)
	fmt.Printf("  watch source:  %v\n", watchSource)
	fmt.Printf("  debounce:      %.1fs\n", watchDebounce)
	if watchMode == "auto" {
		fmt.Printf("  worker-cmd:    %s\n", watchWorkerCmd)
	}
	fmt.Printf("Press Ctrl+C to stop.\n\n")

	// 10. Start watcher, block until signal.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	return fswatch.StartWatcher(ctx, cfg)
}

// resolveWatchPath resolves the --watch-path flag and returns (projectRoot, watchDir).
// If watchPath has a docs/index subdir it is used as watchDir, otherwise
// watchPath itself is watchDir and its parent (or the flag value) is projectRoot.
func resolveWatchPath(watchPath, defaultRoot string) (string, string, error) {
	root := defaultRoot

	if watchPath != "" {
		// If the provided path is an absolute path, use it directly.
		if !filepath.IsAbs(watchPath) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", "", fmt.Errorf("watch: getwd: %w", err)
			}
			watchPath = filepath.Join(cwd, watchPath)
		}
		root = watchPath
	}

	// Check whether root has a docs/index subdir.
	candidate := filepath.Join(root, "docs", "index")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return root, candidate, nil
	}

	// Perhaps root itself is the docs/index dir.
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		// Walk up to find a project root that contains docs/index.
		dir := root
		for {
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			if _, err := os.Stat(filepath.Join(parent, "docs", "index")); err == nil {
				return parent, filepath.Join(parent, "docs", "index"), nil
			}
			dir = parent
		}
		// Fallback: treat root as both project root and watch dir.
		return root, root, nil
	}

	return root, filepath.Join(root, "docs", "index"), nil
}

// parsePolicy converts the string flag value to a PolicyTier.
func parsePolicy(s string) (fswatch.PolicyTier, error) {
	switch s {
	case "notify-only":
		return fswatch.PolicyNotifyOnly, nil
	case "draft-only":
		return fswatch.PolicyDraftOnly, nil
	case "docs-safe":
		return fswatch.PolicyDocsSafe, nil
	case "tests-safe":
		return fswatch.PolicyTestsSafe, nil
	case "scoped-code-safe":
		return fswatch.PolicyScopedCodeSafe, nil
	case "full-auto":
		return fswatch.PolicyFullAuto, nil
	default:
		return 0, fmt.Errorf("watch: unknown policy %q (valid: notify-only|draft-only|docs-safe|tests-safe|scoped-code-safe|full-auto)", s)
	}
}
