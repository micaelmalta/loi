"""Tests for governance.py — GOVERNANCE WATCHLIST aggregation."""
import textwrap
from pathlib import Path
import pytest
from governance import (
    parse_governance_table,
    parse_room_frontmatter_flags,
    _sec_rank,
    _health_rank,
    aggregate_governance,
)


SAMPLE_ROOT = textwrap.dedent("""\
    # LOI Index

    ## GOVERNANCE WATCHLIST

    | Room | Health | Security | Note |
    |------|--------|----------|------|
    | auth/jwt.md | normal | sensitive | JWT signing |
    | broker/core.md | warning | normal | Single-threaded mutex |
""")


class TestSeverityRanks:
    def test_sec_rank_ordering(self):
        assert _sec_rank("normal") < _sec_rank("high") < _sec_rank("sensitive")

    def test_health_rank_ordering(self):
        assert _health_rank("normal") < _health_rank("warning") < _health_rank("critical")

    def test_unknown_values_return_zero(self):
        assert _sec_rank("unknown") == 0
        assert _health_rank("bogus") == 0


class TestParseGovernanceTable:
    def test_parses_two_rows(self):
        rows = parse_governance_table(SAMPLE_ROOT)
        assert len(rows) == 2

    def test_first_row_fields(self):
        rows = parse_governance_table(SAMPLE_ROOT)
        assert rows[0]["room"] == "auth/jwt.md"
        assert rows[0]["health"] == "normal"
        assert rows[0]["security"] == "sensitive"
        assert "JWT" in rows[0]["note"]

    def test_second_row_health_warning(self):
        rows = parse_governance_table(SAMPLE_ROOT)
        assert rows[1]["room"] == "broker/core.md"
        assert rows[1]["health"] == "warning"

    def test_empty_text_returns_empty_list(self):
        assert parse_governance_table("") == []

    def test_no_governance_section_returns_empty(self):
        assert parse_governance_table("# Just a heading\n\nSome prose.") == []

    def test_strips_backticks_from_room(self):
        md = textwrap.dedent("""\
            ## GOVERNANCE WATCHLIST

            | Room | Health | Security | Note |
            |------|--------|----------|------|
            | `auth/jwt.md` | normal | sensitive | note |
        """)
        rows = parse_governance_table(md)
        assert rows[0]["room"] == "auth/jwt.md"

    def test_lowercase_normalization(self):
        md = textwrap.dedent("""\
            ## GOVERNANCE WATCHLIST

            | Room | Health | Security | Note |
            |------|--------|----------|------|
            | room.md | WARNING | SENSITIVE | note |
        """)
        rows = parse_governance_table(md)
        assert rows[0]["health"] == "warning"
        assert rows[0]["security"] == "sensitive"


class TestParseFrontmatterFlags:
    def test_extracts_flags(self, tmp_path):
        room = tmp_path / "auth.md"
        room.write_text(textwrap.dedent("""\
            ---
            room: Auth
            architectural_health: warning
            security_tier: sensitive
            committee_notes: "Needs refactor"
            ---
            # Auth
        """))
        flags = parse_room_frontmatter_flags(room)
        assert flags["health"] == "warning"
        assert flags["security"] == "sensitive"
        assert "refactor" in flags["note"]

    def test_missing_frontmatter_returns_empty(self, tmp_path):
        room = tmp_path / "plain.md"
        room.write_text("# No frontmatter here")
        assert parse_room_frontmatter_flags(room) == {}

    def test_normal_flags_returns_normal(self, tmp_path):
        room = tmp_path / "normal.md"
        room.write_text(textwrap.dedent("""\
            ---
            room: Normal
            architectural_health: normal
            security_tier: normal
            ---
        """))
        flags = parse_room_frontmatter_flags(room)
        assert flags.get("health") == "normal"


class TestAggregateGovernance:
    def _make_project(self, tmp_path):
        index_dir = tmp_path / "docs" / "index"
        index_dir.mkdir(parents=True)
        root = index_dir / "_root.md"
        root.write_text(SAMPLE_ROOT)
        return tmp_path

    def test_aggregates_from_root_files(self, tmp_path):
        project = self._make_project(tmp_path)
        entries = aggregate_governance([project])
        assert len(entries) == 2

    def test_sorted_by_severity_descending(self, tmp_path):
        project = self._make_project(tmp_path)
        entries = aggregate_governance([project])
        # sensitive > normal, so auth/jwt.md should come first
        assert entries[0]["security"] == "sensitive"

    def test_empty_project_returns_empty(self, tmp_path):
        entries = aggregate_governance([tmp_path])
        assert entries == []

    def test_deduplication_no_false_positive(self, tmp_path):
        """auth.md and auth_new.md must not deduplicate each other."""
        index_dir = tmp_path / "docs" / "index"
        index_dir.mkdir(parents=True)
        root = index_dir / "_root.md"
        root.write_text(textwrap.dedent("""\
            ## GOVERNANCE WATCHLIST

            | Room | Health | Security | Note |
            |------|--------|----------|------|
            | auth.md | normal | sensitive | note |
        """))
        # Create auth_new.md with non-normal flags (not in watchlist)
        auth_new = index_dir / "auth_new.md"
        auth_new.write_text(textwrap.dedent("""\
            ---
            room: Auth New
            architectural_health: warning
            security_tier: normal
            ---
            # Auth New
        """))
        entries = aggregate_governance([tmp_path])
        rooms = [e["room"] for e in entries]
        # auth_new.md must NOT be suppressed by auth.md
        assert any("auth_new" in r for r in rooms)

    def test_deduplication_suppresses_covered_room(self, tmp_path):
        """A room already in the GOVERNANCE WATCHLIST must not appear twice."""
        index_dir = tmp_path / "docs" / "index"
        auth_dir = index_dir / "auth"
        auth_dir.mkdir(parents=True)

        # Watchlist entry uses a relative path rooted at docs/index/
        root = index_dir / "_root.md"
        root.write_text(textwrap.dedent("""\
            ## GOVERNANCE WATCHLIST

            | Room | Health | Security | Note |
            |------|--------|----------|------|
            | auth/ucan.md | warning | normal | Already in watchlist |
        """))

        # Same room also has frontmatter flags
        room = auth_dir / "ucan.md"
        room.write_text(textwrap.dedent("""\
            ---
            room: UCAN Auth
            see_also: []
            architectural_health: warning
            security_tier: normal
            ---
            # UCAN Auth
        """))

        entries = aggregate_governance([tmp_path])
        # Only one entry for this room — the watchlist one, not a duplicate from frontmatter
        room_entries = [e for e in entries if "ucan" in e.get("room", "")]
        assert len(room_entries) == 1
