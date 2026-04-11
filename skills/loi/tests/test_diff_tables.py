"""Tests for diff_tables.py — TASK/PATTERN/GOVERNANCE row-level diff."""
import pytest
from diff_tables import parse_tables, diff_tables, format_diff, _rows_to_dict


SAMPLE_MD = """\
# LOI Index

## TASK → LOAD

| Task | Load |
|------|------|
| Authenticate user | auth/jwt.md |
| Refresh token | auth/refresh.md |

## PATTERN → LOAD

| Pattern | Load |
|---------|------|
| Exponential backoff | infra/retry.md |

## GOVERNANCE WATCHLIST

| Room | Health | Security | Note |
|------|--------|----------|------|
| auth/jwt.md | normal | sensitive | JWT signing |
"""


class TestParseTables:
    def test_extracts_task_rows(self):
        tables = parse_tables(SAMPLE_MD)
        assert len(tables["TASK"]) == 2
        assert tables["TASK"][0] == ("Authenticate user", "auth/jwt.md")
        assert tables["TASK"][1] == ("Refresh token", "auth/refresh.md")

    def test_extracts_pattern_rows(self):
        tables = parse_tables(SAMPLE_MD)
        assert len(tables["PATTERN"]) == 1
        assert tables["PATTERN"][0][0] == "Exponential backoff"

    def test_extracts_governance_rows(self):
        tables = parse_tables(SAMPLE_MD)
        assert len(tables["GOVERNANCE"]) == 1
        assert tables["GOVERNANCE"][0][0] == "auth/jwt.md"
        assert tables["GOVERNANCE"][0][2] == "sensitive"

    def test_empty_text_returns_empty_lists(self):
        tables = parse_tables("")
        assert tables["TASK"] == []
        assert tables["PATTERN"] == []
        assert tables["GOVERNANCE"] == []

    def test_skips_separator_and_header_rows(self):
        tables = parse_tables(SAMPLE_MD)
        # Header row ("Task", "Load") and separator row must not appear in results
        for row in tables["TASK"]:
            assert row[0].lower() not in ("task", "---", "---")
            assert not all(c.startswith("-") for c in row)


class TestDiffTables:
    def test_no_changes_returns_empty_diffs(self):
        diff = diff_tables(SAMPLE_MD, SAMPLE_MD)
        for table in ("TASK", "PATTERN", "GOVERNANCE"):
            assert diff[table]["added"] == []
            assert diff[table]["removed"] == []
            assert diff[table]["changed"] == []

    def test_added_row_detected(self):
        new_md = SAMPLE_MD.replace(
            "| Refresh token | auth/refresh.md |",
            "| Refresh token | auth/refresh.md |\n| Issue UCAN token | auth/ucan.md |",
        )
        diff = diff_tables(SAMPLE_MD, new_md)
        assert len(diff["TASK"]["added"]) == 1
        assert diff["TASK"]["added"][0][0] == "Issue UCAN token"

    def test_removed_row_detected(self):
        new_md = SAMPLE_MD.replace("| Refresh token | auth/refresh.md |\n", "")
        diff = diff_tables(SAMPLE_MD, new_md)
        assert len(diff["TASK"]["removed"]) == 1
        assert diff["TASK"]["removed"][0][0] == "Refresh token"

    def test_changed_row_detected(self):
        new_md = SAMPLE_MD.replace("auth/refresh.md", "auth/refresh_v2.md")
        diff = diff_tables(SAMPLE_MD, new_md)
        assert len(diff["TASK"]["changed"]) == 1
        old_row, new_row = diff["TASK"]["changed"][0]
        assert old_row[1] == "auth/refresh.md"
        assert new_row[1] == "auth/refresh_v2.md"

    def test_no_cross_table_contamination(self):
        # Change a GOVERNANCE row; TASK and PATTERN should be clean
        new_md = SAMPLE_MD.replace("normal | sensitive", "warning | sensitive")
        diff = diff_tables(SAMPLE_MD, new_md)
        assert diff["TASK"]["added"] == []
        assert diff["TASK"]["changed"] == []
        assert len(diff["GOVERNANCE"]["changed"]) == 1

    def test_empty_old_text(self):
        diff = diff_tables("", SAMPLE_MD)
        assert len(diff["TASK"]["added"]) == 2
        assert diff["TASK"]["removed"] == []

    def test_empty_new_text(self):
        diff = diff_tables(SAMPLE_MD, "")
        assert len(diff["TASK"]["removed"]) == 2
        assert diff["TASK"]["added"] == []


class TestFormatDiff:
    def test_no_changes_returns_empty_string(self):
        diff = diff_tables(SAMPLE_MD, SAMPLE_MD)
        assert format_diff(diff) == ""

    def test_added_row_appears_in_output(self):
        new_md = SAMPLE_MD.replace(
            "| Refresh token | auth/refresh.md |",
            "| Refresh token | auth/refresh.md |\n| Issue UCAN token | auth/ucan.md |",
        )
        diff = diff_tables(SAMPLE_MD, new_md)
        result = format_diff(diff)
        assert "TASK changes" in result
        assert "added" in result
        assert "Issue UCAN token" in result

    def test_removed_row_appears_in_output(self):
        new_md = SAMPLE_MD.replace("| Refresh token | auth/refresh.md |\n", "")
        diff = diff_tables(SAMPLE_MD, new_md)
        result = format_diff(diff)
        assert "removed" in result
        assert "Refresh token" in result

    def test_unchanged_tables_not_in_output(self):
        new_md = SAMPLE_MD.replace(
            "| Refresh token | auth/refresh.md |",
            "| Refresh token | auth/refresh.md |\n| New task | new.md |",
        )
        diff = diff_tables(SAMPLE_MD, new_md)
        result = format_diff(diff)
        assert "PATTERN changes" not in result
        assert "GOVERNANCE changes" not in result


class TestRowsToDict:
    def test_duplicate_key_triggers_warning(self, capsys):
        rows = [("same-key", "val1"), ("same-key", "val2")]
        result = _rows_to_dict(rows, "TASK")
        out = capsys.readouterr().out
        assert "duplicate key" in out
        assert result["same-key"] == ("same-key", "val2")  # last writer wins

    def test_unique_keys_no_warning(self, capsys):
        rows = [("key1", "val1"), ("key2", "val2")]
        _rows_to_dict(rows, "TASK")
        out = capsys.readouterr().out
        assert out == ""
