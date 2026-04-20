package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/micaelmalta/loi/internal/datadog"
	"github.com/micaelmalta/loi/internal/git"
	"github.com/micaelmalta/loi/internal/notify"
	"github.com/spf13/cobra"
)

var datadogWatchCmd = &cobra.Command{
	Use:   "datadog-watch",
	Short: "Poll a Datadog metric and open draft PRs when the threshold is breached",
	Long: `datadog-watch polls the Datadog Metrics Query API at a configurable interval.
When a metric series exceeds the threshold, it maps the series scope to LOI rooms,
writes a proposal file to docs/index/proposals/, and opens a draft PR.

Credentials are read from environment variables:
  DD_API_KEY            Datadog API key (required)
  DD_APPLICATION_KEY    Datadog application key (required)

Use --dry-run to print alerts and matched rooms without creating any git objects.`,
	RunE: runDatadogWatch,
}

var (
	ddQuery          string
	ddThreshold      float64
	ddOperator       string
	ddInterval       time.Duration
	ddWindow         time.Duration
	ddDryRun         bool
	ddWorkerCmd      string
	ddNotifyBackend  string
	ddNotifyURL      string
	ddNotifyFile     string
	ddNotifyTokenEnv string
)

func init() {
	rootCmd.AddCommand(datadogWatchCmd)

	f := datadogWatchCmd.Flags()
	f.StringVar(&ddQuery, "query", "", "Datadog metric query expression (required)")
	f.Float64Var(&ddThreshold, "threshold", 0, "Alert threshold value (required)")
	f.StringVar(&ddOperator, "operator", ">", "Comparison operator: >, >=, <, <=")
	f.DurationVar(&ddInterval, "interval", 60*time.Second, "Poll interval")
	f.DurationVar(&ddWindow, "window", 0, "Query lookback window (defaults to --interval)")
	f.BoolVar(&ddDryRun, "dry-run", false, "Print alerts without creating git objects or notifications")
	f.StringVar(&ddWorkerCmd, "worker-cmd", "claude", "LLM worker command to invoke on alert (e.g. \"claude --print\"); leave empty to skip")
	f.StringVar(&ddNotifyBackend, "notify-backend", "stdout", "Notification backend: stdout, file, webhook, slack")
	f.StringVar(&ddNotifyURL, "notify-url", "", "URL for webhook/slack backend")
	f.StringVar(&ddNotifyFile, "notify-file", "", "File path for file backend")
	f.StringVar(&ddNotifyTokenEnv, "notify-token-env", "", "Env var name holding the bearer token for webhook backend")

	datadogWatchCmd.MarkFlagRequired("query")
	datadogWatchCmd.MarkFlagRequired("threshold")
}

