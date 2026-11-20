"""Unit tests for request size limit middleware and settings."""

from __future__ import annotations

from http import HTTPStatus
from pathlib import Path

import httpx
import pytest
import yaml
from fastapi import FastAPI
from fastapi.testclient import TestClient
from pydantic import ValidationError
from starlette.responses import JSONResponse
from starlette.types import Receive, Scope, Send

from recap_subworker.app.main import (
    DEFAULT_MAX_REQUEST_BODY_BYTES,
    RequestSizeLimitMiddleware,
    create_app,
)
from recap_subworker.infra.config import Settings, get_settings

pytestmark = pytest.mark.usefixtures("stub_startup_sweep")

REPO_ROOT = Path(__file__).resolve().parents[3]
COMPOSE_RECAP_PATH = REPO_ROOT / "compose" / "recap.yaml"

# Sizing constants for a full 72-hour story clustering batch sent by recap-worker:
# each ClusterStoryItem carries id (UUID), embedding (1,024 f32 values),
# published_at (RFC3339 UTC timestamp), and language ("ja" / "en" / "other").
CLUSTER_STORY_COUNT = 7_000
EMBEDDING_DIM = 1_024
BYTES_PER_FLOAT_JSON = 13
# Non-embedding per-item JSON wrapper + comma separator:
# {"id":"<36-char-uuid>","embedding":...,"published_at":"YYYY-MM-DDTHH:MM:SSZ","language":"ja"},
PER_ITEM_OVERHEAD_BYTES = 113
# Top-level request envelope: {"items":[],"params":{...}}
REQUEST_ENVELOPE_OVERHEAD_BYTES = 107
CLUSTER_STORIES_BODY_BYTES = (
    CLUSTER_STORY_COUNT * (EMBEDDING_DIM * BYTES_PER_FLOAT_JSON + PER_ITEM_OVERHEAD_BYTES)
    + REQUEST_ENVELOPE_OVERHEAD_BYTES
)
EXPECTED_DEFAULT_MAX_REQUEST_BODY_BYTES = 96 * 1024 * 1024
EXACT_96_MIB_BYTES = 100_663_296
CUSTOM_LIMIT_1_MIB_BYTES = 1_048_576
CUSTOM_LIMIT_2_MIB_BYTES = 2_097_152
EXPECTED_ASGI_RESPONSE_EVENTS = 2

# Memory budget constants for quadratic pairwise distance/clustering allocation.
CLUSTERING_BYTES_PER_ITEM_PAIR = 54
CONTAINER_MEMORY_LIMIT_BYTES = 8 * 1024**3
MEASURED_BYTES_PER_CLUSTER_ITEM = 12_883


def test_settings_max_request_body_bytes_default(monkeypatch: pytest.MonkeyPatch) -> None:
    """Settings should default max_request_body_bytes to 96 MiB after clearing env aliases."""
    monkeypatch.delenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", raising=False)
    monkeypatch.delenv("RECAP_MAX_REQUEST_BODY_BYTES", raising=False)
    settings = Settings()
    assert settings.max_request_body_bytes == EXPECTED_DEFAULT_MAX_REQUEST_BODY_BYTES
    assert settings.max_request_body_bytes == EXACT_96_MIB_BYTES
    assert DEFAULT_MAX_REQUEST_BODY_BYTES == EXACT_96_MIB_BYTES


def test_settings_max_request_body_bytes_env_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Settings should respect RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES env variable."""
    monkeypatch.setenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", str(CUSTOM_LIMIT_1_MIB_BYTES))
    monkeypatch.delenv("RECAP_MAX_REQUEST_BODY_BYTES", raising=False)
    settings = Settings()
    assert settings.max_request_body_bytes == CUSTOM_LIMIT_1_MIB_BYTES


def test_settings_max_request_body_bytes_alias_env_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Settings should respect RECAP_MAX_REQUEST_BODY_BYTES alias env variable."""
    monkeypatch.delenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", raising=False)
    monkeypatch.setenv("RECAP_MAX_REQUEST_BODY_BYTES", str(CUSTOM_LIMIT_2_MIB_BYTES))
    settings = Settings()
    assert settings.max_request_body_bytes == CUSTOM_LIMIT_2_MIB_BYTES


