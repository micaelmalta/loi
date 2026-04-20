package notify

import (
	"encoding/json"
	"fmt"
)

// stdoutBackend prints JSON-encoded NotifyEvents to stdout.
type stdoutBackend struct{}

// Send marshals e as JSON and prints it to stdout followed by a newline.
func (s *stdoutBackend) Send(e NotifyEvent) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("notify/stdout: marshal: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
