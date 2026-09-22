"""Unit tests for request size limit middleware and settings."""

from __future__ import annotations

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient
from starlette.responses import JSONResponse

from recap_subworker.app.main import (
    DEFAULT_MAX_REQUEST_BODY_BYTES,
    RequestSizeLimitMiddleware,
    create_app,
)
from recap_subworker.infra.config import Settings, get_settings


def test_settings_max_request_body_bytes_default() -> None:
    """Settings should default max_request_body_bytes to 64 MiB."""
    settings = Settings()
    assert settings.max_request_body_bytes == 64 * 1024 * 1024
    assert settings.max_request_body_bytes == 67108864
    assert DEFAULT_MAX_REQUEST_BODY_BYTES == 67108864


def test_settings_max_request_body_bytes_env_override(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Settings should respect RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES env variable."""
    monkeypatch.setenv("RECAP_SUBWORKER_MAX_REQUEST_BODY_BYTES", "1048576")
    settings = Settings()
    assert settings.max_request_body_bytes == 1048576


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
        assert response.status_code == 200
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
        assert response.status_code == 413
        assert response.headers["content-type"] == "application/json"
        assert response.json() == {"detail": "Request body too large"}


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
            assert res_small.status_code != 413

            # Over limit: rejected by RequestSizeLimitMiddleware (413) before route handling
            large_payload = {"items": [{"padding": "a" * (limit_bytes + 50)}]}
            res_large = client.post("/v1/cluster-stories", json=large_payload)
            assert res_large.status_code == 413
            assert res_large.json() == {"detail": "Request body too large"}
    finally:
        get_settings.cache_clear()
