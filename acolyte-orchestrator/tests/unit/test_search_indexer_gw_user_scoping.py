"""Unit tests verifying SearchIndexerGateway passes user_id parameter for search scoping."""

from __future__ import annotations

import asyncio
from uuid import uuid4

import httpx
import jwt
import pytest

from acolyte.config.settings import Settings
from acolyte.gateway.memory_content_store import MemoryContentStore
from acolyte.gateway.search_indexer_gw import SearchIndexerGateway
from acolyte.infra.user_identity import current_user_jwt
from acolyte.port.evidence_provider import EvidenceStatusError


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


@pytest.mark.asyncio
async def test_search_articles_passes_owner_jwt_to_upstream(content_store: MemoryContentStore) -> None:
    """Verify SearchIndexerGateway forwards the caller's JWT for authorization."""
    target_user_id = uuid4()
    captured_request: httpx.Request | None = None

    def handler(request: httpx.Request) -> httpx.Response:
        nonlocal captured_request
        captured_request = request
        return httpx.Response(200, json={"query": "deep learning", "hits": []})

    mtls_settings = Settings(search_indexer_url="https://search-indexer:9443")
    transport = httpx.MockTransport(handler)

    test_token = jwt.encode(
        {"sub": str(target_user_id), "iss": "auth-hub", "aud": "alt-backend"},
        "secret",
        algorithm="HS256",
    )
    token_var = current_user_jwt.set(test_token)

    try:
        async with httpx.AsyncClient(transport=transport, base_url="https://search-indexer:9443") as client:
            gw = SearchIndexerGateway(client, mtls_settings, content_store)
            await gw.search_articles("deep learning", limit=20, user_id=target_user_id)
    finally:
        current_user_jwt.reset(token_var)

    assert captured_request is not None
    assert str(captured_request.url).startswith("https://search-indexer:9443/v1/search")
    assert captured_request.url.params["user_id"] == str(target_user_id)
    assert captured_request.url.params["q"] == "deep learning"
    assert captured_request.headers.get("x-alt-backend-token") == test_token


# GoC1: SearchGW authorization tested in tests/unit/test_search_indexer_gw_user_scoping.py


@pytest.mark.asyncio
async def test_search_articles_fails_without_owner_jwt(content_store: MemoryContentStore) -> None:
    """Request carries NO owner token → concrete EvidenceStatusError with status 401.

    The MockTransport inspects the request and returns 401 only when no
    x-alt-backend-token header is present, which proves:
    (a) the gateway omits the header when the contextvar is None, AND
    (b) the gateway translates the 401 HTTPStatusError into EvidenceStatusError(401).

    This is NOT a broad raises(Exception) — the exact status code is pinned.
    """

    def inspecting_handler(request: httpx.Request) -> httpx.Response:
        # Simulates upstream: auth check rejects requests without a backend token
        if not request.headers.get("x-alt-backend-token"):
            return httpx.Response(401)
        return httpx.Response(200, json={"query": "q", "hits": []})

    mtls_settings = Settings(search_indexer_url="https://search-indexer:9443")
    transport = httpx.MockTransport(inspecting_handler)

    token_var = current_user_jwt.set(None)
    try:
        async with httpx.AsyncClient(transport=transport, base_url="https://search-indexer:9443") as client:
            gw = SearchIndexerGateway(client, mtls_settings, content_store)
            with pytest.raises(EvidenceStatusError) as exc_info:
                await gw.search_articles("query", limit=10, user_id=uuid4())
    finally:
        current_user_jwt.reset(token_var)

    assert exc_info.value.status_code == 401


@pytest.mark.asyncio
async def test_outgoing_request_with_valid_owner_jwt_returns_200(content_store: MemoryContentStore) -> None:
    """Request WITH valid owner JWT → 200; x-alt-backend-token header captured and equals the set token."""
    owner_id = uuid4()
    test_token = jwt.encode(
        {"sub": str(owner_id), "iss": "auth-hub", "aud": "alt-backend"},
        "test-secret",
        algorithm="HS256",
    )
    captured_headers: dict[str, str] = {}

    def inspecting_handler(request: httpx.Request) -> httpx.Response:
        backend_token = request.headers.get("x-alt-backend-token", "")
        captured_headers["x-alt-backend-token"] = backend_token
        if backend_token:
            return httpx.Response(200, json={"query": "q", "hits": []})
        return httpx.Response(401)

    mtls_settings = Settings(search_indexer_url="https://search-indexer:9443")
    transport = httpx.MockTransport(inspecting_handler)

    token_var = current_user_jwt.set(test_token)
    try:
        async with httpx.AsyncClient(transport=transport, base_url="https://search-indexer:9443") as client:
            gw = SearchIndexerGateway(client, mtls_settings, content_store)
            # Must NOT raise — valid token yields 200
            hits = await gw.search_articles("query", limit=5, user_id=owner_id)
    finally:
        current_user_jwt.reset(token_var)

    # Assert the exact token was sent on the wire (request-local, not bleed from elsewhere)
    assert captured_headers["x-alt-backend-token"] == test_token
    assert hits == []


@pytest.mark.asyncio
async def test_concurrent_2_owners_no_bleed() -> None:
    """Two concurrent requests with different owner JWTs must not bleed tokens into each other.

    asyncio.gather interleaves two coroutines; each captures the token it sent and
    verifies it matches only its own owner's JWT (contextvar isolation via contextvars.ContextVar).
    """
    owner_a = uuid4()
    owner_b = uuid4()
    token_a = jwt.encode(
        {"sub": str(owner_a), "iss": "auth-hub", "aud": "alt-backend"},
        "secret-a",
        algorithm="HS256",
    )
    token_b = jwt.encode(
        {"sub": str(owner_b), "iss": "auth-hub", "aud": "alt-backend"},
        "secret-b",
        algorithm="HS256",
    )

    # Maps owner_id → token header seen in each request
    seen: dict[str, str] = {}

    def handler(request: httpx.Request) -> httpx.Response:
        uid = request.url.params.get("user_id", "unknown")
        seen[uid] = request.headers.get("x-alt-backend-token", "")
        return httpx.Response(200, json={"query": "q", "hits": []})

    transport = httpx.MockTransport(handler)

    async def call_as(owner_id: uuid4, token: str) -> None:  # type: ignore[valid-type]
        cs = MemoryContentStore()
        tok_var = current_user_jwt.set(token)
        try:
            async with httpx.AsyncClient(transport=transport, base_url="https://search-indexer:9443") as client:
                gw = SearchIndexerGateway(
                    client,
                    Settings(search_indexer_url="https://search-indexer:9443"),
                    cs,
                )
                await gw.search_articles("q", limit=5, user_id=owner_id)
        finally:
            current_user_jwt.reset(tok_var)

    await asyncio.gather(
        call_as(owner_a, token_a),
        call_as(owner_b, token_b),
    )

    # Each owner's request must carry its own token, not the other's
    assert seen[str(owner_a)] == token_a, f"Owner A got wrong token: {seen[str(owner_a)]!r}"
    assert seen[str(owner_b)] == token_b, f"Owner B got wrong token: {seen[str(owner_b)]!r}"
    assert seen[str(owner_a)] != seen[str(owner_b)], "Token bleed detected between concurrent owners"
