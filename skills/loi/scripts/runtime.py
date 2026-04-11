#!/usr/bin/env python3
"""LOI Runtime Coordination — room claims, heartbeats, releases, and summaries.

This module provides advisory-first coordination so multiple agents can avoid
unknowingly editing the same room at the same time.

Claims are stored in a local file backend by default (`.loi-claims.json` in the
project root).  A webhook backend can forward all events to a peer service.

Usage:
    python3 runtime.py claim auth/ucan.md --intent edit --ttl 15m
    python3 runtime.py heartbeat auth/ucan.md
    python3 runtime.py release auth/ucan.md
    python3 runtime.py status auth/ucan.md
    python3 runtime.py summary auth/ucan.md "Working on TTL path in MintToken"
    python3 runtime.py claims
    python3 runtime.py claims --repo my-repo
"""

import argparse
import fcntl
import json
import os
import socket
import subprocess
import sys
from contextlib import contextmanager
from datetime import datetime, timedelta, timezone
from pathlib import Path

# ---------------------------------------------------------------------------
# Claim model
# ---------------------------------------------------------------------------

DEFAULT_TTL_SECONDS = 900  # 15 minutes
HEARTBEAT_GRACE = 300       # additional 5 min after a heartbeat
CLAIMS_FILE = ".loi-claims.json"

INTENT_CONFLICT_MATRIX = {
    # (existing, incoming): action
    ("read", "read"): "allow",
    ("read", "edit"): "allow_with_visibility",
    ("edit", "read"): "allow_with_visibility",
    ("edit", "edit"): "conflict",
    ("review", "edit"): "allow_with_warning",
    ("security-sweep", "edit"): "governance_sensitive",
}


def _now() -> datetime:
    return datetime.now(timezone.utc)


def _iso(dt: datetime) -> str:
    return dt.isoformat()


def _parse_ttl(ttl_str: str) -> int:
    """Parse a TTL string like '15m', '2h', '30s' into seconds."""
    m = {"s": 1, "m": 60, "h": 3600, "d": 86400}
    if ttl_str[-1] in m:
        return int(ttl_str[:-1]) * m[ttl_str[-1]]
    return int(ttl_str)


def _agent_id() -> str:
    """Derive a stable agent ID from hostname + user."""
    return f"{os.environ.get('USER', 'agent')}@{socket.gethostname()}"


def _session_id(agent_id: str) -> str:
    return f"{agent_id}-{_now().strftime('%Y-%m-%dT%H:%M:%SZ')}"


def _get_repo(project_root: Path) -> str:
    result = subprocess.run(
        ["git", "remote", "get-url", "origin"],
        capture_output=True, text=True, cwd=project_root,
    )
    if result.returncode == 0:
        url = result.stdout.strip()
        name = url.rstrip("/").rsplit("/", 1)[-1].removesuffix(".git")
        return name
    return project_root.name


def _get_branch(project_root: Path) -> str:
    result = subprocess.run(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"],
        capture_output=True, text=True, cwd=project_root,
    )
    return result.stdout.strip() if result.returncode == 0 else "unknown"


# ---------------------------------------------------------------------------
# Claims store (file backend)
# ---------------------------------------------------------------------------

