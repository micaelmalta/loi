"""Slack notify backend — POSTs LOI events as formatted Slack messages.

Requires a Slack incoming webhook URL:
    https://api.slack.com/messaging/webhooks

Config:
    notify_url  Slack incoming webhook URL (required)
"""

from __future__ import annotations

import json
import urllib.request
from datetime import datetime, timezone


class SlackBackend:
    """Send LOI events as formatted Slack messages via incoming webhook."""

    def __init__(self, webhook_url: str) -> None:
        if not webhook_url:
            raise ValueError("SlackBackend requires a non-empty notify_url (Slack webhook URL)")
        self.webhook_url = webhook_url

    def send(self, event_type: str, payload: dict) -> None:
        ts = datetime.now(timezone.utc).isoformat()
        repo = payload.get("repo", "unknown")
        path = payload.get("path", "")
        summary = payload.get("summary", "")
        governance = payload.get("governance", {})

        # Header line
        header_text = f"LOI: {event_type}"

        # Build blocks
        fields = [
            {"type": "mrkdwn", "text": f"*Repo:*\n{repo}"},
            {"type": "mrkdwn", "text": f"*Event:*\n{event_type}"},
        ]
        if path:
            fields.append({"type": "mrkdwn", "text": f"*Path:*\n`{path}`"})
        if governance:
            health = governance.get("health", "normal")
            security = governance.get("security", "normal")
            fields.append({"type": "mrkdwn", "text": f"*Governance:*\nhealth={health}, security={security}"})

        blocks = [
            {"type": "header", "text": {"type": "plain_text", "text": header_text}},
            {"type": "section", "fields": fields},
        ]
        if summary:
            blocks.append({
                "type": "section",
                "text": {"type": "mrkdwn", "text": summary},
            })

        # Attach table diff if present
        table_diff = payload.get("table_diff")
        if table_diff:
            MAX_DIFF = 2000
            truncated = table_diff[:MAX_DIFF]
            if len(table_diff) > MAX_DIFF:
                truncated += f"\n... (+{len(table_diff) - MAX_DIFF} chars truncated)"
            blocks.append({
                "type": "section",
                "text": {"type": "mrkdwn", "text": f"```{truncated}```"},
            })

        pr_url = payload.get("pr_url")
        if pr_url:
            blocks.append({
                "type": "actions",
                "elements": [{
                    "type": "button",
                    "text": {"type": "plain_text", "text": "Review PR"},
                    "url": pr_url,
                    "style": "primary",
                }],
            })

        body = json.dumps({
            "text": f"LOI: {event_type} — {repo}",
            "blocks": blocks,
        }).encode("utf-8")

        req = urllib.request.Request(
            self.webhook_url,
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        with urllib.request.urlopen(req, timeout=10) as resp:
            if resp.status == 200:
                print(f"[LOI] Slack notification sent: {event_type}")
            else:
                print(f"[LOI] Slack webhook returned HTTP {resp.status}")