@pytest.mark.parametrize("invalid_val", ["abc", "0", "-1"])
def test_settings_max_request_body_bytes_invalid_raises(
    monkeypatch: pytest.MonkeyPatch, invalid_val: str
) -> None:
    """Invalid values must raise ValidationError fail-fast at boot."""
    monkeypatch.setenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", invalid_val)
    with pytest.raises(ValidationError):
        Settings()


def test_middleware_request_just_under_limit_passes() -> None:
    """Request with Content-Length at or below max_bytes must pass through."""
    app = FastAPI()

    @app.post("/echo")
    async def echo(payload: dict) -> JSONResponse:
        return JSONResponse({"received": payload})

    # Set limit to 100 bytes
    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=100)

    with TestClient(app) as client:
        # A payload whose JSON encoding is under 100 bytes
        body = {"key": "under"}
        response = client.post("/echo", json=body)
        assert response.status_code == HTTPStatus.OK
        assert response.json() == {"received": {"key": "under"}}


def test_middleware_request_over_limit_returns_413() -> None:
    """Request with Content-Length exceeding max_bytes must return 413 with fixed detail."""
    app = FastAPI()

    @app.post("/echo")
    async def echo(payload: dict) -> JSONResponse:
        return JSONResponse({"received": payload})

    # Set limit to 50 bytes
    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=50)

    with TestClient(app) as client:
        # A payload whose JSON encoding exceeds 50 bytes
        large_body = {"data": "x" * 100}
        response = client.post("/echo", json=large_body)
        assert response.status_code == HTTPStatus.REQUEST_ENTITY_TOO_LARGE
        assert response.headers["content-type"] == "application/json"
        assert response.json() == {"detail": "Request body too large"}


@pytest.mark.asyncio
async def test_middleware_chunked_transfer_encoding_without_content_length_returns_411() -> None:
    """Transfer-Encoding without Content-Length must return 411 Length Required."""
    app = FastAPI()
    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=100)

    @app.post("/test")
    async def endpoint() -> dict[str, bool]:
        return {"ok": True}

    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/test", headers={"transfer-encoding": "chunked"})
    assert resp.status_code == HTTPStatus.LENGTH_REQUIRED
    assert resp.headers["content-type"] == "application/json"
    assert resp.json() == {"detail": "Length Required"}


@pytest.mark.asyncio
async def test_middleware_chunked_transfer_encoding_with_content_length_returns_411() -> None:
    """Transfer-Encoding with Content-Length present must return 411 Length Required."""
    app = FastAPI()
    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=100)

    @app.post("/test")
    async def endpoint() -> dict[str, bool]:
        return {"ok": True}

    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post(
            "/test",
            headers={"transfer-encoding": "chunked", "content-length": "1"},
        )
    assert resp.status_code == HTTPStatus.LENGTH_REQUIRED
    assert resp.headers["content-type"] == "application/json"
    assert resp.json() == {"detail": "Length Required"}


@pytest.mark.parametrize("invalid_length", ["invalid", "-1", "-100", "12.34", "+5", " 5 ", "1_0"])
@pytest.mark.asyncio
async def test_middleware_malformed_content_length_returns_400(invalid_length: str) -> None:
    """Malformed, non-digit, or negative Content-Length must return 400 Bad Request."""
    app = FastAPI()
    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=100)

    @app.post("/test")
    async def endpoint() -> dict[str, bool]:
        return {"ok": True}

    transport = httpx.ASGITransport(app=app)
    async with httpx.AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.post("/test", headers={"content-length": invalid_length})
    assert resp.status_code == HTTPStatus.BAD_REQUEST
    assert resp.headers["content-type"] == "application/json"
    assert resp.json() == {"detail": "Invalid Content-Length"}


