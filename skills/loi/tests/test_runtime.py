"""Tests for runtime.py — room claims, conflict matrix, and file locking."""
import threading
from pathlib import Path
import pytest
from runtime import ClaimsStore, check_conflict, _parse_ttl


def _claim(scope_id, agent_id, intent="edit", expires="2099-01-01T00:00:00+00:00"):
    return {
        "scope_type": "room",
        "scope_id": scope_id,
        "repo": "test-repo",
        "agent_id": agent_id,
        "session_id": f"{agent_id}-session",
        "intent": intent,
        "claimed_at": "2026-04-10T00:00:00+00:00",
        "expires_at": expires,
        "branch": "main",
    }


class TestParseTTL:
    def test_minutes(self):
        assert _parse_ttl("15m") == 900

    def test_hours(self):
        assert _parse_ttl("2h") == 7200

    def test_seconds(self):
        assert _parse_ttl("30s") == 30

    def test_days(self):
        assert _parse_ttl("1d") == 86400

    def test_bare_integer(self):
        assert _parse_ttl("120") == 120


class TestCheckConflict:
    def test_no_existing_allows(self):
        action, _ = check_conflict([], "edit")
        assert action == "allow"

    def test_edit_edit_conflicts(self):
        existing = [_claim("room.md", "alice")]
        action, msg = check_conflict(existing, "edit")
        assert action == "conflict"
        assert "alice" in msg

    def test_read_read_allows(self):
        existing = [_claim("room.md", "alice", intent="read")]
        action, _ = check_conflict(existing, "read")
        assert action == "allow"

    def test_read_edit_allows_with_visibility(self):
        existing = [_claim("room.md", "alice", intent="read")]
        action, _ = check_conflict(existing, "edit")
        assert action == "allow_with_visibility"

    def test_edit_read_allows_with_visibility(self):
        existing = [_claim("room.md", "alice", intent="edit")]
        action, _ = check_conflict(existing, "read")
        assert action == "allow_with_visibility"

    def test_review_edit_warns(self):
        existing = [_claim("room.md", "alice", intent="review")]
        action, _ = check_conflict(existing, "edit")
        assert action == "allow_with_warning"

    def test_security_sweep_edit_governance_sensitive(self):
        existing = [_claim("room.md", "alice", intent="security-sweep")]
        action, _ = check_conflict(existing, "edit")
        assert action == "governance_sensitive"


class TestClaimsStore:
    def test_add_and_retrieve(self, tmp_path):
        store = ClaimsStore(tmp_path)
        store.add_claim(_claim("auth/ucan.md", "agent-a"))
        claims = store.get_claims_for("auth/ucan.md")
        assert len(claims) == 1
        assert claims[0]["agent_id"] == "agent-a"

    def test_release_removes_claim(self, tmp_path):
        store = ClaimsStore(tmp_path)
        store.add_claim(_claim("auth/ucan.md", "agent-a"))
        removed = store.remove_claim("auth/ucan.md", "agent-a")
        assert removed
        assert store.get_claims_for("auth/ucan.md") == []

    def test_release_nonexistent_returns_false(self, tmp_path):
        store = ClaimsStore(tmp_path)
        assert not store.remove_claim("auth/ucan.md", "ghost")

    def test_duplicate_claim_replaces_own(self, tmp_path):
        """Re-claiming the same room with the same agent replaces the old claim."""
        store = ClaimsStore(tmp_path)
        store.add_claim(_claim("auth/ucan.md", "agent-a", intent="read"))
        store.add_claim(_claim("auth/ucan.md", "agent-a", intent="edit"))
        claims = store.get_claims_for("auth/ucan.md")
        assert len(claims) == 1
        assert claims[0]["intent"] == "edit"

    def test_expired_claims_pruned(self, tmp_path):
        store = ClaimsStore(tmp_path)
        expired = _claim("auth/old.md", "agent-a", expires="2000-01-01T00:00:00+00:00")
        store.add_claim(expired)
        assert store.get_claims_for("auth/old.md") == []

    def test_update_expiry(self, tmp_path):
        store = ClaimsStore(tmp_path)
        store.add_claim(_claim("auth/ucan.md", "agent-a"))
        updated = store.update_expiry("auth/ucan.md", "agent-a", 300)
        assert updated

    def test_add_and_retrieve_summary(self, tmp_path):
        store = ClaimsStore(tmp_path)
        store.add_summary("auth/ucan.md", "agent-a", "Working on TTL path")
        summaries = store.get_summaries_for("auth/ucan.md")
        assert len(summaries) == 1
        assert summaries[0]["summary"] == "Working on TTL path"

    def test_summaries_capped_at_100(self, tmp_path):
        store = ClaimsStore(tmp_path)
        for i in range(110):
            store.add_summary("room.md", "agent", f"summary {i}")
        summaries = store.get_summaries_for("room.md")
        assert len(summaries) == 100
        # Most recent summaries are kept
        assert summaries[-1]["summary"] == "summary 109"

    def test_all_claims_across_scopes(self, tmp_path):
        store = ClaimsStore(tmp_path)
        store.add_claim(_claim("room-a.md", "agent-1"))
        store.add_claim(_claim("room-b.md", "agent-2"))
        all_claims = store.all_claims()
        assert len(all_claims) == 2

    def test_no_lost_updates_concurrent(self, tmp_path):
        """Ten threads writing claims simultaneously — all must survive (locking test)."""
        store = ClaimsStore(tmp_path)
        errors = []

        def add(i):
            try:
                store.add_claim(_claim(f"room-{i}.md", f"agent-{i}"))
            except Exception as e:
                errors.append(e)

        threads = [threading.Thread(target=add, args=(i,)) for i in range(10)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert not errors, f"Unexpected errors: {errors}"
        assert len(store.all_claims()) == 10

    def test_no_lost_updates_same_room_concurrent(self, tmp_path):
        """Multiple agents racing to claim the same room — last writer wins, no corruption."""
        store = ClaimsStore(tmp_path)
        results = {}
        errors = []

        def claim_room(agent_id):
            try:
                store.add_claim(_claim("auth/ucan.md", agent_id))
                results[agent_id] = True
            except Exception as e:
                errors.append(e)

        threads = [threading.Thread(target=claim_room, args=(f"agent-{i}",)) for i in range(8)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert not errors
        claims = store.get_claims_for("auth/ucan.md")
        # Each agent_id is unique so all 8 claims should survive
        assert len(claims) == 8

    def test_summary_read_during_concurrent_write(self, tmp_path):
        """get_summaries_for must not return partial data while add_summary writes."""
        store = ClaimsStore(tmp_path)
        errors = []

        def writer():
            for i in range(20):
                try:
                    store.add_summary("room.md", "writer", f"summary {i}")
                except Exception as e:
                    errors.append(e)

        def reader():
            for _ in range(20):
                try:
                    store.get_summaries_for("room.md")
                except Exception as e:
                    errors.append(e)

        threads = [threading.Thread(target=writer), threading.Thread(target=reader)]
        for t in threads:
            t.start()
        for t in threads:
            t.join()

        assert not errors, f"Unexpected errors during concurrent read/write: {errors}"
