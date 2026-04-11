"""Tests for validate_patterns.py — semantic PATTERN validation."""
import textwrap
from pathlib import Path
import pytest
from validate_patterns import normalize, validate_patterns, extract_pattern_rows


def _make_project(tmp_path, pattern_label, room_content, pattern_aliases=None):
    """Build a minimal LOI project with one PATTERN entry pointing to auth/ucan.md."""
    index_dir = tmp_path / "docs" / "index"
    auth_dir = index_dir / "auth"
    auth_dir.mkdir(parents=True)

    alias_block = ""
    if pattern_aliases:
        items = "\n".join(f"  - {a}" for a in pattern_aliases)
        alias_block = f"pattern_aliases:\n{items}\n"

    root_md = index_dir / "_root.md"
    root_md.write_text(textwrap.dedent(f"""\
        ---
        room: Campus
        see_also: []
        architectural_health: normal
        security_tier: normal
        ---
        # LOI Index

        ## TASK → LOAD

        | Task | Load |
        |------|------|
        | Dummy task | auth/_root.md |

        ## PATTERN → LOAD

        | Pattern | Load |
        |---------|------|
        | {pattern_label} | auth/ucan.md |

        ## Buildings

        | Subdomain | Description | Rooms |
        |-----------|-------------|-------|
        | auth/ | Auth | ucan.md |
    """))

    auth_root = auth_dir / "_root.md"
    auth_root.write_text(textwrap.dedent("""\
        ---
        room: Auth
        see_also: []
        architectural_health: normal
        security_tier: normal
        ---
        # Auth

        ## TASK → LOAD

        | Task | Load |
        |------|------|
        | Dummy | ucan.md |
    """))

    room = auth_dir / "ucan.md"
    # Build frontmatter without textwrap.dedent so the leading "---" is never indented
    frontmatter = (
        "---\n"
        "room: UCAN Auth\n"
        "see_also: []\n"
        "architectural_health: normal\n"
        "security_tier: normal\n"
        f"{alias_block}"
        "---\n"
    )
    room.write_text(frontmatter + "\n" + room_content)

    return tmp_path


class TestNormalize:
    def test_lowercases(self):
        assert normalize("TOKEN ROTATION") == "token rotation"

    def test_strips_punctuation(self):
        assert normalize("exponential-backoff!") == "exponential backoff"

    def test_collapses_whitespace(self):
        assert normalize("  token   rotation  ") == "token rotation"

    def test_full_label(self):
        result = normalize("Token rotation without service restart")
        assert result == "token rotation without service restart"


class TestExtractPatternRows:
    def test_extracts_rows(self, tmp_path):
        root = tmp_path / "_root.md"
        root.write_text(textwrap.dedent("""\
            ## PATTERN → LOAD

            | Pattern | Load |
            |---------|------|
            | Exponential backoff | infra/retry.md |
            | Circuit breaker | infra/circuit.md |
        """))
        rows = extract_pattern_rows(root)
        assert len(rows) == 2
        assert rows[0]["pattern"] == "Exponential backoff"
        assert rows[0]["target_path"] == "infra/retry.md"

    def test_skips_header_row(self, tmp_path):
        root = tmp_path / "_root.md"
        root.write_text(textwrap.dedent("""\
            ## PATTERN → LOAD

            | Pattern | Load |
            |---------|------|
            | Retry | infra/retry.md |
        """))
        rows = extract_pattern_rows(root)
        assert not any(r["pattern"].lower() == "pattern" for r in rows)


class TestValidatePatterns:
    def test_exact_match_passes(self, tmp_path):
        content = "Token rotation without service restart is the primary pattern."
        project = _make_project(tmp_path, "Token rotation without service restart", content)
        result = validate_patterns(project, level=1)
        assert result.ok
        assert result.orphans == []

    def test_orphan_when_label_absent(self, tmp_path):
        content = "This room does not mention the pattern at all."
        project = _make_project(tmp_path, "Token rotation without service restart", content)
        result = validate_patterns(project, level=1)
        assert len(result.orphans) == 1
        assert "Token rotation without service restart" in result.orphans[0]

    def test_missing_target_is_error(self, tmp_path):
        index_dir = tmp_path / "docs" / "index"
        index_dir.mkdir(parents=True)

        root_md = index_dir / "_root.md"
        root_md.write_text(textwrap.dedent("""\
            ---
            room: Campus
            see_also: []
            architectural_health: normal
            security_tier: normal
            ---
            # LOI Index

            ## PATTERN → LOAD

            | Pattern | Load |
            |---------|------|
            | Some pattern | nonexistent/room.md |
        """))
        result = validate_patterns(tmp_path, level=1)
        assert not result.ok
        assert len(result.errors) == 1
        assert "missing target" in result.errors[0]

    def test_alias_match_level2_gives_warning_not_orphan(self, tmp_path):
        content = "Hot rotation of credentials is used here."
        project = _make_project(
            tmp_path,
            "Token rotation without service restart",
            content,
            pattern_aliases=["hot rotation"],
        )
        result = validate_patterns(project, level=2)
        assert result.ok
        assert result.orphans == []
        assert len(result.warnings) >= 1  # alias-only warning

    def test_alias_ignored_at_level1(self, tmp_path):
        """Level 1 does not check aliases — label must appear verbatim."""
        content = "Hot rotation of credentials is used here."
        project = _make_project(
            tmp_path,
            "Token rotation without service restart",
            content,
            pattern_aliases=["hot rotation"],
        )
        result = validate_patterns(project, level=1)
        assert len(result.orphans) == 1

    def test_no_pattern_section_produces_no_results(self, tmp_path):
        index_dir = tmp_path / "docs" / "index"
        index_dir.mkdir(parents=True)
        (index_dir / "_root.md").write_text(textwrap.dedent("""\
            ---
            room: Campus
            see_also: []
            architectural_health: normal
            security_tier: normal
            ---
            # LOI Index

            ## TASK → LOAD

            | Task | Load |
            |------|------|
            | Dummy | room.md |
        """))
        result = validate_patterns(tmp_path, level=1)
        assert result.ok
        assert result.orphans == []
        assert result.errors == []