func runDatadogWatch(cmd *cobra.Command, args []string) error {
	apiKey := os.Getenv("DD_API_KEY")
	appKey := os.Getenv("DD_APPLICATION_KEY")
	if apiKey == "" || appKey == "" {
		return fmt.Errorf("datadog-watch: DD_API_KEY and DD_APPLICATION_KEY must be set")
	}

	var backend notify.NotifyBackend
	if !ddDryRun {
		cfg := map[string]string{
			"backend":        ddNotifyBackend,
			"notify_url":     ddNotifyURL,
			"file_path":      ddNotifyFile,
			"auth_token_env": ddNotifyTokenEnv,
		}
		var err error
		backend, err = notify.LoadBackend(cfg)
		if err != nil {
			return fmt.Errorf("datadog-watch: load notify backend: %w", err)
		}
	}

	client := datadog.NewClient(apiKey, appKey)

	pollCfg := datadog.PollConfig{
		Query:       ddQuery,
		Interval:    ddInterval,
		Window:      ddWindow,
		Threshold:   ddThreshold,
		Operator:    ddOperator,
		ProjectRoot: projectRoot,
		OnAlert: func(series datadog.Series, rooms []string) {
			onDatadogAlert(series, rooms, backend)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Fprintf(os.Stderr, "datadog-watch: polling %q every %s (threshold %s %g)\n",
		ddQuery, ddInterval, ddOperator, ddThreshold)

	return datadog.Poll(ctx, pollCfg, client)
}

func onDatadogAlert(series datadog.Series, rooms []string, backend notify.NotifyBackend) {
	ts := time.Now().UTC()
	metricSlug := strings.ReplaceAll(series.Metric, ".", "-")

	if ddDryRun {
		fmt.Printf("[dry-run] alert: metric=%s scope=%s value=%g threshold=%s%g\n",
			series.Metric, series.Scope, series.LastValue, ddOperator, ddThreshold)
		if len(rooms) > 0 {
			fmt.Printf("[dry-run] matched rooms: %s\n", strings.Join(rooms, ", "))
		} else {
			fmt.Println("[dry-run] no matching LOI rooms found")
		}
		return
	}

	targetRoom := ""
	if len(rooms) > 0 {
		rel, err := filepath.Rel(projectRoot, rooms[0])
		if err == nil {
			targetRoom = rel
		} else {
			targetRoom = rooms[0]
		}
	}

	// Write proposal file.
	proposalPath := writeProposal(ts, metricSlug, series, targetRoom)

	// Invoke LLM worker if configured.
	if ddWorkerCmd != "" {
		runAlertWorker(ddWorkerCmd, series, rooms, proposalPath)
	}

	// Create draft PR.
	branch := fmt.Sprintf("loi/datadog-alert-%s-%s", metricSlug, ts.Format("20060102-150405"))
	var prURL string
	if err := git.CheckoutNewBranch(projectRoot, branch); err == nil {
		files := []string{proposalPath}
		msg := fmt.Sprintf("loi: datadog alert — %s breached threshold (value: %g)", series.Metric, series.LastValue)
		if err := git.AddAndCommit(projectRoot, files, msg); err == nil {
			prURL, _ = git.CreatePR(projectRoot, branch,
				fmt.Sprintf("LOI: Datadog alert — %s", series.Metric),
				buildAlertPRBody(series, rooms, ts),
				true)
		}
	}

	if backend != nil {
		govInfo := map[string]string{}
		_ = backend.Send(notify.NotifyEvent{
			Type:       "datadog.alert",
			Timestamp:  ts,
			Repo:       git.RepoName(projectRoot),
			Path:       proposalPath,
			Summary:    fmt.Sprintf("%s=%g breached %s%g (scope: %s)", series.Metric, series.LastValue, ddOperator, ddThreshold, series.Scope),
			PRURL:      prURL,
			Rooms:      rooms,
			Governance: govInfo,
		})
	}
}

func writeProposal(ts time.Time, metricSlug string, series datadog.Series, targetRoom string) string {
	proposalDir := filepath.Join(projectRoot, "docs", "index", "proposals")
	os.MkdirAll(proposalDir, 0o755)

	proposalID := fmt.Sprintf("%s-%s", ts.Format("20060102-150405"), metricSlug)
	filename := proposalID + ".md"
	path := filepath.Join(proposalDir, filename)

	content := fmt.Sprintf(`---
proposal_id: %s
generated_at: %s
source_run_id: datadog-watch
target_room: %s
metric: %s
scope: %s
last_value: %g
threshold: %g
operator: %s
---

# Proposal: Review intent for %s (metric alert)

Metric %s exceeded threshold %g%s%g (scope: %s, last value: %g).

## Suggested intent review

Review the DOES field in the matched room. If this metric alert indicates a new
hot path, performance concern, or scaling requirement, update the room intent to
reflect it and consider opening a `+"`loi implement`"+` run.
`,
		proposalID,
		ts.Format(time.RFC3339),
		targetRoom,
		series.Metric,
		series.Scope,
		series.LastValue,
		ddThreshold,
		ddOperator,
		series.Scope,
		series.Metric,
		series.LastValue,
		ddOperator,
		ddThreshold,
		series.Scope,
		series.LastValue,
	)

	os.WriteFile(path, []byte(content), 0o644)
	return path
}

// runAlertWorker pipes a focused prompt to the LLM worker (e.g. claude --print).
// The prompt instructs the model to review the matched rooms in light of the alert
// and propose intent updates. Output is printed to stderr so it stays out of
// notification pipelines.
func runAlertWorker(workerCmd string, series datadog.Series, rooms []string, proposalPath string) {
	roomList := "(no rooms matched)"
	if len(rooms) > 0 {
		roomList = strings.Join(rooms, "\n- ")
		roomList = "- " + roomList
	}

	prompt := fmt.Sprintf(`A Datadog metric alert has fired. Review the matched LOI rooms and propose intent updates.

## Alert

Metric:    %s
Scope:     %s
Value:     %g
Threshold: %s%g
Proposal:  %s

## Matched LOI Rooms

%s

## Task

1. Read each matched room file listed above.
2. Assess whether the metric alert (e.g. CPU spike, latency increase, error rate) suggests the room's DOES or PATTERNS fields are stale or incomplete.
3. If an update is warranted, output a patch for the relevant DOES or PATTERNS field in unified diff format.
4. If no update is needed, explain briefly why the current intent already covers this scenario.

Keep the response concise. Do not modify any files directly.`,
		series.Metric, series.Scope, series.LastValue, ddOperator, ddThreshold,
		proposalPath, roomList)

	args := strings.Fields(workerCmd)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = projectRoot
	cmd.Stdin = bytes.NewBufferString(prompt)
	cmd.Stdout = os.Stderr // route LLM output to stderr, not stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "datadog-watch: worker %q: %v\n", workerCmd, err)
	}
}

func buildAlertPRBody(series datadog.Series, rooms []string, ts time.Time) string {
	roomList := "(none matched)"
	if len(rooms) > 0 {
		roomList = strings.Join(rooms, "\n- ")
		roomList = "- " + roomList
	}

	body := map[string]any{
		"metric":    series.Metric,
		"scope":     series.Scope,
		"value":     series.LastValue,
		"threshold": fmt.Sprintf("%s%g", ddOperator, ddThreshold),
		"rooms":     rooms,
		"timestamp": ts.Format(time.RFC3339),
	}
	bodyJSON, _ := json.MarshalIndent(body, "", "  ")

	return fmt.Sprintf(`## Datadog Alert

**Metric:** %s
**Scope:** %s
**Last value:** %g (threshold: %s%g)
**Timestamp:** %s

## Matched LOI Rooms

%s

## Alert Context

`+"```json\n%s\n```", series.Metric, series.Scope, series.LastValue, ddOperator, ddThreshold,
		ts.Format(time.RFC3339), roomList, string(bodyJSON))
}
