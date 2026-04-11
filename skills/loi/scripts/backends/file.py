"""File notify backend — appends JSON events to a .jsonl file."""

from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path


class FileBackend:
    """Append each event as a newline-delimited JSON record to *file_path*."""

    def __init__(self, file_path: str) -> None:
        self.file_path = Path(file_path)

    def send(self, event_type: str, payload: dict) -> None:
        event = {
            "event": event_type,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            **payload,
        }
        self.file_path.parent.mkdir(parents=True, exist_ok=True)
        with self.file_path.open("a", encoding="utf-8") as fh:
            fh.write(json.dumps(event) + "\n")
        print(f"[LOI] Event written to {self.file_path}")
