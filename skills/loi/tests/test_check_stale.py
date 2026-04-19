"""Tests for check_stale.py — pre-commit stale index detection."""
import textwrap
from pathlib import Path
import pytest
from check_stale import extract_source_paths, find_covering_rooms, main


def _make_index(tmp_path: Path, source_paths: str) -> Path:
    """Create a minimal docs/index/ with one room declaring Source paths."""
    index_dir = tmp_path / "docs" / "index"
    index_dir.mkdir(parents=True)
    root = index_dir / "_root.md"
    root.write_text(f"# LOI Index\n\nSource paths: {source_paths}\n")
    return tmp_path


class TestExtractSourcePaths:
    def test_single_path(self, tmp_path):
        f = tmp_path / "room.md"
        f.write_text("Source paths: skills/loi/\n")
        assert extract_source_paths(f) == ["skills/loi"]

    def test_multiple_paths(self, tmp_path):
        f = tmp_path / "room.md"
        f.write_text("Source paths: skills/loi/, .github/workflows/, README.md\n")
        assert extract_source_paths(f) == ["skills/loi", ".github/workflows", "README.md"]

    def test_strips_trailing_slash(self, tmp_path):
        f = tmp_path / "room.md"
        f.write_text("Source paths: src/\n")
        assert extract_source_paths(f) == ["src"]

    def test_no_source_paths(self, tmp_path):
        f = tmp_path / "room.md"
        f.write_text("# Just a heading\n")
        assert extract_source_paths(f) == []

    def test_missing_file(self, tmp_path):
        assert extract_source_paths(tmp_path / "nonexistent.md") == []


class TestFindCoveringRooms:
    def test_finds_covering_room(self, tmp_path):
        project = _make_index(tmp_path, "skills/loi/")
        rooms = find_covering_rooms(project, "skills/loi/scripts/watcher.py")
        assert len(rooms) == 1

    def test_no_coverage_for_unrelated_file(self, tmp_path):
        project = _make_index(tmp_path, "skills/loi/")
        rooms = find_covering_rooms(project, "some/other/file.py")
        assert rooms == []

    def test_exact_path_match(self, tmp_path):
        project = _make_index(tmp_path, "README.md")
        rooms = find_covering_rooms(project, "README.md")
        assert len(rooms) == 1

    def test_no_index_dir(self, tmp_path):
        rooms = find_covering_rooms(tmp_path, "src/main.py")
        assert rooms == []


class TestMain:
    def _init_git(self, tmp_path: Path) -> Path:
        """Create a minimal git repo with an LOI index."""
        import subprocess
        subprocess.run(["git", "init"], cwd=tmp_path, capture_output=True)
        subprocess.run(["git", "config", "user.email", "test@test.com"], cwd=tmp_path, capture_output=True)
        subprocess.run(["git", "config", "user.name", "Test"], cwd=tmp_path, capture_output=True)
        subprocess.run(["git", "config", "commit.gpgsign", "false"], cwd=tmp_path, capture_output=True)

        index_dir = tmp_path / "docs" / "index"
        index_dir.mkdir(parents=True)
        root = index_dir / "_root.md"
        root.write_text("# LOI Index\n\nSource paths: src/\n")
        subprocess.run(["git", "add", "."], cwd=tmp_path, capture_output=True)
        subprocess.run(["git", "commit", "-m", "init"], cwd=tmp_path, capture_output=True)
        return tmp_path

    def test_no_staged_files_exits_0(self, tmp_path):
        project = self._init_git(tmp_path)
        import sys
        sys.argv = ["check_stale.py", str(project)]
        assert main() == 0

    def test_staged_source_with_no_coverage_exits_0(self, tmp_path):
        import subprocess
        project = self._init_git(tmp_path)
        # Stage a file outside LOI Source paths
        other = project / "other.py"
        other.write_text("x = 1\n")
        subprocess.run(["git", "add", "other.py"], cwd=project, capture_output=True)
        import sys
        sys.argv = ["check_stale.py", str(project)]
        assert main() == 0

    def test_staged_source_covered_blocks_by_default(self, tmp_path, capsys):
        import subprocess
        project = self._init_git(tmp_path)
        src_dir = project / "src"
        src_dir.mkdir()
        (src_dir / "main.py").write_text("x = 1\n")
        subprocess.run(["git", "add", "src/main.py"], cwd=project, capture_output=True)
        import sys
        sys.argv = ["check_stale.py", str(project)]
        result = main()
        out = capsys.readouterr().out
        assert result == 1  # blocks by default
        assert "stale" in out.lower()

    def test_stale_block_0_warns_only(self, tmp_path, monkeypatch, capsys):
        import subprocess
        project = self._init_git(tmp_path)
        src_dir = project / "src"
        src_dir.mkdir()
        (src_dir / "main.py").write_text("x = 1\n")
        subprocess.run(["git", "add", "src/main.py"], cwd=project, capture_output=True)
        monkeypatch.setenv("LOI_STALE_BLOCK", "0")
        import sys
        sys.argv = ["check_stale.py", str(project)]
        result = main()
        assert result == 0  # warns but doesn't block
