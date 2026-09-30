"""Tests for module-level application entrypoint in tts_speaker.main."""

import importlib
import logging
import sys
from pathlib import Path

import pytest
from fastapi.testclient import TestClient


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
