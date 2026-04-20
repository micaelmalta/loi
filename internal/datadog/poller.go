package datadog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/micaelmalta/loi/internal/index"
)

// AlertCallback is called for each series that breaches the threshold.
// rooms is the list of LOI room paths that cover the metric's scope.
type AlertCallback func(series Series, rooms []string)

// PollConfig configures the Poll loop.
type PollConfig struct {
	Query       string
	Interval    time.Duration
	Window      time.Duration // lookback window for each query; defaults to Interval
	Threshold   float64
	Operator    string // ">", ">=", "<", "<=" — default ">"
	ProjectRoot string
	OnAlert     AlertCallback
}

// Poll runs a polling loop, calling cfg.OnAlert for each series that breaches
// the threshold. Blocks until ctx is cancelled or ErrAuthFailure is returned
// by the client.
func Poll(ctx context.Context, cfg PollConfig, client *Client) error {
	if cfg.Window == 0 {
		cfg.Window = cfg.Interval
	}
	if cfg.Operator == "" {
		cfg.Operator = ">"
	}
	if cfg.Interval == 0 {
		cfg.Interval = 60 * time.Second
	}

	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			series, err := client.QueryLastValues(ctx, cfg.Query, cfg.Window)
			if err != nil {
				if err == ErrAuthFailure {
					return fmt.Errorf("poll: %w", err)
				}
				// Non-fatal errors: log and continue.
				fmt.Printf("datadog poll error: %v\n", err)
				continue
			}
			for _, s := range series {
				if breaches(s.LastValue, cfg.Threshold, cfg.Operator) {
					rooms := mapScopeToRooms(cfg.ProjectRoot, s.Scope)
					cfg.OnAlert(s, rooms)
				}
			}
		}
	}
}

// breaches returns true if value satisfies `value <op> threshold`.
func breaches(value, threshold float64, op string) bool {
	switch op {
	case ">=":
		return value >= threshold
	case "<":
		return value < threshold
	case "<=":
		return value <= threshold
	default: // ">"
		return value > threshold
	}
}

// mapScopeToRooms maps a Datadog scope tag (e.g. "service:api-gateway") to
// LOI room paths by treating the tag value as a source path component.
// Falls back to the full scope string if no ":" is present.
func mapScopeToRooms(projectRoot, scope string) []string {
	// Strip "key:" prefix — "service:api-gateway" → "api-gateway"
	pathHint := scope
	if idx := strings.Index(scope, ":"); idx >= 0 {
		pathHint = scope[idx+1:]
	}

	rooms, err := index.FindCoveringRooms(projectRoot, pathHint, index.CoverByContent)
	if err != nil || len(rooms) == 0 {
		// Also try CoverBySourcePaths as a fallback.
		rooms, _ = index.FindCoveringRooms(projectRoot, pathHint, index.CoverBySourcePaths)
	}
	return rooms
}
