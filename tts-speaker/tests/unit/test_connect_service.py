"""Tests for connect-rpc TTSService implementation."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest
from httpx import ASGITransport, AsyncClient

from tts_speaker.app.main import create_app
from tts_speaker.core.pipeline import TTSPipeline


@pytest.mark.asyncio
async def test_synthesize_success(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize returns WAV audio bytes for valid text."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        content=b'{"text": "\xe3\x83\x86\xe3\x82\xb9\xe3\x83\x88\xe9\x9f\xb3\xe5\xa3\xb0\xe3\x81\xa7\xe3\x81\x99\xe3\x80\x82"}',
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200
    body = resp.json()
    assert "audioWav" in body
    assert body["sampleRate"] == 24000
    assert body["durationSeconds"] > 0
    mock_pipeline.synthesize.assert_awaited_once()


@pytest.mark.asyncio
async def test_synthesize_with_voice_and_speed(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize respects voice and speed parameters."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "テスト", "voice": "jm_kumo", "speed": 1.5},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200
    call_kwargs = mock_pipeline.synthesize.call_args
    assert call_kwargs[1]["voice"] == "jm_kumo"
    assert call_kwargs[1]["speed"] == 1.5


@pytest.mark.asyncio
async def test_synthesize_empty_text(client: AsyncClient):
    """Synthesize rejects empty text."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": ""},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200


@pytest.mark.asyncio
async def test_synthesize_text_too_long(client: AsyncClient):
    """Synthesize rejects text exceeding 5000 characters."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "あ" * 5001},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200


@pytest.mark.asyncio
async def test_synthesize_pipeline_not_ready(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize returns error when pipeline is not loaded."""
    mock_pipeline.is_ready = False
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "テスト"},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200


@pytest.mark.asyncio
async def test_synthesize_pipeline_error(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize returns error on pipeline failure."""
    mock_pipeline.synthesize = AsyncMock(side_effect=RuntimeError("model error"))
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "テスト"},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200


@pytest.mark.asyncio
async def test_synthesize_unknown_voice(client: AsyncClient):
    """Synthesize rejects unknown voice ID."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "テスト", "voice": "unknown_voice"},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200


@pytest.mark.asyncio
async def test_synthesize_speed_out_of_range(client: AsyncClient):
    """Synthesize rejects speed outside 0.5-2.0 range."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "テスト", "speed": 3.0},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200


@pytest.mark.asyncio
async def test_list_voices_returns_all(client: AsyncClient):
    """ListVoices returns list of 5 Japanese voices."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/ListVoices",
        json={},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200
    body = resp.json()
    assert len(body["voices"]) == 5


@pytest.mark.asyncio
async def test_list_voices_structure(client: AsyncClient):
    """Each voice has id, name, and gender fields."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/ListVoices",
        json={},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200
    for voice in resp.json()["voices"]:
        assert "id" in voice
        assert "name" in voice
        assert "gender" in voice


@pytest.mark.asyncio
async def test_list_voices_ids(client: AsyncClient):
    """ListVoices returns expected voice IDs."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/ListVoices",
        json={},
        headers={"Content-Type": "application/json"},
    )
    voice_ids = [v["id"] for v in resp.json()["voices"]]
    assert "jf_alpha" in voice_ids
    assert "jf_gongitsune" in voice_ids
    assert "jf_nezumi" in voice_ids
    assert "jf_tebukuro" in voice_ids
    assert "jm_kumo" in voice_ids


@pytest.mark.asyncio
async def test_auth_required_when_secret_set(mock_pipeline: MagicMock):
    """Connect-RPC endpoints reject requests without token when SECRET is set."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post(
                "/alt.tts.v1.TTSService/Synthesize",
                json={"text": "テスト"},
                headers={"Content-Type": "application/json"},
            )
            assert resp.status_code != 200


@pytest.mark.asyncio
async def test_auth_correct_token(mock_pipeline: MagicMock):
    """Connect-RPC endpoints accept requests with correct token."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post(
                "/alt.tts.v1.TTSService/Synthesize",
                json={"text": "テスト"},
                headers={
                    "Content-Type": "application/json",
                    "X-Service-Token": "test-secret",
                },
            )
            assert resp.status_code == 200


@pytest.mark.asyncio
async def test_auth_wrong_token(mock_pipeline: MagicMock):
    """Connect-RPC endpoints reject requests with wrong token."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.post(
                "/alt.tts.v1.TTSService/Synthesize",
                json={"text": "テスト"},
                headers={
                    "Content-Type": "application/json",
                    "X-Service-Token": "wrong-token",
                },
            )
            assert resp.status_code != 200


@pytest.mark.asyncio
async def test_health_no_auth_required(mock_pipeline: MagicMock):
    """Health endpoint does not require authentication."""
    with patch.dict("os.environ", {"SERVICE_SECRET": "test-secret"}, clear=False):
        app = create_app(pipeline_override=mock_pipeline)
        transport = ASGITransport(app=app)
        async with AsyncClient(transport=transport, base_url="http://test") as ac:
            resp = await ac.get("/health")
            assert resp.status_code == 200
