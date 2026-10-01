"""Tests for module-level application entrypoint in tts_speaker.main."""

import importlib
import json
import logging
import struct
import sys
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from tts_speaker.infra.inbound_tls import forget_tls_peer, remember_tls_peer


@pytest.fixture(autouse=True)
def clean_main_module():
    sys.modules.pop("tts_speaker.main", None)
    yield
    sys.modules.pop("tts_speaker.main", None)


def test_main_exits_on_missing_config(monkeypatch: pytest.MonkeyPatch) -> None:
    """Bad config ends the process with non-zero exit and clear log line."""
    monkeypatch.delenv("IRODORI_BASE_URL", raising=False)
    monkeypatch.delenv("IRODORI_API_KEY_FILE", raising=False)
    monkeypatch.delenv("TTS_VOICE_ID", raising=False)

    with pytest.raises(SystemExit) as exc_info:
        importlib.import_module("tts_speaker.main")

    assert exc_info.value.code == 1


def test_main_loads_app_with_valid_env(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """When valid env is set, tts_speaker.main defines module-level app."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "false")

    module = importlib.import_module("tts_speaker.main")
    assert hasattr(module, "app")
    assert module.app is not None

    client = TestClient(module.app, raise_server_exceptions=False)
    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.json() == {"status": "ok"}


def test_main_exits_on_strict_with_empty_allowlist(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    """When strict is true and allowed_peers is empty, process exits with status 1."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.delenv("MTLS_ALLOWED_PEERS", raising=False)

    with caplog.at_level(logging.ERROR), pytest.raises(SystemExit) as exc_info:
        importlib.import_module("tts_speaker.main")

    assert exc_info.value.code == 1
    assert any(
        record.levelname == "ERROR"
        and "PEER_IDENTITY_STRICT is enabled but MTLS_ALLOWED_PEERS is empty" in record.message
        for record in caplog.records
    )


def test_main_startup_log_strict_disabled(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Logs peer_identity_strict_disabled allowed=<comma list> on startup."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "false")
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "peer-a,peer-b")

    with caplog.at_level(logging.INFO):
        importlib.import_module("tts_speaker.main")

    assert any("peer_identity_strict_disabled allowed=peer-a,peer-b" in record.message for record in caplog.records)


def test_main_startup_log_strict_enabled(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Logs peer_identity_strict_enabled allowed=<comma list> on startup."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "peer-a,peer-b")

    with caplog.at_level(logging.INFO):
        importlib.import_module("tts_speaker.main")

    assert any("peer_identity_strict_enabled allowed=peer-a,peer-b" in record.message for record in caplog.records)


async def test_main_wires_default_speed_into_usecase(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
    sample_wav_bytes: bytes,
) -> None:
    """main wires settings.tts_default_speed into the usecase default_speed."""
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "false")
    monkeypatch.setenv("TTS_DEFAULT_SPEED", "1.4")

    module = importlib.import_module("tts_speaker.main")
    captured_speeds: list[float] = []

    async def fake_synthesize(text: str, speed: float) -> bytes:
        captured_speeds.append(speed)
        return sample_wav_bytes

    monkeypatch.setattr(module.gateway, "synthesize_chunk", fake_synthesize)
    chunks = [chunk async for chunk in module.usecase.stream("テスト")]
    assert len(chunks) == 1
    assert captured_speeds == [1.4]


def test_main_production_auth_via_in_process_tls_peer_path(
    dummy_api_key_file: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Production auth on real tts_speaker.main.app via in-process TLS peer path:
    - CN alt-butterfly-facade -> 200 application/connect+json
    - other CN -> 403
    - /health over TLS under strict -> 200
    """
    monkeypatch.setenv("IRODORI_BASE_URL", "http://localhost:8000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "speaker_01")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "alt-butterfly-facade")

    module = importlib.import_module("tts_speaker.main")

    async def fake_synthesize(text: str, speed: float) -> bytes:
        import io
        import wave

        b = io.BytesIO()
        with wave.open(b, "wb") as w:
            w.setnchannels(1)
            w.setsampwidth(2)
            w.setframerate(48000)
            w.writeframes(b"\0\0" * 4800)
        return b.getvalue()

    monkeypatch.setattr(module.gateway, "synthesize_chunk", fake_synthesize)

    client_addr = ("testclient", 50000)
    client = TestClient(module.app, client=client_addr, raise_server_exceptions=False)

    connect_payload = json.dumps({"text": "テスト"}).encode("utf-8")
    connect_frame = struct.pack(">BI", 0, len(connect_payload)) + connect_payload
    connect_headers = {
        "Content-Type": "application/connect+json",
        "Connect-Protocol-Version": "1",
    }

    try:
        # 1. Allowed CN alt-butterfly-facade -> 200 application/connect+json
        remember_tls_peer(client_addr, "alt-butterfly-facade")
        resp = client.post(
            "/alt.tts.v1.TTSService/SynthesizeStream",
            headers=connect_headers,
            content=connect_frame,
        )
        assert resp.status_code == 200
        assert resp.headers.get("content-type") == "application/connect+json"

        # 2. Other CN -> 403
        remember_tls_peer(client_addr, "impostor")
        resp = client.post(
            "/alt.tts.v1.TTSService/SynthesizeStream",
            headers=connect_headers,
            content=connect_frame,
        )
        assert resp.status_code == 403
        assert resp.text == "peer not allowlisted"

        # 3. /health over TLS under strict -> 200 (exempt from allowlist)
        resp = client.get("/health")
        assert resp.status_code == 200
        assert resp.json() == {"status": "ok"}
    finally:
        forget_tls_peer(client_addr)
