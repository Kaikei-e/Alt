"""Tests for application wiring, lifespan cleanup, and mTLS peer identity enforcement."""

import importlib
import json
import struct
import sys
import urllib.request
from collections.abc import AsyncIterator
from pathlib import Path
from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

from tts_speaker.app import create_app
from tts_speaker.infra.peer_identity import PeerIdentityMiddleware
from tts_speaker.infra.pki.ops import start_ops
from tts_speaker.usecase.synthesize_usecase import ChunkAudio, SynthesizeUsecase


def _encode_connect_frame(payload: dict) -> bytes:
    data = json.dumps(payload).encode("utf-8")
    return struct.pack(">BI", 0, len(data)) + data


def _decode_connect_frames(raw: bytes) -> list[tuple[int, dict]]:
    frames = []
    offset = 0
    while offset + 5 <= len(raw):
        flag, length = struct.unpack_from(">BI", raw, offset)
        offset += 5
        data = raw[offset : offset + length]
        offset += length
        frames.append((flag, json.loads(data.decode("utf-8"))))
    return frames


async def _async_chunks(chunks: list[ChunkAudio]) -> AsyncIterator[ChunkAudio]:
    for c in chunks:
        yield c


@pytest.fixture(autouse=True)
def clean_main_module():
    sys.modules.pop("tts_speaker.main", None)
    yield
    sys.modules.pop("tts_speaker.main", None)


@pytest.fixture
def mock_usecase(sample_wav_bytes: bytes) -> AsyncMock:
    usecase = AsyncMock(spec=SynthesizeUsecase)
    usecase.stream.return_value = _async_chunks(
        [ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1)]
    )
    return usecase


def test_main_peer_identity_strict_plaintext_returns_401(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """With PEER_IDENTITY_STRICT=true, a plaintext call to Connect service returns 401."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "alt-butterfly-facade")

    module = importlib.import_module("tts_speaker.main")
    client = TestClient(module.app, raise_server_exceptions=False)
    resp = client.post(
        "/alt.tts.v1.TTSService/SynthesizeStream",
        headers={"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"},
        content=_encode_connect_frame({"text": "hello"}),
    )
    assert resp.status_code == 401
    assert resp.text == "unauthenticated peer"


def test_main_lifespan_closes_client_on_shutdown(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """With TestClient(module.app): ... then module.client.is_closed is true."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "alt-butterfly-facade")

    module = importlib.import_module("tts_speaker.main")
    assert not module.client.is_closed

    with TestClient(module.app):
        assert not module.client.is_closed

    assert module.client.is_closed


def test_peer_identity_strict_plaintext_unauthenticated(
    mock_usecase: AsyncMock,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Under strict=True, plaintext Connect call without identity gets 401."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "off")
    app = create_app(usecase=mock_usecase)
    app.add_middleware(
        PeerIdentityMiddleware,
        allowed=["alt-butterfly-facade"],
        strict=True,
    )

    client = TestClient(app, raise_server_exceptions=False)
    resp = client.post(
        "/alt.tts.v1.TTSService/SynthesizeStream",
        headers={"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"},
        content=_encode_connect_frame({"text": "hello"}),
    )
    assert resp.status_code == 401
    assert resp.text == "unauthenticated peer"


def test_peer_identity_strict_plaintext_health_behavior(
    mock_usecase: AsyncMock,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Plaintext GET /health on strict listener returns 401 because PeerIdentityMiddleware
    does not exempt /health on plaintext callers (only on TLS callers via _TLS_ALLOWLIST_EXEMPT_PATH_PREFIXES).
    """
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "off")
    app = create_app(usecase=mock_usecase)
    app.add_middleware(
        PeerIdentityMiddleware,
        allowed=["alt-butterfly-facade"],
        strict=True,
    )

    client = TestClient(app, raise_server_exceptions=False)
    resp = client.get("/health")
    assert resp.status_code == 401
    assert resp.text == "unauthenticated peer"


def test_peer_identity_strict_with_allowed_peer_from_sidecar(
    mock_usecase: AsyncMock,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Under strict=True, request from sidecar (loopback + trusted) with allowed peer reaches the app."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "on")
    app = create_app(usecase=mock_usecase)
    app.add_middleware(
        PeerIdentityMiddleware,
        allowed=["alt-butterfly-facade"],
        strict=True,
    )

    with TestClient(app, client=("127.0.0.1", 50000)) as client:
        resp = client.post(
            "/alt.tts.v1.TTSService/SynthesizeStream",
            content=_encode_connect_frame({"text": "hello"}),
            headers={
                "x-alt-peer-identity": "alt-butterfly-facade",
                "Content-Type": "application/connect+json",
                "Connect-Protocol-Version": "1",
            },
        )
        assert resp.status_code == 200
        assert resp.headers["content-type"] == "application/connect+json"


def test_ops_listener_health_endpoint() -> None:
    """Dedicated loopback ops listener returns 200 for health checks under strict mode."""
    handle = start_ops("tts-speaker", None, listen="127.0.0.1:0")
    try:
        url = f"http://{handle.addr}/health"
        with urllib.request.urlopen(url, timeout=2.0) as resp:  # noqa: S310 # nosec B310
            assert resp.status == 200
            body = resp.read().decode("utf-8")
            assert '"status": "healthy"' in body
            assert '"service": "tts-speaker"' in body
    finally:
        handle.aclose_sync()


def test_in_process_connect_streaming_3_chunks(sample_wav_bytes: bytes) -> None:
    """In-process Connect client test: 3-chunk text yields 3 audio messages and clean end-of-stream."""
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    mock_usecase.stream.return_value = _async_chunks(
        [
            ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1),
            ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1),
            ChunkAudio(wav=sample_wav_bytes, sample_rate=48000, duration_seconds=0.1),
        ]
    )

    app = create_app(usecase=mock_usecase)
    client = TestClient(app)

    resp = client.post(
        "/alt.tts.v1.TTSService/SynthesizeStream",
        headers={"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"},
        content=_encode_connect_frame({"text": "文1。文2。文3。"}),
    )

    assert resp.status_code == 200
    assert resp.headers.get("content-type") == "application/connect+json"

    frames = _decode_connect_frames(resp.content)
    # 3 data frames (flag 0) + 1 end-of-stream frame (flag 2)
    data_frames = [f for f in frames if f[0] == 0]
    end_frames = [f for f in frames if f[0] == 2]

    assert len(data_frames) == 3
    for _, payload in data_frames:
        assert "audioWav" in payload
        assert payload.get("sampleRate") == 48000
        assert payload.get("durationSeconds") == pytest.approx(0.1, abs=0.01)

    assert len(end_frames) == 1
    assert "error" not in end_frames[0][1]


def test_in_process_connect_streaming_invalid_speed_yields_invalid_argument() -> None:
    """In-process Connect client test: speed outside 0.5..1.5 yields ConnectError invalid_argument."""
    mock_usecase = AsyncMock(spec=SynthesizeUsecase)
    app = create_app(usecase=mock_usecase)
    client = TestClient(app)

    resp = client.post(
        "/alt.tts.v1.TTSService/SynthesizeStream",
        headers={"Content-Type": "application/connect+json", "Connect-Protocol-Version": "1"},
        content=_encode_connect_frame({"text": "test", "speed": 2.0}),
    )

    assert resp.status_code == 200
    assert resp.headers.get("content-type") == "application/connect+json"

    frames = _decode_connect_frames(resp.content)
    assert len(frames) == 1
    flag, payload = frames[0]
    assert flag == 2  # end-of-stream frame
    assert "error" in payload
    assert payload["error"].get("code") == "invalid_argument"
