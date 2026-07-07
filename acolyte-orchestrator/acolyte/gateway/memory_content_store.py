"""In-memory content store — bounded LRU cache for article bodies.

The store is a single process-global instance (see main.py) shared across
every run, not a per-run cache, so it must cap its own growth: without an
eviction policy article bodies accumulate forever and memory grows
unbounded. ``max_size`` bounds the cache to a headroom sized for a handful
of concurrent runs; least-recently-used entries are evicted first.
"""

from __future__ import annotations

from collections import OrderedDict

_DEFAULT_MAX_SIZE = 2000


class MemoryContentStore:
    """ContentStorePort implementation backed by a bounded LRU dict."""

    def __init__(self, *, max_size: int = _DEFAULT_MAX_SIZE) -> None:
        self._max_size = max_size
        self._store: OrderedDict[str, str] = OrderedDict()

    def __len__(self) -> int:
        return len(self._store)

    async def store(self, article_id: str, content: str) -> None:
        self._store[article_id] = content
        self._store.move_to_end(article_id)
        while len(self._store) > self._max_size:
            self._store.popitem(last=False)

    async def fetch(self, article_id: str) -> str | None:
        if article_id not in self._store:
            return None
        self._store.move_to_end(article_id)
        return self._store[article_id]

    async def fetch_many(self, article_ids: list[str]) -> dict[str, str]:
        result: dict[str, str] = {}
        for aid in article_ids:
            if aid in self._store:
                self._store.move_to_end(aid)
                result[aid] = self._store[aid]
        return result
