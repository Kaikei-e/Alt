"""Tests for X-Service-Token authentication."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest
from httpx import ASGITransport, AsyncClient

from tts_speaker.app.main import create_app


@pytest.mark.asyncio
async def test_auth_required_when_secret_set(mock_pipeline: MagicMock):
    """Authenticated endpoints reject requests without token when SECRET is set."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post("/v1/synthesize", json={"text": "テスト"})
            assert resp.status_code == 401


@pytest.mark.asyncio
async def test_auth_wrong_token(mock_pipeline: MagicMock):
    """Authenticated endpoints reject requests with wrong token."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post(
                "/v1/synthesize",
                json={"text": "テスト"},
                headers={"X-Service-Token": "wrong-token"},
            )
            assert resp.status_code == 401


@pytest.mark.asyncio
async def test_auth_correct_token(mock_pipeline: MagicMock):
    """Authenticated endpoints accept requests with correct token."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post(
                "/v1/synthesize",
                json={"text": "テスト"},
                headers={"X-Service-Token": "test-secret"},
            )
            assert resp.status_code == 200


@pytest.mark.asyncio
async def test_auth_skipped_when_no_secret(mock_pipeline: MagicMock):
    """Auth is skipped in dev mode (empty SERVICE_SECRET)."""
    with patch.dict("os.environ", {"SERVICE_SECRET": ""}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post("/v1/synthesize", json={"text": "テスト"})
            assert resp.status_code == 200


@pytest.mark.asyncio
async def test_health_no_auth_required(mock_pipeline: MagicMock):
    """Health endpoint does not require authentication."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.get("/health")
            assert resp.status_code == 200


@pytest.mark.asyncio
async def test_voices_requires_auth(mock_pipeline: MagicMock):
    """Voices endpoint requires auth when SECRET is set."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.get("/v1/voices")
            assert resp.status_code == 401
