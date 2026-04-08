# LOI Index

Generated: 2026-04-08
Source paths: skills/loi/, .github/workflows/, README.md, LEVELS.md

## TASK → LOAD

| Task | Load |
|------|------|
| Understand what LOI does or how to install it | loi/concept.md |
| Learn the 10-level AI engineering roadmap | loi/concept.md |
| Use the LOI skill — navigate, generate, update, or implement | loi/skill.md |
| Look up LOI entry field format rules (DOES, SYMBOLS, etc.) | loi/skill.md |
| Validate an LOI index after generation | loi/automation.md |
| Set up Level 7 background daemon (file watcher) | loi/automation.md |
| Set up pre-commit hook for automatic implementation | loi/automation.md |
| Set up CI/CD committee (Architect + Security) review on PRs | loi/automation.md |

## PATTERN → LOAD

Cross-cutting behavioral patterns that span multiple rooms.

| Pattern | Load |
|---------|------|
| File watcher / debounce | loi/automation.md |
| Pre-commit hook / git workflow automation | loi/automation.md |
| CI/CD matrix persona review | loi/automation.md |

## 🚨 GOVERNANCE WATCHLIST

Rooms flagged by the RLM Committee for architectural drift or security audits.

| Room | Health | Security | Committee Note |
|------|--------|----------|----------------|
| `loi/automation.md` | `healthy` | `sensitive` | "pre-commit-loi.sh passes raw git diff via shell variable — potential injection if index content is attacker-controlled. watcher.py passes user-supplied --worker-cmd to subprocess without argument sanitization." |

## Buildings

| Subdomain | Description | Rooms |
|-----------|-------------|-------|
| loi/ | LOI skill definition, concept docs, and automation tooling | concept.md, skill.md, automation.md |
