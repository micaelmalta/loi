# LOI Index

Generated: 2026-04-19
Source paths: cmd/, internal/, main.go, skills/loi/, .github/workflows/, README.md, LEVELS.md

## TASK → LOAD

| Task | Load |
|------|------|
| Understand what LOI does or how to install it | loi/concept.md |
| Learn the 10-level AI engineering roadmap | loi/concept.md |
| Use the LOI skill — navigate, generate, update, or implement | loi/skill.md |
| Look up LOI entry field format rules (DOES, SYMBOLS, etc.) | loi/skill.md |
| Validate an LOI index after generation | loi/automation.md |
| Validate only changed rooms before pushing (pre-push) | loi/automation.md |
| Warn at commit time when source changed without index update | loi/automation.md |
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
| Add or modify a CLI subcommand (loi generate, validate, watch, etc.) | cli.md |
| Modify LOI scaffold generation from codetect symbols | cli.md |
| Modify validation rules or exit codes | cli.md |
| Modify watch daemon flags or startup logic | cli.md |
| Modify claim/release/heartbeat subcommand behaviour | cli.md |
| Parse or update YAML frontmatter in room files | core/_root.md |
| Find which rooms cover a changed source file | core/_root.md |
| Parse or diff TASK/PATTERN/GOVERNANCE tables programmatically | core/_root.md |
| Claim, check conflicts for, or release a room | core/_root.md |
| Send LOI notifications (Slack, webhook, file, stdout) | core/_root.md |
| Watch docs/index/ or source dirs for changes and react | runtime.md |
| Run git operations (diff, branch, commit, push, create PR) | runtime.md |
| Auto-detect and run the project test suite | runtime.md |
| Read codetect symbols.db and generate LOI room entries | runtime.md |

## PATTERN → LOAD

Cross-cutting behavioral patterns that span multiple rooms.

| Pattern | Load |
|---------|------|
| File watcher / debounce | loi/automation.md, runtime.md |
| Pre-commit / pre-push hook automation | loi/automation.md, cli.md |
| CI/CD matrix persona review | loi/automation.md |
| Pluggable notify backend (strategy pattern) | loi/automation.md, core/notify.md |
| Advisory file-based coordination with flock | loi/automation.md, core/claims.md |
| Policy-tier gating (notify-only → full-auto) | loi/automation.md, runtime.md |
| Cobra command registration | cli.md |
| LOI scaffold generation from codetect | cli.md, runtime.md |
| Intent-to-code / code-to-intent bidirectional sync | runtime.md |

## 🚨 GOVERNANCE WATCHLIST

Rooms flagged by the RLM Committee for architectural drift or security audits.

| Room | Health | Security | Committee Note |
|------|--------|----------|----------------|
| `loi/automation.md` | `warning` | `high` | "Fixed shell quoting, shutil.which() guard, and workspace path validation. Remaining: --worker-cmd is operator-controlled and intentionally unrestricted beyond existence check." |
| `core/claims.md` | `normal` | `sensitive` | "Handles agent identity, session IDs, and advisory lock files. Review any changes to ClaimsStore or LockFile for TOCTOU risks." |

## Buildings

| Subdomain | Description | Rooms |
|-----------|-------------|-------|
| loi/ | LOI skill definition, concept docs, and automation tooling | concept.md, skill.md, automation.md |
| cli.md | Cobra CLI layer — all `loi` subcommands | (flat file) |
| core/ | Index parsing, claims coordination, notification backends | index.md, claims.md, notify.md |
| runtime.md | Flat: file watcher, git ops, test detection, codetect symbols | (flat file) |
