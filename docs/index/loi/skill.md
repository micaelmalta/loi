---
room: LOI Skill Definition & Format Reference
see_also: ["concept.md", "automation.md"]
hot_paths: "If adding a new sub-command -> update SKILL.md and FORMAT_REFERENCE.md Schema Extensions. If changing navigation protocol -> update Navigate section in SKILL.md."
security_tier: "normal"
architectural_health: "healthy"
---

# SKILL.md
DOES: Full LOI skill definition — 5 core modes (Navigate: 3-read campus→building→room with staleness check; Full-Generate: RLM Map/Critique/Reduce pipeline; Incremental-Generate: stale-room detection via git diff; Implement Level 7: intent delta → branch → implement → test → PR; Validate: run validate_loi.py) plus sub-command docs: Pattern Validation (Level 1/2), Table Diff, Governance Aggregation, Runtime Coordination (claim/heartbeat/release/status/summary), Proposal Provenance, Setup Hook; Automation Options with 6-tier --policy table and --notify-backend examples; Staleness Policy
SYMBOLS:
- Navigate: Load _root.md → subdomain/_root.md → room.md (3 reads, zero grep)
- Full-Generate: source census → coverage checklist → RLM Map/Critique/Reduce → write docs/index/**
- Implement: git diff HEAD -- room.md → extract DOES/SYMBOLS delta → checkout branch → implement → test → PR
USE WHEN: Using or extending the LOI skill; understanding the exact protocol for any mode or sub-command

# FORMAT_REFERENCE.md
DOES: Field guide for LOI entry format — defines all fields (DOES, SYMBOLS, TYPE, INTERFACE, ROUTES, CONFIG, TABLE, CONSUMERS, EMITS, PROPS, HOOKS, PATTERNS, USE WHEN) with when-to-include rules and full-signature examples; per-subdomain examples (infra, identity, api, data, integrations, workers, business, frontend); anti-patterns checklist; Schema Extensions: pattern_aliases frontmatter (enables Level 2 alias-aware pattern validation), pattern_metadata body block (first_introduced, last_validated, validation_source for freshness tracking), proposal_metadata header (proposal_id, generated_at, source_run_id, target_room + optional provenance fields)
USE WHEN: Generating or reviewing LOI entries; adding pattern_aliases for alias-based validation; structuring proposal_metadata in eval-generated proposals
