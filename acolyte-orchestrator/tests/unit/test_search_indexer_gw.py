"""Unit tests for search-indexer gateway.

Tests are aligned with search-indexer actual REST API:
  GET /v1/search → {query, hits: [{id, title, content, tags, score}]}

score is Meilisearch _rankingScore (0.0-1.0).
No url or published_at in response.
"""

from __future__ import annotations

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
                            "content": "Full article body text about artificial intelligence and market trends.",
                            "tags": ["AI", "technology"],
                        },
                        {
                            "id": "art-2",
                            "title": "Second Article",
                            "content": "Another article about technology.",
                            "tags": ["tech"],
                        },
                    ],
                },
            )
        return httpx.Response(404)

    return httpx.MockTransport(handler)


@pytest.mark.asyncio
async def test_search_articles_returns_metadata_only(
    settings: Settings, mock_transport: httpx.MockTransport, content_store: MemoryContentStore
) -> None:
    """ArticleHit should contain metadata fields only — no content."""
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        hits = await gw.search_articles("AI trends", limit=10)

    assert len(hits) == 2
    assert hits[0].article_id == "art-1"
    assert hits[0].title == "Test Article about AI"
    assert hits[0].tags == ["AI", "technology"]
    # score defaults to 0.0 since search-indexer doesn't return _rankingScore
    assert hits[0].score == 0.0


@pytest.mark.asyncio
async def test_search_articles_stores_content_in_content_store(
    settings: Settings, mock_transport: httpx.MockTransport, content_store: MemoryContentStore
) -> None:
    """Content from search response should be stored in ContentStore, not in ArticleHit."""
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        await gw.search_articles("AI trends", limit=10)

    body = await content_store.fetch("art-1")
    assert body == "Full article body text about artificial intelligence and market trends."
    body2 = await content_store.fetch("art-2")
    assert body2 == "Another article about technology."


@pytest.mark.asyncio
async def test_search_articles_empty_response(
    settings: Settings, content_store: MemoryContentStore
) -> None:
    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json={"query": "xyz", "hits": []})

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        hits = await gw.search_articles("xyz", limit=10)

    assert hits == []


@pytest.mark.asyncio
async def test_search_articles_extracts_score(
    settings: Settings, content_store: MemoryContentStore
) -> None:
    """Score from search-indexer response should be propagated to ArticleHit."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "query": "AI",
                "hits": [
                    {"id": "a1", "title": "T1", "content": "C1", "tags": [], "score": 0.85},
                    {"id": "a2", "title": "T2", "content": "C2", "tags": [], "score": 0.42},
                ],
            },
        )

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        hits = await gw.search_articles("AI", limit=10)

    assert hits[0].score == 0.85
    assert hits[1].score == 0.42


@pytest.mark.asyncio
async def test_search_articles_score_default_zero(
    settings: Settings, content_store: MemoryContentStore
) -> None:
    """When score is missing from response, default to 0.0."""

    def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "query": "AI",
                "hits": [{"id": "a1", "title": "T1", "content": "C1", "tags": []}],
            },
        )

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        hits = await gw.search_articles("AI", limit=10)

    assert hits[0].score == 0.0


@pytest.mark.asyncio
async def test_search_recaps_returns_empty(
    settings: Settings, mock_transport: httpx.MockTransport, content_store: MemoryContentStore
) -> None:
    """Recap search via REST is not available — returns empty list."""
    async with httpx.AsyncClient(transport=mock_transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        hits = await gw.search_recaps("technology trends")

    assert hits == []
