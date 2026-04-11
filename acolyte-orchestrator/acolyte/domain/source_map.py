"""Source map for stable citation references.

Maps UUID source_ids to short stable IDs (S1, S2, ...) so Writer only
references short IDs. Finalizer resolves short IDs to titles/URLs.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class SourceEntry:
    """A registered evidence source with stable short ID."""

    short_id: str
    source_id: str
    title: str
    publisher: str = ""
    url: str = ""
    source_type: str = "article"


class SourceMap:
    """Bidirectional map between UUID source_ids and short stable IDs."""

    def __init__(self) -> None:
        self._entries: dict[str, SourceEntry] = {}  # short_id → entry
        self._id_to_short: dict[str, str] = {}  # source_id → short_id
        self._counter = 0

    def register(
        self,
        source_id: str,
        title: str,
        publisher: str = "",
        url: str = "",
        source_type: str = "article",
    ) -> str:
        """Register a source and return its short ID. Idempotent for same source_id."""
        if source_id in self._id_to_short:
            return self._id_to_short[source_id]

        self._counter += 1
        short_id = f"S{self._counter}"
        entry = SourceEntry(
            short_id=short_id,
            source_id=source_id,
            title=title,
            publisher=publisher,
            url=url,
            source_type=source_type,
        )
        self._entries[short_id] = entry
        self._id_to_short[source_id] = short_id
        return short_id

    def resolve(self, short_id: str) -> SourceEntry | None:
        """Resolve a short ID to its full source entry."""
        return self._entries.get(short_id)

    def short_id_for(self, source_id: str) -> str | None:
        """Get the short ID for a source_id, or None if not registered."""
        return self._id_to_short.get(source_id)

    def all_entries(self) -> list[SourceEntry]:
        """Return all registered entries in registration order."""
        return list(self._entries.values())

    def to_dict(self) -> dict:
        """Serialize for state storage."""
        return {
            "entries": {
                sid: {
                    "short_id": e.short_id,
                    "source_id": e.source_id,
                    "title": e.title,
                    "publisher": e.publisher,
                    "url": e.url,
                    "source_type": e.source_type,
                }
                for sid, e in self._entries.items()
            }
        }

    @classmethod
    def from_dict(cls, data: dict) -> SourceMap:
        """Deserialize from state storage."""
        sm = cls()
        for entry_data in data.get("entries", {}).values():
            sm.register(
                source_id=entry_data["source_id"],
                title=entry_data["title"],
                publisher=entry_data.get("publisher", ""),
                url=entry_data.get("url", ""),
                source_type=entry_data.get("source_type", "article"),
            )
        return sm
