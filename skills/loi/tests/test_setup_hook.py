"""Tests for setup_hook.py — hook installation and .gitignore management."""
import stat
from pathlib import Path
import pytest
from setup_hook import install_hook, _ensure_gitignore, find_hooks_dir


class TestEnsureGitignore:
    def test_adds_entries_when_absent(self, tmp_path):
        _ensure_gitignore(tmp_path)
        text = (tmp_path / ".gitignore").read_text()
        assert ".loi-claims.json" in text
        assert ".loi-claims.json.lock" in text

    def test_skips_entries_already_present(self, tmp_path):
        gitignore = tmp_path / ".gitignore"
        gitignore.write_text(".loi-claims.json\n.loi-claims.json.lock\n")
        _ensure_gitignore(tmp_path)
        text = gitignore.read_text()
        # Must not duplicate entries
        assert text.count(".loi-claims.json\n") == 1
        assert text.count(".loi-claims.json.lock\n") == 1

    def test_adds_only_missing_entry(self, tmp_path):
        gitignore = tmp_path / ".gitignore"
        gitignore.write_text(".loi-claims.json\n")
        _ensure_gitignore(tmp_path)
        text = gitignore.read_text()
        assert ".loi-claims.json.lock" in text
        assert text.count(".loi-claims.json\n") == 1  # original not duplicated

    def test_creates_gitignore_if_missing(self, tmp_path):
        assert not (tmp_path / ".gitignore").exists()
        _ensure_gitignore(tmp_path)
        assert (tmp_path / ".gitignore").exists()

    def test_appends_with_newline_separator(self, tmp_path):
        gitignore = tmp_path / ".gitignore"
        # Existing file without trailing newline
        gitignore.write_text("*.pyc")
        _ensure_gitignore(tmp_path)
        text = gitignore.read_text()
        # Existing content preserved and new entries on separate lines
        assert "*.pyc" in text
        assert ".loi-claims.json" in text
        # No joined line like "*.pyc.loi-claims.json"
        assert "*.pyc.loi" not in text

    def test_idempotent(self, tmp_path):
        _ensure_gitignore(tmp_path)
        first = (tmp_path / ".gitignore").read_text()
        _ensure_gitignore(tmp_path)
        second = (tmp_path / ".gitignore").read_text()
        assert first == second


class TestInstallHook:
    def _make_git_repo(self, tmp_path):
        """Create a minimal .git/hooks directory."""
        (tmp_path / ".git" / "hooks").mkdir(parents=True)
        return tmp_path

    def test_installs_pre_push_hook(self, tmp_path):
        project = self._make_git_repo(tmp_path)
        result = install_hook(project, "pre-push", force=False)
        assert result == 0
        dest = project / ".git" / "hooks" / "pre-push"
        assert dest.is_file()

    def test_hook_is_executable(self, tmp_path):
        project = self._make_git_repo(tmp_path)
        install_hook(project, "pre-push", force=False)
        dest = project / ".git" / "hooks" / "pre-push"
        mode = dest.stat().st_mode
        assert mode & stat.S_IEXEC

    def test_no_overwrite_without_force(self, tmp_path):
        project = self._make_git_repo(tmp_path)
        install_hook(project, "pre-push", force=False)
        # Write something identifiable to the hook
        dest = project / ".git" / "hooks" / "pre-push"
        dest.write_text("# sentinel\n")
        result = install_hook(project, "pre-push", force=False)
        assert result == 1
        assert dest.read_text() == "# sentinel\n"

    def test_force_overwrites_existing(self, tmp_path):
        project = self._make_git_repo(tmp_path)
        dest = project / ".git" / "hooks" / "pre-push"
        dest.write_text("# old hook\n")
        result = install_hook(project, "pre-push", force=True)
        assert result == 0
        assert dest.read_text() != "# old hook\n"

    def test_fails_without_git_dir(self, tmp_path):
        result = install_hook(tmp_path, "pre-push", force=False)
        assert result == 2

    def test_installs_gitignore_entries(self, tmp_path):
        project = self._make_git_repo(tmp_path)
        install_hook(project, "pre-push", force=False)
        gitignore = project / ".gitignore"
        assert gitignore.exists()
        text = gitignore.read_text()
        assert ".loi-claims.json" in text

    def test_unsupported_mode_returns_error(self, tmp_path):
        project = self._make_git_repo(tmp_path)
        result = install_hook(project, "post-commit", force=False)
        assert result == 2
