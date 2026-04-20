"""HyDE-generated passages can exceed 600 chars; the gateway's INFO log
must truncate the query so observability pipelines are not flooded with
synthetic topic content (security review Finding 2, Low)."""

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


@pytest.fixture
def mock_transport() -> httpx.MockTransport:
    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"query": "x", "hits": []})

    return httpx.MockTransport(handler)


@pytest.mark.asyncio
async def test_search_articles_log_truncates_long_query(
    settings: Settings,
    content_store: MemoryContentStore,
    mock_transport: httpx.MockTransport,
) -> None:
    long_query = "A" * 500
    async with httpx.AsyncClient(transport=mock_transport) as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        with capture_logs() as logs:
            await gw.search_articles(long_query, limit=1)

    search_events = [e for e in logs if e.get("event") == "search_articles"]
    assert search_events, "gateway must log a search_articles event"
    logged_query = search_events[0].get("query", "")
    assert len(logged_query) <= 160, f"logged query must be truncated; got {len(logged_query)} chars"
    assert logged_query.startswith("A"), "truncation should keep the leading portion"


@pytest.mark.asyncio
async def test_search_articles_log_keeps_short_query_intact(
    settings: Settings,
    content_store: MemoryContentStore,
    mock_transport: httpx.MockTransport,
) -> None:
    short_query = "イラン情勢 2026"
    async with httpx.AsyncClient(transport=mock_transport) as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        with capture_logs() as logs:
            await gw.search_articles(short_query, limit=1)

    search_events = [e for e in logs if e.get("event") == "search_articles"]
    assert search_events
    assert short_query in search_events[0].get("query", "")
