package claims

import (
	"fmt"
	"time"
)

// IntentAction is the result of a conflict check.
type IntentAction string

const (
	ActionAllow               IntentAction = "allow"
	ActionAllowWithVisibility IntentAction = "allow_with_visibility"
	ActionAllowWithWarning    IntentAction = "allow_with_warning"
	ActionConflict            IntentAction = "conflict"
	ActionGovernanceSensitive IntentAction = "governance_sensitive"
)

// conflictMatrix mirrors INTENT_CONFLICT_MATRIX from runtime.py EXACTLY.
// Keys not in the matrix default to ActionAllowWithVisibility.
var conflictMatrix = map[[2]string]IntentAction{
	{"read", "read"}:           ActionAllow,
	{"read", "edit"}:           ActionAllowWithVisibility,
	{"edit", "read"}:           ActionAllowWithVisibility,
	{"edit", "edit"}:           ActionConflict,
	{"review", "edit"}:         ActionAllowWithWarning,
	{"security-sweep", "edit"}: ActionGovernanceSensitive,
}

// CheckConflict returns the most severe action and a human-readable message
// given a list of existing claims and an incoming intent.
// Returns (ActionAllow, "") if existing is empty.
// Severity order: conflict > governance_sensitive > allow_with_warning > allow_with_visibility > allow.
// Message format mirrors check_conflict() in runtime.py.
// Python source: check_conflict() in runtime.py
func CheckConflict(existing []Claim, incomingIntent string) (IntentAction, string) {
	if len(existing) == 0 {
		return ActionAllow, ""
	}

	for _, c := range existing {
		existingIntent := c.Intent
		if existingIntent == "" {
			existingIntent = "read"
		}

		action, ok := conflictMatrix[[2]string{existingIntent, incomingIntent}]
		if !ok {
			action = ActionAllowWithVisibility
		}

		agent := c.AgentID
		if agent == "" {
			agent = "unknown"
		}

		switch action {
		case ActionConflict:
			msg := fmt.Sprintf(
				"CONFLICT: '%s' already holds an edit claim on this room "+
					"(branch: %s, expires: %s). "+
					"Use `/loi status <room>` to see the active claim.",
				agent, c.Branch, formatExpiry(c.ExpiresAt),
			)
			return ActionConflict, msg

		case ActionGovernanceSensitive:
			msg := fmt.Sprintf(
				"CAUTION: '%s' is running a security-sweep on this room. "+
					"Edit claims on security-sensitive rooms require explicit override.",
				agent,
			)
			return ActionGovernanceSensitive, msg

		case ActionAllowWithWarning:
			msg := fmt.Sprintf(
				"WARNING: '%s' has a review claim. Edits may conflict.",
				agent,
			)
			return ActionAllowWithWarning, msg

		case ActionAllowWithVisibility:
			msg := fmt.Sprintf(
				"NOTE: '%s' has a %s claim on this room.",
				agent, existingIntent,
			)
			return ActionAllowWithVisibility, msg
		}
	}

	return ActionAllow, ""
}

// formatExpiry formats a time.Time as an ISO 8601 string, matching Python's
// isoformat() output used in the claims file.
func formatExpiry(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
