"""Tests for GET /health endpoint."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest
from httpx import AsyncClient


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


@pytest.mark.asyncio
async def test_health_not_ready(client: AsyncClient, mock_pipeline: MagicMock):
    """Health endpoint returns 503 when pipeline is not ready."""
    mock_pipeline.is_ready = False
    resp = await client.get("/health")
    assert resp.status_code == 503
    body = resp.json()
    assert body["status"] == "loading"
