"""LOI notification backends.

All backends implement the NotifyBackend protocol:

    class NotifyBackend(Protocol):
        def send(self, event_type: str, payload: dict) -> None: ...

Available backends:
    stdout   — Print to stdout (default, no config needed)
    file     — Append JSON events to a file
    webhook  — POST JSON to an HTTP endpoint
    slack    — Post to Slack via incoming webhook

Config example (loi.yaml):
    watcher:
      notify:
        backend: webhook
        notify_url: http://peer-broker.local/loi/events
        auth_token_env: LOI_NOTIFY_TOKEN
"""

from __future__ import annotations

from typing import Protocol, runtime_checkable


@runtime_checkable
class NotifyBackend(Protocol):
    """Minimal interface every notify backend must implement."""

    def send(self, event_type: str, payload: dict) -> None:
        """Send a notification event.

        Args:
            event_type: e.g. "proposal.updated", "room.changed"
            payload: arbitrary dict serialisable to JSON
        """
        ...


def load_backend(config: dict) -> "NotifyBackend":
    """Instantiate and return the backend named in *config*.

    config keys:
        backend         str   Backend name: stdout | file | webhook | slack
        notify_url      str   URL for webhook/slack
        auth_token_env  str   Env var holding bearer token (webhook)
        file_path       str   Log file path (file backend)
    """
    name = config.get("backend", "stdout")

    if name == "stdout":
        from .stdout import StdoutBackend
        return StdoutBackend()

    if name == "file":
        from .file import FileBackend
        path = config.get("file_path", "loi-events.jsonl")
        return FileBackend(path)

    if name == "webhook":
        from .webhook import WebhookBackend
        url = config.get("notify_url", "")
        token_env = config.get("auth_token_env")
        return WebhookBackend(url, token_env=token_env)

    if name == "slack":
        from .slack import SlackBackend
        url = config.get("notify_url", "")
        return SlackBackend(url)

    raise ValueError(f"Unknown notify backend: {name!r}. Choose from: stdout, file, webhook, slack")


__all__ = ["NotifyBackend", "load_backend"]
