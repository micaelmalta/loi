---
room: auth/session.md
architectural_health: normal
security_tier: normal
see_also: ["auth/login.md"]
---

# LOI Room: auth/session

Source paths: internal/auth

## Entries

# session.go

DOES: Manages session tokens and TTL.
SYMBOLS:
- CreateSession(ctx context.Context, userID string) (Session, error)
PATTERNS: token-rotation
