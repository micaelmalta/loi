---
room: auth/login.md
architectural_health: normal
security_tier: normal
see_also: ["auth/session.md"]
---

# LOI Room: auth/login

Source paths: internal/auth

## Entries

# login.go

DOES: Handles the login flow.
SYMBOLS:
- Login(ctx context.Context, creds Credentials) (Token, error)
DEPENDS: internal/config
PATTERNS: token-rotation