class ClaimsStore:
    """JSON file-backed claims store at project_root/.loi-claims.json.

    All mutating operations hold an exclusive flock on a companion lock file
    (.loi-claims.json.lock) for the duration of the read-modify-write cycle,
    preventing lost updates when multiple agents run concurrently.
    """

    def __init__(self, project_root: Path) -> None:
        self.path = project_root / CLAIMS_FILE
        self._lock_path = project_root / (CLAIMS_FILE + ".lock")

    @contextmanager
    def _exclusive(self):
        """Hold an exclusive flock for the duration of a read-modify-write."""
        with open(self._lock_path, "w") as lf:
            fcntl.flock(lf, fcntl.LOCK_EX)
            try:
                yield
            finally:
                fcntl.flock(lf, fcntl.LOCK_UN)

    @contextmanager
    def _shared(self):
        """Hold a shared flock for read-only operations."""
        with open(self._lock_path, "a") as lf:
            fcntl.flock(lf, fcntl.LOCK_SH)
            try:
                yield
            finally:
                fcntl.flock(lf, fcntl.LOCK_UN)

    def _load(self) -> dict:
        if not self.path.is_file():
            return {"claims": [], "summaries": []}
        try:
            return json.loads(self.path.read_text(encoding="utf-8"))
        except Exception:
            return {"claims": [], "summaries": []}

    def _save(self, data: dict) -> None:
        self.path.write_text(json.dumps(data, indent=2), encoding="utf-8")

    def _prune_expired(self, claims: list[dict]) -> list[dict]:
        now = _now()
        live = []
        for c in claims:
            try:
                exp = datetime.fromisoformat(c["expires_at"])
                if exp > now:
                    live.append(c)
            except (KeyError, ValueError):
                live.append(c)
        return live

    def all_claims(self) -> list[dict]:
        with self._exclusive():
            data = self._load()
            pruned = self._prune_expired(data.get("claims", []))
            if len(pruned) != len(data.get("claims", [])):
                data["claims"] = pruned
                self._save(data)
            return pruned

    def get_claims_for(self, scope_id: str) -> list[dict]:
        return [c for c in self.all_claims() if c["scope_id"] == scope_id]

    def add_claim(self, claim: dict) -> None:
        with self._exclusive():
            data = self._load()
            claims = self._prune_expired(data.get("claims", []))
            # Remove existing claim from same agent+scope
            claims = [
                c for c in claims
                if not (c["scope_id"] == claim["scope_id"] and c["agent_id"] == claim["agent_id"])
            ]
            claims.append(claim)
            data["claims"] = claims
            self._save(data)

    def remove_claim(self, scope_id: str, agent_id: str) -> bool:
        with self._exclusive():
            data = self._load()
            before = len(data.get("claims", []))
            data["claims"] = [
                c for c in data.get("claims", [])
                if not (c["scope_id"] == scope_id and c["agent_id"] == agent_id)
            ]
            removed = len(data["claims"]) < before
            self._save(data)
            return removed

    def update_expiry(self, scope_id: str, agent_id: str, extra_seconds: int) -> bool:
        with self._exclusive():
            data = self._load()
            updated = False
            for c in data.get("claims", []):
                if c["scope_id"] == scope_id and c["agent_id"] == agent_id:
                    try:
                        exp = datetime.fromisoformat(c["expires_at"])
                    except (KeyError, ValueError):
                        exp = _now()
                    c["expires_at"] = _iso(max(exp, _now()) + timedelta(seconds=extra_seconds))
                    c["last_heartbeat"] = _iso(_now())
                    updated = True
            if updated:
                self._save(data)
            return updated

    def add_summary(self, scope_id: str, agent_id: str, summary: str) -> None:
        with self._exclusive():
            data = self._load()
            summaries = data.get("summaries", [])
            summaries.append({
                "scope_id": scope_id,
                "agent_id": agent_id,
                "summary": summary,
                "recorded_at": _iso(_now()),
            })
            # Keep only last 100 summaries
            data["summaries"] = summaries[-100:]
            self._save(data)

    def get_summaries_for(self, scope_id: str) -> list[dict]:
        with self._shared():
            data = self._load()
            return [s for s in data.get("summaries", []) if s["scope_id"] == scope_id]


# ---------------------------------------------------------------------------
# Conflict check
# ---------------------------------------------------------------------------

def check_conflict(existing_claims: list[dict], incoming_intent: str) -> tuple[str, str]:
    """Return (action, message) for the incoming claim against existing ones.

    Actions: allow | allow_with_visibility | allow_with_warning | conflict | governance_sensitive
    """
    if not existing_claims:
        return "allow", ""

    for c in existing_claims:
        existing_intent = c.get("intent", "read")
        action = INTENT_CONFLICT_MATRIX.get((existing_intent, incoming_intent), "allow")
        agent = c.get("agent_id", "unknown")
        branch = c.get("branch", "")
        exp = c.get("expires_at", "")

        if action == "conflict":
            return "conflict", (
                f"CONFLICT: '{agent}' already holds an edit claim on this room "
                f"(branch: {branch}, expires: {exp}). "
                f"Use `/loi status <room>` to see the active claim."
            )
        if action == "governance_sensitive":
            return "governance_sensitive", (
                f"CAUTION: '{agent}' is running a security-sweep on this room. "
                f"Edit claims on security-sensitive rooms require explicit override."
            )
        if action == "allow_with_warning":
            return "allow_with_warning", (
                f"WARNING: '{agent}' has a review claim. Edits may conflict."
            )
        if action == "allow_with_visibility":
            return "allow_with_visibility", (
                f"NOTE: '{agent}' has a {existing_intent} claim on this room."
            )

    return "allow", ""


# ---------------------------------------------------------------------------
# Commands
# ---------------------------------------------------------------------------

def cmd_claim(args, project_root: Path, store: ClaimsStore) -> int:
    scope_id = args.room
    ttl_sec = _parse_ttl(args.ttl)
    agent_id = _agent_id()
    now = _now()

    existing = store.get_claims_for(scope_id)
    action, msg = check_conflict(existing, args.intent)

    if action == "conflict":
        print(f"[LOI runtime] {msg}")
        return 1
    if action in ("allow_with_warning", "allow_with_visibility", "governance_sensitive"):
        print(f"[LOI runtime] {msg}")

    claim = {
        "scope_type": "room",
        "scope_id": scope_id,
        "repo": _get_repo(project_root),
        "agent_id": agent_id,
        "session_id": _session_id(agent_id),
        "intent": args.intent,
        "claimed_at": _iso(now),
        "expires_at": _iso(now + timedelta(seconds=ttl_sec)),
        "branch": _get_branch(project_root),
    }

    store.add_claim(claim)
    print(f"[LOI runtime] Claimed '{scope_id}' with intent={args.intent}, ttl={args.ttl}")
    print(f"  Agent:   {agent_id}")
    print(f"  Branch:  {claim['branch']}")
    print(f"  Expires: {claim['expires_at']}")
    return 0


