"""Tests for application wiring, lifespan cleanup, and mTLS peer identity enforcement."""

import importlib
import sys
import urllib.request
from pathlib import Path
from unittest.mock import AsyncMock

import pytest
from fastapi.testclient import TestClient

from tts_speaker.app import create_app
from tts_speaker.infra.peer_identity import PeerIdentityMiddleware
from tts_speaker.infra.pki.ops import start_ops
from tts_speaker.usecase.synthesize_usecase import SynthesisResult, SynthesizeUsecase


@pytest.fixture(autouse=True)
def clean_main_module():
    sys.modules.pop("tts_speaker.main", None)
    yield
    sys.modules.pop("tts_speaker.main", None)


@pytest.fixture
def mock_usecase(sample_wav_bytes: bytes) -> AsyncMock:
    usecase = AsyncMock(spec=SynthesizeUsecase)
    usecase.execute.return_value = SynthesisResult(
        wav=sample_wav_bytes,
        chunk_count=1,
        duration_seconds=0.1,
    )
    return usecase


def test_main_peer_identity_strict_plaintext_returns_401(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """With PEER_IDENTITY_STRICT=true, a plaintext POST /v1/synthesize returns 401."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "alt-butterfly-facade")

    module = importlib.import_module("tts_speaker.main")
    client = TestClient(module.app, raise_server_exceptions=False)
    resp = client.post("/v1/synthesize", json={"text": "hello"})
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
    """Under strict=True, plaintext POST /v1/synthesize without identity gets 401."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "off")
    app = create_app(usecase=mock_usecase)
    app.add_middleware(
        PeerIdentityMiddleware,
        allowed=["alt-butterfly-facade"],
        strict=True,
    )

    client = TestClient(app, raise_server_exceptions=False)
    resp = client.post("/v1/synthesize", json={"text": "hello"})
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
    """Under strict=True, request from sidecar (loopback + trusted) with allowed peer succeeds."""
    monkeypatch.setenv("PEER_IDENTITY_TRUSTED", "on")
    app = create_app(usecase=mock_usecase)
    app.add_middleware(
        PeerIdentityMiddleware,
        allowed=["alt-butterfly-facade"],
        strict=True,
    )

    with TestClient(app, client=("127.0.0.1", 50000)) as client:
        resp = client.post(
            "/v1/synthesize",
            json={"text": "hello"},
            headers={"x-alt-peer-identity": "alt-butterfly-facade"},
        )
        assert resp.status_code == 200
        assert resp.headers["content-type"] == "audio/wav"


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
