"""Unit tests verifying SearchIndexerGateway passes user_id parameter for search scoping."""

from __future__ import annotations

from uuid import uuid4

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
async def test_search_articles_passes_explicit_user_id(settings: Settings, content_store: MemoryContentStore) -> None:
    target_user_id = uuid4()
    captured_request: httpx.Request | None = None

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal captured_request
        captured_request = request
        return httpx.Response(200, json={"query": "AI", "hits": []})

    transport = httpx.MockTransport(handler)
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        await gw.search_articles("AI trends", limit=10, user_id=target_user_id)

    assert captured_request is not None
    assert captured_request.url.params["user_id"] == str(target_user_id)
    assert captured_request.url.params["q"] == "AI trends"
    assert captured_request.url.params["limit"] == "10"


@pytest.mark.asyncio
async def test_search_articles_rejects_non_uuid_user_id(settings: Settings, content_store: MemoryContentStore) -> None:
    transport = httpx.MockTransport(lambda req: httpx.Response(200, json={"query": "AI", "hits": []}))
    async with httpx.AsyncClient(transport=transport, base_url="http://fake:9300") as client:
        gw = SearchIndexerGateway(client, settings, content_store)
        with pytest.raises(TypeError, match="user_id must be a UUID for article search"):
            await gw.search_articles("AI trends", limit=10, user_id="not-a-uuid")  # type: ignore[arg-type]
