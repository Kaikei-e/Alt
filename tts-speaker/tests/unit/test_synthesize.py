"""Tests for POST /v1/synthesize endpoint."""

from __future__ import annotations

import io
from unittest.mock import AsyncMock, MagicMock

import numpy as np
import pytest
from httpx import AsyncClient


@pytest.mark.asyncio
async def test_synthesize_success(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize returns WAV audio for valid text."""
    resp = await client.post("/v1/synthesize", json={"text": "テスト音声です。"})
    assert resp.status_code == 200
    assert resp.headers["content-type"] == "audio/wav"
    assert resp.headers["content-disposition"] == 'attachment; filename="speech.wav"'
    # WAV files start with RIFF header
    assert resp.content[:4] == b"RIFF"
    mock_pipeline.synthesize.assert_awaited_once()


@pytest.mark.asyncio
async def test_synthesize_with_voice_and_speed(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize respects voice and speed parameters."""
    resp = await client.post(
        "/v1/synthesize",
        json={"text": "テスト", "voice": "jm_kumo", "speed": 1.5},
    )
    assert resp.status_code == 200
    call_kwargs = mock_pipeline.synthesize.call_args
    assert call_kwargs[1]["voice"] == "jm_kumo"
    assert call_kwargs[1]["speed"] == 1.5


@pytest.mark.asyncio
async def test_synthesize_empty_text(client: AsyncClient):
    """Synthesize rejects empty text."""
    resp = await client.post("/v1/synthesize", json={"text": ""})
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_synthesize_text_too_long(client: AsyncClient):
    """Synthesize rejects text exceeding 5000 characters."""
    resp = await client.post("/v1/synthesize", json={"text": "あ" * 5001})
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_synthesize_speed_too_low(client: AsyncClient):
    """Synthesize rejects speed below 0.5."""
    resp = await client.post("/v1/synthesize", json={"text": "テスト", "speed": 0.3})
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_synthesize_speed_too_high(client: AsyncClient):
    """Synthesize rejects speed above 2.0."""
    resp = await client.post("/v1/synthesize", json={"text": "テスト", "speed": 2.5})
    assert resp.status_code == 422


@pytest.mark.asyncio
async def test_synthesize_pipeline_not_ready(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize returns 503 when pipeline is not loaded."""
    mock_pipeline.is_ready = False
    resp = await client.post("/v1/synthesize", json={"text": "テスト"})
    assert resp.status_code == 503
    assert resp.json()["detail"] == "TTS pipeline not ready"


@pytest.mark.asyncio
async def test_synthesize_pipeline_error(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize returns 500 on pipeline failure."""
    mock_pipeline.synthesize = AsyncMock(side_effect=RuntimeError("model error"))
    resp = await client.post("/v1/synthesize", json={"text": "テスト"})
    assert resp.status_code == 500
