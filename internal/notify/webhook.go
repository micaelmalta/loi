package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// webhookBackend POSTs JSON-encoded NotifyEvents to an HTTP URL.
// An optional bearer token is read from an environment variable at send time.
type webhookBackend struct {
	url      string
	tokenEnv string
	client   *http.Client
}

// newWebhookBackend returns a webhookBackend targeting url.
// If tokenEnv is non-empty the value of os.Getenv(tokenEnv) is sent as a
// Bearer token on every request.
func newWebhookBackend(url, tokenEnv string) *webhookBackend {
	return &webhookBackend{
		url:      url,
		tokenEnv: tokenEnv,
		client:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Send marshals e as JSON and POSTs it to the configured URL.
func (b *webhookBackend) Send(e NotifyEvent) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("notify/webhook: marshal: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, b.url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("notify/webhook: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if b.tokenEnv != "" {
		if token := os.Getenv(b.tokenEnv); token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("notify/webhook: POST %s: %w", b.url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("notify/webhook: POST %s: unexpected status %d", b.url, resp.StatusCode)
	}
	return nil
}
