"""Tests for GET /health endpoint."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest
from httpx import ASGITransport, AsyncClient

from tts_speaker.app.main import create_app


@pytest.mark.asyncio
async def test_health_ok(client: AsyncClient):
    """Health endpoint returns 200 with model info when pipeline is ready."""
    resp = await client.get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"
    assert body["model"] == "kokoro-82m"
    assert body["lang"] == "ja"
    assert body["device"] == "cpu"
    assert "gpu_name" not in body


@pytest.mark.asyncio
async def test_health_not_ready(client: AsyncClient, mock_pipeline: MagicMock):
    """Health endpoint returns 503 when pipeline is not ready."""
    mock_pipeline.is_ready = False
    resp = await client.get("/health")
    assert resp.status_code == 503
    body = resp.json()
    assert body["status"] == "loading"


@pytest.mark.asyncio
async def test_health_with_gpu(mock_pipeline: MagicMock):
    """Health endpoint includes gpu_name when GPU is active."""
    mock_pipeline._device = "cuda"
    mock_pipeline._gpu_name = "AMD Radeon 890M"

    with patch.dict("os.environ", {"SERVICE_SECRET": ""}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)

    transport = ASGITransport(app=app)
    async with AsyncClient(transport=transport, base_url="http://test") as client:
        resp = await client.get("/health")

    assert resp.status_code == 200
    body = resp.json()
    assert body["device"] == "cuda"
    assert body["gpu_name"] == "AMD Radeon 890M"
