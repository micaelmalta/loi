---
room: core/claims
see_also:
  - core/index.md
  - core/notify.md
architectural_health: normal
security_tier: sensitive
hot_paths: store.go, conflict.go, lock.go
---

# LOI Room: core/claims

Source paths: internal/claims/

## Entries

# conflict.go

DOES: Evaluates incoming agent intent against existing room claims using a fixed conflict matrix; returns the most severe `IntentAction` and a human-readable advisory message.
SYMBOLS:
- CheckConflict(existing []Claim, incomingIntent string) (IntentAction, string)
- Types: IntentAction
- Constants: ActionAllow, ActionAllowWithVisibility, ActionAllowWithWarning, ActionConflict, ActionGovernanceSensitive
PATTERNS: conflict-matrix
USE WHEN: Before claiming a room, to detect edit+edit conflicts, security-sweep+edit governance blocks, or review+edit warnings.

---

# lock.go

DOES: Acquires an exclusive advisory lock on `path+".lock"` with exponential backoff retry up to a timeout; delegates the syscall to platform-specific `tryLock`/`doUnlock` implementations.
SYMBOLS:
- LockFile(path string, timeout time.Duration) (unlock func(), err error)
USE WHEN: Wrapping any read-modify-write cycle on `.loi-claims.json` to prevent concurrent corruption.

---

# lock_unix.go

DOES: Unix implementation of `tryLock`/`doUnlock` using `syscall.Flock` (LOCK_EX|LOCK_NB); returns an fd and an unlock closure that calls LOCK_UN and closes the file.
SYMBOLS:
- tryLock(lockPath string) (fd uintptr, unlock func(), err error)
- isWouldBlock(err error) bool

---

# lock_windows.go

DOES: Windows implementation of `tryLock`/`doUnlock` using `LockFileEx` Win32 API, mirroring the Unix Flock semantics for exclusive non-blocking acquisition.
SYMBOLS:
- tryLock(lockPath string) (fd uintptr, unlock func(), err error)
- isWouldBlock(err error) bool

---

# store.go

DOES: JSON file-backed advisory claims store at `.loi-claims.json`; every mutating operation holds an exclusive flock via `withLock` then atomically saves. Provides claim CRUD, expiry heartbeat extension, room summary storage, agent/session ID derivation, and TTL parsing.
SYMBOLS:
- NewClaimsStore(projectRoot string) *ClaimsStore
- ClaimsStore.AllClaims() ([]Claim, error)
- ClaimsStore.GetClaimsFor(scopeID string) ([]Claim, error)
- ClaimsStore.AddClaim(c Claim) error
- ClaimsStore.RemoveClaim(scopeID, agentID string) (bool, error)
- ClaimsStore.UpdateExpiry(scopeID, agentID string, extra time.Duration) (bool, error)
- ClaimsStore.AddSummary(scopeID, agentID, text string) error
- ClaimsStore.GetSummariesFor(scopeID string) ([]Summary, error)
- AgentID() string
- SessionID(agentID string) string
- ParseTTL(s string) (time.Duration, error)
- Types: ClaimsStore, Claim, Summary
- Constants: DefaultTTL, HeartbeatGrace, ClaimsFile
TYPE: Claim { ScopeType, ScopeID, Repo, AgentID, SessionID, Intent string; ClaimedAt, ExpiresAt time.Time; LastHeartbeat *time.Time; Branch string }
TYPE: Summary { ScopeID, AgentID, Text string; RecordedAt time.Time }
DEPENDS: internal/claims (lock.go)
PATTERNS: advisory-coordination, file-locking, atomic-write
USE WHEN: Registering, extending, or releasing an agent's room claim; checking existing claims before starting work; storing handoff summaries between agents.

---

# store_test.go

DOES: Tests ClaimsStore AddClaim, RemoveClaim, UpdateExpiry, AddSummary, GetSummariesFor, and concurrent locking correctness.

---
