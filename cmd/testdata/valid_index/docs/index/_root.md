# LOI Index

Generated: 2026-04-19
Source paths: internal/

## TASK → LOAD

| Task | Load |
|------|------|
| Handle authentication | auth/_root.md |

## PATTERN → LOAD

Cross-cutting behavioral patterns that span multiple rooms.

| Pattern | Load |
|---------|------|
| token-rotation | auth/login.md |

## GOVERNANCE WATCHLIST

Rooms flagged by the RLM Committee for architectural drift or security audits.

| Room | Health | Security | Committee Note |
|------|--------|----------|----------------|
| `auth/session.md` | `warning` | `sensitive` | "Session tokens need audit" |

## Buildings

| Subdomain | Description | Rooms |
|-----------|-------------|-------|
| auth/ | Authentication and sessions | login.md, session.md |
