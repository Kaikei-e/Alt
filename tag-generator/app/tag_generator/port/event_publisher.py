"""Port for publishing events to external systems."""

from __future__ import annotations

from typing import Any, Protocol


class EventPublisherPort(Protocol):
    """Port for publishing events (e.g. Redis Streams)."""

    async def publish_reply(self, stream_key: str, event_data: dict[str, Any]) -> str | None: ...
