package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// slackBackend posts events to a Slack incoming-webhook URL using Block Kit.
type slackBackend struct {
	url    string
	client *http.Client
}

// newSlackBackend returns a slackBackend targeting url.
func newSlackBackend(url string) *slackBackend {
	return &slackBackend{
		url:    url,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// slackBlock is a generic Block Kit block element.
type slackBlock map[string]any

// Send builds a Block Kit payload from e and POSTs it to the Slack webhook.
func (b *slackBackend) Send(e NotifyEvent) error {
	blocks := buildSlackBlocks(e)

	payload := map[string]any{
		"blocks": blocks,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("notify/slack: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, b.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("notify/slack: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("notify/slack: POST %s: %w", b.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notify/slack: POST %s: unexpected status %d", b.url, resp.StatusCode)
	}
	return nil
}

// buildSlackBlocks constructs the Block Kit block list for e.
func buildSlackBlocks(e NotifyEvent) []slackBlock {
	var blocks []slackBlock

	// --- Header block: event type + repo ---
	headerText := e.Type
	if e.Repo != "" {
		headerText = fmt.Sprintf("%s — %s", e.Type, e.Repo)
	}
	blocks = append(blocks, slackBlock{
		"type": "header",
		"text": map[string]any{
			"type":  "plain_text",
			"text":  headerText,
			"emoji": true,
		},
	})

	// --- Section with fields: path, governance health/security ---
	var fields []map[string]any

	if e.Path != "" {
		fields = append(fields, map[string]any{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Path*\n%s", e.Path),
		})
	}

	if health, ok := e.Governance["health"]; ok {
		fields = append(fields, map[string]any{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Health*\n%s", health),
		})
	}

	if security, ok := e.Governance["security"]; ok {
		fields = append(fields, map[string]any{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Security*\n%s", security),
		})
	}

	if e.Summary != "" {
		fields = append(fields, map[string]any{
			"type": "mrkdwn",
			"text": fmt.Sprintf("*Summary*\n%s", e.Summary),
		})
	}

	if len(fields) > 0 {
		section := slackBlock{
			"type":   "section",
			"fields": fields,
		}
		blocks = append(blocks, section)
	}

	// --- Optional code block: table_diff (truncated at 2000 chars) ---
	if e.TableDiff != "" {
		diff := e.TableDiff
		if len(diff) > 2000 {
			diff = diff[:2000] + "\n... (truncated)"
		}
		blocks = append(blocks, slackBlock{
			"type": "section",
			"text": map[string]any{
				"type": "mrkdwn",
				"text": fmt.Sprintf("*Table diff*\n```%s```", diff),
			},
		})
	}

	// --- Divider before actions ---
	if e.PRURL != "" {
		blocks = append(blocks, slackBlock{"type": "divider"})

		// Optional button: PR URL if present
		blocks = append(blocks, slackBlock{
			"type": "actions",
			"elements": []map[string]any{
				{
					"type": "button",
					"text": map[string]any{
						"type":  "plain_text",
						"text":  "View Pull Request",
						"emoji": true,
					},
					"url":   e.PRURL,
					"style": "primary",
				},
			},
		})
	}

	// --- Context block: timestamp ---
	blocks = append(blocks, slackBlock{
		"type": "context",
		"elements": []map[string]any{
			{
				"type": "mrkdwn",
				"text": fmt.Sprintf("_Sent at %s_", e.Timestamp.UTC().Format(time.RFC3339)),
			},
		},
	})

	return blocks
}
