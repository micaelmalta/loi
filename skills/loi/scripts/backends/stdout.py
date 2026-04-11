"""Stdout notify backend — prints events as JSON to stdout."""

from __future__ import annotations

import json
from datetime import datetime, timezone


class StdoutBackend:
    """Print every event as a JSON line to stdout."""

    def send(self, event_type: str, payload: dict) -> None:
        event = {
            "event": event_type,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            **payload,
        }
        print(json.dumps(event))
