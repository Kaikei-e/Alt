"""Gateway must forward ``published_after`` / ``published_before`` as
ISO-8601 UTC timestamps when the Gatherer supplies a window, so
search-indexer can filter stale articles out of weekly_briefing
retrievals."""

from __future__ import annotations

from datetime import UTC, datetime

import httpx
import pytest

from acolyte.config.settings import Settings
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.gateway.search_indexer_gw import SearchIndexerGateway


@pytest.fixture
def settings() -> Settings:
    return Settings(search_indexer_url="http://fake:9300")


@pytest.fixture
def content_store() -> MemoryContentStore:
    return MemoryContentStore()


@pytest.mark.asyncio
async def test_search_articles_passes_date_window(settings: Settings, content_store: MemoryContentStore) -> None:
    captured: dict[str, str] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured.update(dict(request.url.params))
        return httpx.Response(200, json={"query": "x", "hits": []})

    after = datetime(2026, 4, 12, tzinfo=UTC)
    before = datetime(2026, 4, 20, tzinfo=UTC)

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        await gw.search_articles("イラン情勢", limit=5, published_after=after, published_before=before)

    assert captured.get("published_after") == "2026-04-12T00:00:00+00:00"
    assert captured.get("published_before") == "2026-04-20T00:00:00+00:00"


@pytest.mark.asyncio
async def test_search_articles_omits_date_params_when_not_set(
    settings: Settings, content_store: MemoryContentStore
) -> None:
    captured: dict[str, str] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        captured.update(dict(request.url.params))
        return httpx.Response(200, json={"query": "x", "hits": []})

    async with httpx.AsyncClient(transport=httpx.MockTransport(handler)) as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        await gw.search_articles("イラン情勢", limit=5)

    assert "published_after" not in captured
    assert "published_before" not in captured
