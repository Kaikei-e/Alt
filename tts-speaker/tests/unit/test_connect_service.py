"""Tests for connect-rpc TTSService implementation."""

from __future__ import annotations

from unittest.mock import AsyncMock, MagicMock, patch

import numpy as np
import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError
from httpx import ASGITransport, AsyncClient

from tts_speaker.app.main import create_app
from tts_speaker.core.pipeline import TTSPipeline
from tts_speaker.core.preprocess import preprocess_for_tts


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


@pytest.mark.asyncio
async def test_synthesize_preprocesses_english(client: AsyncClient, mock_pipeline: MagicMock):
    """Synthesize applies English->Katakana preprocessing before passing to pipeline."""
    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": "APIのニュース"},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code == 200
    call_kwargs = mock_pipeline.synthesize.call_args
    # "API" should be expanded to "エーピーアイ" before reaching the pipeline
    assert "エーピーアイ" in call_kwargs[1]["text"]
    assert "API" not in call_kwargs[1]["text"]


@pytest.mark.asyncio
async def test_synthesize_stream_accepts_long_text(mock_pipeline: MagicMock):
    """Synthesize stream accepts text between 5001 and 30000 characters."""
    from tts_speaker.app.connect_service import TTSConnectService
    from tts_speaker.infra.config import Settings

    async def fake_stream(**kwargs):
        yield np.zeros(2400, dtype=np.float32)

    mock_pipeline.synthesize_stream = MagicMock(side_effect=fake_stream)

    service = TTSConnectService(mock_pipeline, Settings())
    request = MagicMock()
    request.text = "あ" * 10000  # Well over 5000 but under 30000
    request.voice = ""
    request.speed = 0.0
    ctx = MagicMock()
    ctx.request_headers.return_value = {}

    chunks = []
    async for chunk in service.synthesize_stream(request, ctx):
        chunks.append(chunk)

    assert len(chunks) > 0
    mock_pipeline.synthesize_stream.assert_called_once()


@pytest.mark.asyncio
async def test_synthesize_stream_rejects_over_limit(mock_pipeline: MagicMock):
    """Synthesize stream rejects text exceeding 30000 characters."""
    from tts_speaker.app.connect_service import TTSConnectService
    from tts_speaker.infra.config import Settings

    service = TTSConnectService(mock_pipeline, Settings())
    request = MagicMock()
    request.text = "あ" * 30001
    request.voice = ""
    request.speed = 0.0
    ctx = MagicMock()
    ctx.request_headers.return_value = {}

    with pytest.raises(ConnectError) as exc_info:
        async for _ in service.synthesize_stream(request, ctx):
            pass

    assert exc_info.value.code == Code.INVALID_ARGUMENT


@pytest.mark.asyncio
async def test_synthesize_stream_long_text_preprocessed(mock_pipeline: MagicMock):
    """Synthesize stream correctly processes long preprocessed text."""
    from tts_speaker.app.connect_service import TTSConnectService
    from tts_speaker.infra.config import Settings

    async def fake_stream(**kwargs):
        yield np.zeros(2400, dtype=np.float32)

    mock_pipeline.synthesize_stream = MagicMock(side_effect=fake_stream)

    service = TTSConnectService(mock_pipeline, Settings())
    # Use text that stays under 30000 after preprocessing
    request = MagicMock()
    request.text = "テスト。" * 2000  # 8000 chars, all Japanese, no expansion
    request.voice = ""
    request.speed = 0.0
    ctx = MagicMock()
    ctx.request_headers.return_value = {}

    chunks = []
    async for chunk in service.synthesize_stream(request, ctx):
        chunks.append(chunk)

    assert len(chunks) > 0
    call_kwargs = mock_pipeline.synthesize_stream.call_args
    assert len(call_kwargs[1]["text"]) > 5000


@pytest.mark.asyncio
async def test_synthesize_stream_preprocesses_english(mock_pipeline: MagicMock):
    """Synthesize stream also applies English->Katakana preprocessing."""
    from tts_speaker.app.connect_service import TTSConnectService
    from tts_speaker.infra.config import Settings

    async def fake_stream(**kwargs):
        yield np.zeros(2400, dtype=np.float32)

    mock_pipeline.synthesize_stream = MagicMock(side_effect=fake_stream)

    service = TTSConnectService(mock_pipeline, Settings())
    request = MagicMock()
    request.text = "RSSリーダー"
    request.voice = ""
    request.speed = 0.0
    ctx = MagicMock()
    ctx.request_headers.return_value = {}

    # Consume the async generator
    chunks = []
    async for chunk in service.synthesize_stream(request, ctx):
        chunks.append(chunk)

    call_kwargs = mock_pipeline.synthesize_stream.call_args
    assert "アールエスエス" in call_kwargs[1]["text"]
    assert "RSS" not in call_kwargs[1]["text"]


@pytest.mark.asyncio
async def test_synthesize_length_validated_after_preprocessing(client: AsyncClient):
    """Text length is validated AFTER preprocessing, not before.

    A string of acronyms that is under 5000 chars raw but expands
    beyond 5000 chars after preprocessing should be rejected.
    """
    # Each 2-letter acronym like "AB" expands to ~4 katakana chars
    # We need raw text < 5000 but expanded > 5000
    # "AA " (3 chars) -> "エーエー " (5 chars) => expansion ratio ~1.67x
    # Use 3-letter acronyms: "AAA " (4 chars) -> "エーエーエー " (7 chars) => 1.75x
    # Need expanded > 5000, so raw > 5000/1.75 ≈ 2857
    # Use "AAA " * 1000 = 4000 chars raw -> ~7000 chars expanded
    text = " ".join(["AAA"] * 1000)
    assert len(text) < 5000  # raw is under limit
    expanded = preprocess_for_tts(text)
    assert len(expanded) > 5000  # expanded exceeds limit

    resp = await client.post(
        "/alt.tts.v1.TTSService/Synthesize",
        json={"text": text},
        headers={"Content-Type": "application/json"},
    )
    assert resp.status_code != 200