@pytest.mark.asyncio
async def test_middleware_conflicting_content_length_headers_returns_400() -> None:
    """Conflicting duplicate Content-Length headers must return 400 Bad Request."""
    sent_events: list[dict[str, object]] = []

    async def dummy_send(event: dict[str, object]) -> None:
        sent_events.append(event)

    async def dummy_receive() -> dict[str, object]:
        return {"type": "http.request", "body": b""}

    async def inner_app(scope: Scope, receive: Receive, send: Send) -> None:
        pass

    middleware = RequestSizeLimitMiddleware(inner_app, max_bytes=100)
    scope: Scope = {
        "type": "http",
        "headers": [
            (b"content-length", b"10"),
            (b"content-length", b"20"),
        ],
    }
    await middleware(scope, dummy_receive, dummy_send)  # type: ignore[arg-type]
    assert len(sent_events) == EXPECTED_ASGI_RESPONSE_EVENTS
    assert sent_events[0]["status"] == HTTPStatus.BAD_REQUEST
    assert sent_events[1]["body"] == b'{"detail":"Invalid Content-Length"}'


@pytest.mark.asyncio
async def test_middleware_non_http_scope_passthrough() -> None:
    """Non-HTTP scope passes through to inner application."""
    called = False

    async def inner_app(scope: Scope, receive: Receive, send: Send) -> None:
        nonlocal called
        called = True

    async def dummy_receive() -> dict[str, object]:
        return {}

    async def dummy_send(event: dict[str, object]) -> None:
        pass

    middleware = RequestSizeLimitMiddleware(inner_app, max_bytes=100)
    scope: Scope = {"type": "lifespan"}
    await middleware(scope, dummy_receive, dummy_send)  # type: ignore[arg-type]
    assert called is True


