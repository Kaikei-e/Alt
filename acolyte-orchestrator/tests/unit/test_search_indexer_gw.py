"""Unit tests for search-indexer gateway."""

from __future__ import annotations

import httpx
import pytest

from acolyte.config.settings import Settings
from acolyte.gateway.search_indexer_gw import SearchIndexerGateway


@pytest.fixture
def settings() -> Settings:
    return Settings(search_indexer_url="http://fake:9300")


@pytest.fixture
def mock_transport() -> httpx.MockTransport:
    def handler(request: httpx.Request) -> httpx.Response:
        if "/v1/search" in str(request.url):
            return httpx.Response(
                200,
                json={
                    "query": "AI",
                    "hits": [
                        {
                            "id": "art-1",
                            "title": "Test Article about AI",
                            "url": "https://example.com/1",
                            "content": "Full article body text about artificial intelligence and market trends.",
                            "_rankingScore": 0.95,
                            "published_at": "2026-04-01T00:00:00Z",
                        }
                    ],
                },
            )
        return httpx.Response(404)

    return httpx.MockTransport(handler)


@pytest.mark.asyncio
async def test_search_articles(settings: Settings, mock_transport: httpx.MockTransport) -> None:
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings)
        hits = await gw.search_articles("AI trends", limit=10)

    assert len(hits) == 1
    assert hits[0].article_id == "art-1"
    assert hits[0].title == "Test Article about AI"
    assert hits[0].score == 0.95
    assert hits[0].excerpt is not None


@pytest.mark.asyncio
async def test_search_articles_truncates_excerpt(settings: Settings, mock_transport: httpx.MockTransport) -> None:
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings)
        hits = await gw.search_articles("AI", limit=1)

    # Content shorter than 200 chars should not be truncated with "..."
    assert hits[0].excerpt
    assert not hits[0].excerpt.endswith("...")


@pytest.mark.asyncio
async def test_search_recaps_returns_empty(settings: Settings, mock_transport: httpx.MockTransport) -> None:
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings)
        hits = await gw.search_recaps("technology trends")

    assert hits == []
