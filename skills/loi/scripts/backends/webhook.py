"""Webhook notify backend — POSTs JSON events to an HTTP endpoint.

Config:
    notify_url      The full URL to POST to (required)
    auth_token_env  Name of env var holding a bearer token (optional)

Payload shape follows the LOI event schema:
    {
      "event": "proposal.updated",
      "repo": "my-repo",
      "path": "docs/index/auth/_root.md",
      "timestamp": "2026-04-10T12:00:00Z",
      "summary": "...",
      "governance": {"health": "normal", "security": "normal"}
    }
"""

from __future__ import annotations

import json
import os
import urllib.request
from datetime import datetime, timezone


class WebhookBackend:
    """POST LOI events as JSON to a generic HTTP webhook endpoint."""

    def __init__(self, url: str, *, token_env: str | None = None) -> None:
        if not url:
            raise ValueError("WebhookBackend requires a non-empty notify_url")
        self.url = url
        self.token_env = token_env

    def _headers(self) -> dict[str, str]:
        headers = {"Content-Type": "application/json"}
        if self.token_env:
            token = os.environ.get(self.token_env, "")
            if token:
                headers["Authorization"] = f"Bearer {token}"
        return headers

    def send(self, event_type: str, payload: dict) -> None:
        event = {
            "event": event_type,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            **payload,
        }
        data = json.dumps(event).encode("utf-8")
        req = urllib.request.Request(
            self.url,
            data=data,
            headers=self._headers(),
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            if resp.status == 200:
                print(f"[LOI] Webhook event sent: {event_type}")
            else:
                print(f"[LOI] Webhook returned HTTP {resp.status} for event {event_type}")
