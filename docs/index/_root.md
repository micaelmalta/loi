# LOI Index

Generated: 2026-04-11
Source paths: skills/loi/, .github/workflows/, README.md, LEVELS.md

## TASK → LOAD

| Task | Load |
|------|------|
| Understand what LOI does or how to install it | loi/concept.md |
| Learn the 10-level AI engineering roadmap | loi/concept.md |
| Use the LOI skill — navigate, generate, update, or implement | loi/skill.md |
| Look up LOI entry field format rules (DOES, SYMBOLS, etc.) | loi/skill.md |
| Validate an LOI index after generation | loi/automation.md |
| Validate only changed rooms before pushing (pre-push) | loi/automation.md |
| Install pre-push validation hook into a repo | loi/automation.md |
| Set up Level 7 background daemon (file watcher) | loi/automation.md |
| Set up pre-commit hook for automatic implementation | loi/automation.md |
| Set up CI/CD committee (Architect + Security) review on PRs | loi/automation.md |
| Check PATTERN table semantic validity | loi/automation.md |
| Diff TASK / PATTERN / GOVERNANCE table changes between commits | loi/automation.md |
| View governance flags across all rooms or repos | loi/automation.md |
| Coordinate between agents — claim, heartbeat, release a room | loi/automation.md |
| Query or validate AI-generated proposal provenance | loi/automation.md |
| Run LOI script test suite | loi/automation.md |

## PATTERN → LOAD

Cross-cutting behavioral patterns that span multiple rooms.

| Pattern | Load |
|---------|------|
| File watcher / debounce | loi/automation.md |
| Pre-commit / pre-push hook automation | loi/automation.md |
| CI/CD matrix persona review | loi/automation.md |
| Pluggable notify backend (strategy pattern) | loi/automation.md |
| Advisory file-based coordination with flock | loi/automation.md |
| Policy-tier gating (notify-only → full-auto) | loi/automation.md |

## 🚨 GOVERNANCE WATCHLIST

Rooms flagged by the RLM Committee for architectural drift or security audits.

| Room | Health | Security | Committee Note |
|------|--------|----------|----------------|
| `loi/automation.md` | `warning` | `sensitive` | "watcher.py passes --worker-cmd to subprocess unsanitized (RCE). pre-commit-loi.sh passes raw git diff via shell var (injection). loi-committee.yml embeds CHANGED_FILES into github-script env (runner path traversal). Acceptable for homelab operator-controlled config." |

## Buildings

| Subdomain | Description | Rooms |
|-----------|-------------|-------|
| loi/ | LOI skill definition, concept docs, and automation tooling | concept.md, skill.md, automation.md |