def cmd_heartbeat(args, project_root: Path, store: ClaimsStore) -> int:
    agent_id = _agent_id()
    updated = store.update_expiry(args.room, agent_id, HEARTBEAT_GRACE)
    if updated:
        print(f"[LOI runtime] Heartbeat sent for '{args.room}' — extended by {HEARTBEAT_GRACE}s")
    else:
        print(f"[LOI runtime] No active claim found for '{args.room}' by {agent_id}")
        return 1
    return 0


def cmd_release(args, project_root: Path, store: ClaimsStore) -> int:
    agent_id = _agent_id()
    removed = store.remove_claim(args.room, agent_id)
    if removed:
        print(f"[LOI runtime] Released claim on '{args.room}'")
    else:
        print(f"[LOI runtime] No claim to release for '{args.room}' by {agent_id}")
    return 0


def cmd_status(args, project_root: Path, store: ClaimsStore) -> int:
    claims = store.get_claims_for(args.room)
    summaries = store.get_summaries_for(args.room)

    if not claims:
        print(f"[LOI runtime] '{args.room}' — no active claims")
    else:
        print(f"[LOI runtime] '{args.room}' — {len(claims)} active claim(s):")
        for c in claims:
            print(f"  agent={c.get('agent_id')}  intent={c.get('intent')}  "
                  f"branch={c.get('branch')}  expires={c.get('expires_at')}")

    if args.include_freshness and summaries:
        print(f"\n  Recent summaries:")
        for s in summaries[-5:]:
            print(f"  [{s['recorded_at']}] {s['agent_id']}: {s['summary']}")
    return 0


def cmd_summary(args, project_root: Path, store: ClaimsStore) -> int:
    agent_id = _agent_id()
    store.add_summary(args.room, agent_id, args.text)
    print(f"[LOI runtime] Summary recorded for '{args.room}'")
    return 0


def cmd_claims(args, project_root: Path, store: ClaimsStore) -> int:
    all_claims = store.all_claims()
    if args.repo:
        all_claims = [c for c in all_claims if c.get("repo") == args.repo]

    if not all_claims:
        print("[LOI runtime] No active claims.")
        return 0

    print(f"[LOI runtime] {len(all_claims)} active claim(s):")
    for c in all_claims:
        print(
            f"  {c.get('scope_id', '?'):40s}  "
            f"intent={c.get('intent', '?'):12s}  "
            f"agent={c.get('agent_id', '?'):30s}  "
            f"expires={c.get('expires_at', '?')}"
        )
    return 0


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def find_project_root(start: Path) -> Path:
    p = start.resolve()
    while p != p.parent:
        if (p / ".git").exists():
            return p
        p = p.parent
    return start


def main():
    parser = argparse.ArgumentParser(
        description="LOI Runtime Coordination — room claims and summaries",
    )
    parser.add_argument(
        "--project-root",
        help="Project root (default: nearest git root from cwd)",
    )
    sub = parser.add_subparsers(dest="command")

    # claim
    p_claim = sub.add_parser("claim", help="Claim a room")
    p_claim.add_argument("room", help="Room path, e.g. auth/ucan.md")
    p_claim.add_argument("--intent", default="read",
                         choices=["read", "edit", "review", "security-sweep"],
                         help="Claim intent (default: read)")
    p_claim.add_argument("--ttl", default="15m",
                         help="Claim TTL, e.g. 15m, 2h, 30s (default: 15m)")

    # heartbeat
    p_hb = sub.add_parser("heartbeat", help="Extend a claim's TTL")
    p_hb.add_argument("room")

    # release
    p_rel = sub.add_parser("release", help="Release a claim")
    p_rel.add_argument("room")

    # status
    p_stat = sub.add_parser("status", help="Show claims for a room")
    p_stat.add_argument("room")
    p_stat.add_argument("--include-freshness", action="store_true",
                        help="Also show recent summaries")

    # summary
    p_sum = sub.add_parser("summary", help="Record a handoff summary for a room")
    p_sum.add_argument("room")
    p_sum.add_argument("text", help="Summary text")

    # claims (list all)
    p_cls = sub.add_parser("claims", help="List all active claims")
    p_cls.add_argument("--repo", help="Filter by repo name")

    args = parser.parse_args()

    if args.command is None:
        parser.print_help()
        return 0

    project_root = (
        Path(args.project_root).resolve()
        if args.project_root
        else find_project_root(Path.cwd())
    )

    store = ClaimsStore(project_root)

    dispatch = {
        "claim": cmd_claim,
        "heartbeat": cmd_heartbeat,
        "release": cmd_release,
        "status": cmd_status,
        "summary": cmd_summary,
        "claims": cmd_claims,
    }

    fn = dispatch.get(args.command)
    if fn:
        sys.exit(fn(args, project_root, store))
    else:
        parser.print_help()
        sys.exit(2)


if __name__ == "__main__":
    main()
