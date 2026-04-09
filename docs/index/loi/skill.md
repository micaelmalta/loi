---
room: LOI Skill Definition & Format Reference
see_also: ["concept.md", "automation.md"]
hot_paths: "If changing navigation protocol -> update SKILL.md Navigate section. If adding a new field type -> update FORMAT_REFERENCE.md Field Guide table and Examples section."
security_tier: "standard"
architectural_health: "healthy"
---

# SKILL.md
DOES: Full LOI skill definition — Navigate mode (3-read Campus→Building→Room with staleness check via git diff, zero grep), Full-Generate mode (RLM-driven census → coverage checklist → Map/Critique/Reduce pipeline → write docs/index/**), Incremental-Generate mode (detect stale rooms via git diff, regenerate only affected rooms), Implement mode (Level 7: diff room file → extract intent delta → branch → implement per-entry → run tests → open PR), Validate mode (run validate_loi.py), Staleness Policy
SYMBOLS:
- Navigate: Load _root.md → subdomain/_root.md → room.md (3 reads, zero grep)
- Full-Generate: Glob source files → coverage checklist → RLM Map/Critique/Reduce → write docs/index/**
- Implement: git diff HEAD -- room.md → extract DOES/SYMBOLS delta → checkout branch → implement → test → PR
USE WHEN: Using or extending the LOI skill itself; understanding the exact protocol for any of the five modes

# FORMAT_REFERENCE.md
DOES: Field guide for LOI entry format — defines all applicable fields (DOES, SYMBOLS, TYPE, INTERFACE, ROUTES, CONFIG, TABLE, CONSUMERS, USE WHEN, PATTERNS) with when-to-include rules and full-signature examples; provides per-subdomain examples (infra, identity, api, data, integrations, workers, business, frontend); documents anti-patterns (generic DOES, behavioral strategy hidden in DOES, incomplete SYMBOLS, empty fields, alphabet-splitting); includes a checklist for new domain files
USE WHEN: Generating or reviewing LOI entries; deciding which fields apply to a given file; checking DOES/SYMBOLS style