def test_create_app_enforces_configured_request_size_limit(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """The application created by create_app must enforce settings.max_request_body_bytes."""
    limit_bytes = 200
    monkeypatch.setenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", str(limit_bytes))
    get_settings.cache_clear()

    try:
        app = create_app()
        with TestClient(app) as client:
            # Under limit: passes size middleware (not 413, proceeds to auth/validation)
            small_payload = {"items": []}
            res_small = client.post("/v1/cluster-stories", json=small_payload)
            assert res_small.status_code != HTTPStatus.REQUEST_ENTITY_TOO_LARGE

            # Over limit: rejected by RequestSizeLimitMiddleware (413) before route handling
            large_payload = {"items": [{"padding": "a" * (limit_bytes + 50)}]}
            res_large = client.post("/v1/cluster-stories", json=large_payload)
            assert res_large.status_code == HTTPStatus.REQUEST_ENTITY_TOO_LARGE
            assert res_large.json() == {"detail": "Request body too large"}
    finally:
        get_settings.cache_clear()


def _load_recap_subworker_compose_service() -> dict[str, object] | None:
    if not COMPOSE_RECAP_PATH.is_file():
        return None
    compose_data = yaml.safe_load(COMPOSE_RECAP_PATH.read_text(encoding="utf-8"))
    return compose_data["services"]["recap-subworker"]


@pytest.mark.asyncio
async def test_default_limit_admits_7000_story_cluster_payload(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Default limit must admit a 7,000-story clustering payload without returning 413."""
    monkeypatch.delenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", raising=False)
    monkeypatch.delenv("RECAP_MAX_REQUEST_BODY_BYTES", raising=False)
    settings = Settings()

    for max_bytes in (DEFAULT_MAX_REQUEST_BODY_BYTES, settings.max_request_body_bytes):
        forwarded = False
        sent_events: list[dict[str, object]] = []

        async def inner_app(scope: Scope, receive: Receive, send: Send) -> None:
            nonlocal forwarded
            forwarded = True
            await send({"type": "http.response.start", "status": 200, "headers": []})
            await send({"type": "http.response.body", "body": b'{"ok":true}'})

        async def dummy_receive() -> dict[str, object]:
            return {"type": "http.request", "body": b"", "more_body": False}

        async def dummy_send(
            event: dict[str, object], events: list[dict[str, object]] = sent_events
        ) -> None:
            events.append(event)

        middleware = RequestSizeLimitMiddleware(inner_app, max_bytes=max_bytes)
        scope: Scope = {
            "type": "http",
            "method": "POST",
            "path": "/v1/cluster-stories",
            "headers": [
                (b"content-type", b"application/json"),
                (b"content-length", str(CLUSTER_STORIES_BODY_BYTES).encode("ascii")),
            ],
        }
        await middleware(scope, dummy_receive, dummy_send)  # type: ignore[arg-type]
        assert forwarded is True, (
            f"Expected {CLUSTER_STORIES_BODY_BYTES} bytes (7,000 stories) to fit within {max_bytes}"
        )
        assert sent_events[0]["status"] == HTTPStatus.OK


@pytest.mark.asyncio
async def test_default_limit_rejects_one_byte_over_with_413(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Request one byte over the default limit must be rejected with 413."""
    monkeypatch.delenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", raising=False)
    monkeypatch.delenv("RECAP_MAX_REQUEST_BODY_BYTES", raising=False)
    settings = Settings()

    for max_bytes in (DEFAULT_MAX_REQUEST_BODY_BYTES, settings.max_request_body_bytes):
        forwarded = False
        sent_events: list[dict[str, object]] = []

        async def inner_app(scope: Scope, receive: Receive, send: Send) -> None:
            nonlocal forwarded
            forwarded = True

        async def dummy_receive() -> dict[str, object]:
            return {"type": "http.request", "body": b"", "more_body": False}

        async def dummy_send(
            event: dict[str, object], events: list[dict[str, object]] = sent_events
        ) -> None:
            events.append(event)

        middleware = RequestSizeLimitMiddleware(inner_app, max_bytes=max_bytes)
        scope: Scope = {
            "type": "http",
            "method": "POST",
            "path": "/v1/cluster-stories",
            "headers": [
                (b"content-type", b"application/json"),
                (b"content-length", str(max_bytes + 1).encode("ascii")),
            ],
        }
        await middleware(scope, dummy_receive, dummy_send)  # type: ignore[arg-type]
        assert forwarded is False
        assert sent_events[0]["status"] == HTTPStatus.REQUEST_ENTITY_TOO_LARGE
        assert sent_events[1]["body"] == b'{"detail":"Request body too large"}'


def test_default_limit_stays_within_clustering_memory_budget(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Largest admitted batch must keep quadratic clustering memory within half the container limit."""
    monkeypatch.delenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", raising=False)
    monkeypatch.delenv("RECAP_MAX_REQUEST_BODY_BYTES", raising=False)
    settings = Settings()

    for limit_bytes in (DEFAULT_MAX_REQUEST_BODY_BYTES, settings.max_request_body_bytes):
        max_items = limit_bytes // MEASURED_BYTES_PER_CLUSTER_ITEM
        clustering_bytes = CLUSTERING_BYTES_PER_ITEM_PAIR * max_items * max_items
        assert clustering_bytes <= CONTAINER_MEMORY_LIMIT_BYTES // 2, (
            f"Limit {limit_bytes} bytes admits {max_items} items needing {clustering_bytes} bytes "
            f"for clustering, exceeding half of {CONTAINER_MEMORY_LIMIT_BYTES} bytes"
        )

    service = _load_recap_subworker_compose_service()
    if service is None:
        pytest.skip(f"compose file not present at {COMPOSE_RECAP_PATH}")
    assert service["mem_limit"] == "8192m"


def test_compose_recap_max_request_body_bytes_matches_code_default(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Compose configuration for recap-subworker must match the code default and admit 7,000 stories."""
    service = _load_recap_subworker_compose_service()
    if service is None:
        pytest.skip(f"compose file not present at {COMPOSE_RECAP_PATH}")

    monkeypatch.delenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", raising=False)
    monkeypatch.delenv("RECAP_MAX_REQUEST_BODY_BYTES", raising=False)
    settings = Settings()

    env_entries = service["environment"]
    env_map: dict[str, str] = {}
    if isinstance(env_entries, dict):
        env_map = {str(k): str(v) for k, v in env_entries.items()}
    else:
        assert isinstance(env_entries, list)
        for entry in env_entries:
            key, _, val = str(entry).partition("=")
            env_map[key] = val

    assert "RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES" in env_map
    compose_limit = int(env_map["RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES"])
    assert compose_limit == DEFAULT_MAX_REQUEST_BODY_BYTES
    assert compose_limit == settings.max_request_body_bytes
    assert compose_limit >= CLUSTER_STORIES_BODY_BYTES
