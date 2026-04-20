package notify

import (
	"fmt"
	"time"
)

// NotifyEvent is sent to all backends.
type NotifyEvent struct {
	Type       string            `json:"event"`
	Timestamp  time.Time         `json:"timestamp"`
	Repo       string            `json:"repo,omitempty"`
	Path       string            `json:"path,omitempty"`
	Summary    string            `json:"summary,omitempty"`
	PRURL      string            `json:"pr_url,omitempty"`
	TableDiff  string            `json:"table_diff,omitempty"`
	Governance map[string]string `json:"governance,omitempty"`
	Rooms      []string          `json:"rooms,omitempty"`
	TestOutput string            `json:"test_output,omitempty"`
	Extra      map[string]any    `json:"extra,omitempty"`
}

// NotifyBackend sends events.
type NotifyBackend interface {
	Send(e NotifyEvent) error
}

// LoadBackend builds a backend from a config map.
//
// config["backend"]:        "stdout" | "file" | "webhook" | "slack"
// config["notify_url"]:     URL for webhook/slack
// config["file_path"]:      path for file backend
// config["auth_token_env"]: env var name for bearer token
func LoadBackend(config map[string]string) (NotifyBackend, error) {
	backend := config["backend"]
	if backend == "" {
		backend = "stdout"
	}

	switch backend {
	case "stdout":
		return &stdoutBackend{}, nil

	case "file":
		path := config["file_path"]
		if path == "" {
			path = "loi-events.jsonl"
		}
		return newFileBackend(path)

	case "webhook":
		url := config["notify_url"]
		if url == "" {
			return nil, fmt.Errorf("notify: webhook backend requires notify_url")
		}
		tokenEnv := config["auth_token_env"]
		return newWebhookBackend(url, tokenEnv), nil

	case "slack":
		url := config["notify_url"]
		if url == "" {
			return nil, fmt.Errorf("notify: slack backend requires notify_url")
		}
		return newSlackBackend(url), nil

	default:
		return nil, fmt.Errorf("notify: unknown backend %q", backend)
	}
}
