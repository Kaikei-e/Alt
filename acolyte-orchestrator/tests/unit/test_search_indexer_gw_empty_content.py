"""search-indexer content-crop bug (2026-07-23): a Meilisearch driver bug made
_formatted.content decode to "" for every hit, so search_articles silently
stored nothing in ContentStore. The gateway swallowed this with a plain
``if content:`` skip. Per CLAUDE.md Rule 8 (no silent fallback), an empty
content hit must surface as a WARN log, not disappear quietly."""

from __future__ import annotations

import httpx
import pytest
from structlog.testing import capture_logs

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
async def test_search_articles_warns_on_empty_content(settings: Settings, content_store: MemoryContentStore) -> None:
    """A hit with empty content must emit a WARN log and must not be stored."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "query": "AI",
                "hits": [
                    {"id": "a1", "title": "T1", "content": "", "tags": []},
                ],
            },
        )

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        with capture_logs() as logs:
            hits = await gw.search_articles("AI", limit=10)

    assert len(hits) == 1
    assert await content_store.fetch("a1") is None

    warn_events = [e for e in logs if e.get("log_level") == "warning"]
    assert warn_events, "empty content hit must emit a warning log"
    assert warn_events[0].get("event") == "search_articles_empty_content"
    assert warn_events[0].get("article_id") == "a1"


@pytest.mark.asyncio
async def test_search_articles_warns_once_per_empty_hit(settings: Settings, content_store: MemoryContentStore) -> None:
    """Every empty-content hit in a batch is counted -- the warning is not
    deduplicated across hits, so the WARN count reflects the real blast
    radius of the underlying content bug."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "query": "AI",
                "hits": [
                    {"id": "a1", "title": "T1", "content": "", "tags": []},
                    {"id": "a2", "title": "T2", "content": "real content here", "tags": []},
                    {"id": "a3", "title": "T3", "content": "", "tags": []},
                ],
            },
        )

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        with capture_logs() as logs:
            await gw.search_articles("AI", limit=10)

    warn_events = [
        e for e in logs if e.get("log_level") == "warning" and e.get("event") == "search_articles_empty_content"
    ]
    assert len(warn_events) == 2
    assert {e.get("article_id") for e in warn_events} == {"a1", "a3"}
    assert await content_store.fetch("a2") == "real content here"


@pytest.mark.asyncio
async def test_search_articles_no_warning_when_content_present(
    settings: Settings, content_store: MemoryContentStore
) -> None:
    """The healthy path (non-empty content) must not emit the empty-content warning."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "query": "AI",
                "hits": [{"id": "a1", "title": "T1", "content": "real content", "tags": []}],
            },
        )

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        with capture_logs() as logs:
            await gw.search_articles("AI", limit=10)

    warn_events = [
        e for e in logs if e.get("log_level") == "warning" and e.get("event") == "search_articles_empty_content"
    ]
    assert warn_events == []
