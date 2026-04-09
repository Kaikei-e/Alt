"""Content store port — run-scoped cache for article body text.

Follows 'Fetch metadata first, body only for top-N' rule:
Gatherer stores all content during search, but only curated top-N
are retrieved later by the Hydrator/Extractor.
"""

from __future__ import annotations

from typing import Protocol


class ContentStorePort(Protocol):
    async def store(self, article_id: str, content: str) -> None: ...

    async def fetch(self, article_id: str) -> str | None: ...

    async def fetch_many(self, article_ids: list[str]) -> dict[str, str]: ...
