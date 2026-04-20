package claims

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

const (
	DefaultTTL     = 900 * time.Second
	HeartbeatGrace = 300 * time.Second
	ClaimsFile     = ".loi-claims.json"
	maxSummaries   = 100
)

// Claim mirrors the Python dict schema from runtime.py.
type Claim struct {
	ScopeType     string     `json:"scope_type"`
	ScopeID       string     `json:"scope_id"`
	Repo          string     `json:"repo"`
	AgentID       string     `json:"agent_id"`
	SessionID     string     `json:"session_id"`
	Intent        string     `json:"intent"`
	ClaimedAt     time.Time  `json:"claimed_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	Branch        string     `json:"branch,omitempty"`
}

// Summary is a room work summary published by an agent.
type Summary struct {
	ScopeID    string    `json:"scope_id"`
	AgentID    string    `json:"agent_id"`
	Text       string    `json:"summary"`
	RecordedAt time.Time `json:"recorded_at"`
}

type storeData struct {
	Claims    []Claim   `json:"claims"`
	Summaries []Summary `json:"summaries"`
}

// ClaimsStore is a JSON file-backed advisory claims store.
// All mutating operations acquire an exclusive advisory lock on
// path+".lock" for the duration of the read-modify-write cycle.
type ClaimsStore struct {
	path     string
	lockPath string
}

// NewClaimsStore returns a store backed by projectRoot/.loi-claims.json.
func NewClaimsStore(projectRoot string) *ClaimsStore {
	p := filepath.Join(projectRoot, ClaimsFile)
	return &ClaimsStore{
		path:     p,
		lockPath: p, // LockFile appends ".lock" internally
	}
}

// AllClaims returns all non-expired claims, pruning expired ones from disk.
func (s *ClaimsStore) AllClaims() ([]Claim, error) {
	var result []Claim
	err := s.withLock(func(data *storeData) error {
		result = data.Claims
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// GetClaimsFor returns non-expired claims for the given scopeID.
func (s *ClaimsStore) GetClaimsFor(scopeID string) ([]Claim, error) {
	all, err := s.AllClaims()
	if err != nil {
		return nil, err
	}
	var out []Claim
	for _, c := range all {
		if c.ScopeID == scopeID {
			out = append(out, c)
		}
	}
	return out, nil
}

// AddClaim adds a new claim, replacing any existing claim from the same agent+scope.
func (s *ClaimsStore) AddClaim(c Claim) error {
	return s.withLock(func(data *storeData) error {
		// Remove existing claim from same agent+scope
		filtered := data.Claims[:0]
		for _, existing := range data.Claims {
			if !(existing.ScopeID == c.ScopeID && existing.AgentID == c.AgentID) {
				filtered = append(filtered, existing)
			}
		}
		data.Claims = append(filtered, c)
		return nil
	})
}

// RemoveClaim removes a claim by scopeID + agentID. Returns false if not found.
func (s *ClaimsStore) RemoveClaim(scopeID, agentID string) (bool, error) {
	removed := false
	err := s.withLock(func(data *storeData) error {
		before := len(data.Claims)
		filtered := data.Claims[:0]
		for _, c := range data.Claims {
			if !(c.ScopeID == scopeID && c.AgentID == agentID) {
				filtered = append(filtered, c)
			}
		}
		data.Claims = filtered
		removed = len(data.Claims) < before
		return nil
	})
	return removed, err
}

// UpdateExpiry extends a claim's expiry by extra duration. Returns false if not found.
func (s *ClaimsStore) UpdateExpiry(scopeID, agentID string, extra time.Duration) (bool, error) {
	updated := false
	err := s.withLock(func(data *storeData) error {
		now := time.Now().UTC()
		for i := range data.Claims {
			c := &data.Claims[i]
			if c.ScopeID == scopeID && c.AgentID == agentID {
				exp := c.ExpiresAt
				if exp.IsZero() {
					exp = now
				}
				// max(exp, now) + extra — mirrors Python's max(exp, _now()) logic
				base := exp
				if now.After(exp) {
					base = now
				}
				newExp := base.Add(extra)
				c.ExpiresAt = newExp
				hb := now
				c.LastHeartbeat = &hb
				updated = true
			}
		}
		return nil
	})
	return updated, err
}

// AddSummary adds a room summary. Caps at maxSummaries (keeps most recent).
func (s *ClaimsStore) AddSummary(scopeID, agentID, text string) error {
	return s.withLock(func(data *storeData) error {
		data.Summaries = append(data.Summaries, Summary{
			ScopeID:    scopeID,
			AgentID:    agentID,
			Text:       text,
			RecordedAt: time.Now().UTC(),
		})
		// Keep only last maxSummaries — mirrors Python's summaries[-100:]
		if len(data.Summaries) > maxSummaries {
			data.Summaries = data.Summaries[len(data.Summaries)-maxSummaries:]
		}
		return nil
	})
}

// GetSummariesFor returns summaries for the given scopeID.
func (s *ClaimsStore) GetSummariesFor(scopeID string) ([]Summary, error) {
	var out []Summary
	err := s.withLock(func(data *storeData) error {
		for _, sm := range data.Summaries {
			if sm.ScopeID == scopeID {
				out = append(out, sm)
			}
		}
		return nil
	})
	return out, err
}

// ParseTTL parses "15m", "2h", "30s", "1d", or bare integer (seconds).
// Python source: _parse_ttl() in runtime.py
func ParseTTL(s string) (time.Duration, error) {
	if len(s) == 0 {
		return 0, fmt.Errorf("claims: empty TTL string")
	}
	suffix := s[len(s)-1]
	multipliers := map[byte]int64{
		's': 1,
		'm': 60,
		'h': 3600,
		'd': 86400,
	}
	if mult, ok := multipliers[suffix]; ok {
		n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("claims: invalid TTL %q: %w", s, err)
		}
		return time.Duration(n*mult) * time.Second, nil
	}
	// Bare integer seconds
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("claims: invalid TTL %q: %w", s, err)
	}
	return time.Duration(n) * time.Second, nil
}

// AgentID returns USER@hostname, mirroring _agent_id() in runtime.py.
func AgentID() string {
	user := os.Getenv("USER")
	if user == "" {
		user = os.Getenv("USERNAME") // Windows fallback
	}
	if user == "" {
		user = "agent"
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	return user + "@" + hostname
}

// SessionID returns agentID-timestamp, mirroring _session_id() in runtime.py.
func SessionID(agentID string) string {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05Z")
	return agentID + "-" + ts
}

// --- internal helpers ---

// load reads and prunes the JSON file. Caller must hold the lock.
// If the file is absent or corrupt, returns an empty storeData (tolerant).
// Tolerant parsing: claims whose time fields fail to parse are kept.
func (s *ClaimsStore) load() (*storeData, error) {
	data := &storeData{}

	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return data, nil
		}
		return data, nil // corrupt file → start fresh, same as Python
	}

	if err := json.Unmarshal(raw, data); err != nil {
		return data, nil // corrupt JSON → start fresh
	}

	// Prune expired claims — mirrors _prune_expired() in runtime.py.
	// Tolerant: if ExpiresAt is zero (parse failed), keep the claim.
	now := time.Now().UTC()
	live := data.Claims[:0]
	for _, c := range data.Claims {
		if c.ExpiresAt.IsZero() || c.ExpiresAt.After(now) {
			live = append(live, c)
		}
	}
	data.Claims = live

	return data, nil
}

// save writes data atomically via a temp file + rename. Caller must hold the lock.
func (s *ClaimsStore) save(data *storeData) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("claims: marshal: %w", err)
	}

	dir := filepath.Dir(s.path)
	// Write to a temp file in the same directory so rename is atomic.
	tmp, err := os.CreateTemp(dir, ".loi-claims-*.json.tmp")
	if err != nil {
		return fmt.Errorf("claims: create temp: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("claims: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("claims: close temp: %w", err)
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("claims: rename: %w", err)
	}
	return nil
}

// withLock acquires an exclusive lock, loads the store, calls fn, then saves.
func (s *ClaimsStore) withLock(fn func(*storeData) error) error {
	unlock, err := LockFile(s.path, 30*time.Second)
	if err != nil {
		return err
	}
	defer unlock()

	data, err := s.load()
	if err != nil {
		return err
	}
	if err := fn(data); err != nil {
		return err
	}
	return s.save(data)
}

