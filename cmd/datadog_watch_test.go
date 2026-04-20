package cmd_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDatadogWatch_missingAPIKey_exits1(t *testing.T) {
	root := initGitRepo(t)

	cmd := exec.Command(loiBin, "datadog-watch", "--query", "avg:cpu{*}", "--threshold", "80")
	cmd.Dir = root
	// Strip DD_API_KEY and DD_APPLICATION_KEY from environment.
	cmd.Env = envWithout(os.Environ(), "DD_API_KEY", "DD_APPLICATION_KEY")

	if err := cmd.Run(); err == nil {
		t.Error("expected non-zero exit when DD_API_KEY and DD_APPLICATION_KEY are missing")
	}
}

func TestDatadogWatch_missingQuery_exits1(t *testing.T) {
	root := initGitRepo(t)

	cmd := exec.Command(loiBin, "datadog-watch", "--threshold", "80")
	cmd.Dir = root
	cmd.Env = append(envWithout(os.Environ()), "DD_API_KEY=k", "DD_APPLICATION_KEY=a")

	var errBuf strings.Builder
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err == nil {
		t.Error("expected non-zero exit when --query is missing")
	}
}

func TestDatadogWatch_missingThreshold_exits1(t *testing.T) {
	root := initGitRepo(t)

	cmd := exec.Command(loiBin, "datadog-watch", "--query", "avg:cpu{*}")
	cmd.Dir = root
	cmd.Env = append(envWithout(os.Environ()), "DD_API_KEY=k", "DD_APPLICATION_KEY=a")

	if err := cmd.Run(); err == nil {
		t.Error("expected non-zero exit when --threshold is missing")
	}
}

// envWithout returns a copy of env with all entries whose key matches any of
// the given keys removed.
func envWithout(env []string, keys ...string) []string {
	keySet := make(map[string]bool, len(keys))
	for _, k := range keys {
		keySet[k] = true
	}
	var out []string
	for _, kv := range env {
		key := kv
		if idx := strings.IndexByte(kv, '='); idx >= 0 {
			key = kv[:idx]
		}
		if !keySet[key] {
			out = append(out, kv)
		}
	}
	return out
}
