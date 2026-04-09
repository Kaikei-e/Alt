"""In-memory content store — run-scoped cache for article bodies."""

from __future__ import annotations


class MemoryContentStore:
    """ContentStorePort implementation backed by a dict."""

    def __init__(self) -> None:
        self._store: dict[str, str] = {}

    async def store(self, article_id: str, content: str) -> None:
        self._store[article_id] = content

    async def fetch(self, article_id: str) -> str | None:
        return self._store.get(article_id)

    async def fetch_many(self, article_ids: list[str]) -> dict[str, str]:
        return {aid: body for aid in article_ids if (body := self._store.get(aid)) is not None}
