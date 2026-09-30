"""Unit tests for Settings."""

from pathlib import Path

import pytest
from pydantic import ValidationError

from tts_speaker.config.settings import Settings


def test_valid_settings_with_defaults(dummy_api_key_file: Path) -> None:
    settings = Settings(
        irodori_base_url="http://localhost:8000",
        irodori_api_key_file=dummy_api_key_file,
        tts_voice_id="speaker_01",
    )
    assert str(settings.irodori_base_url).rstrip("/") == "http://localhost:8000"
    assert settings.irodori_api_key.get_secret_value() == "test-secret-key-12345"
    assert settings.irodori_model_name == "irodori-tts"
    assert settings.irodori_request_timeout_seconds == 330.0
    assert settings.irodori_max_attempts == 3
    assert settings.irodori_retry_backoff_seconds == 1.0
    assert settings.tts_voice_id == "speaker_01"
    assert settings.tts_max_chunk_chars == 100
    assert settings.tts_max_text_chars == 5000
    assert settings.tts_chunk_gap_ms == 200
    assert settings.host == "0.0.0.0"
    assert settings.port == 9700
    assert settings.log_level == "INFO"


def test_missing_irodori_base_url(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
        )


def test_invalid_irodori_base_url(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="not-a-valid-http-url",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
        )


def test_missing_api_key_file() -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            tts_voice_id="speaker_01",
        )


def test_nonexistent_api_key_file(tmp_path: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=tmp_path / "does_not_exist.txt",
            tts_voice_id="speaker_01",
        )


def test_empty_api_key_file(empty_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=empty_api_key_file,
            tts_voice_id="speaker_01",
        )


@pytest.mark.parametrize("timeout", [0.0, -1.0])
def test_invalid_timeout(dummy_api_key_file: Path, timeout: float) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            irodori_request_timeout_seconds=timeout,
        )


@pytest.mark.parametrize("attempts", [0, -1, 6, 10])
def test_invalid_max_attempts(dummy_api_key_file: Path, attempts: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            irodori_max_attempts=attempts,
        )


def test_invalid_retry_backoff(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            irodori_retry_backoff_seconds=-0.1,
        )


@pytest.mark.parametrize("disallowed_voice", ["", "none", "NONE", "no-ref", "No-Ref", "text-only", "TEXT-ONLY"])
def test_disallowed_voice_ids(dummy_api_key_file: Path, disallowed_voice: str) -> None:
    with pytest.raises(ValidationError, match="(?i)reference voice"):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id=disallowed_voice,
        )


@pytest.mark.parametrize("chunk_chars", [19, 201])
def test_invalid_max_chunk_chars(dummy_api_key_file: Path, chunk_chars: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            tts_max_chunk_chars=chunk_chars,
        )


@pytest.mark.parametrize("text_chars", [0, -1, 30001])
def test_invalid_max_text_chars(dummy_api_key_file: Path, text_chars: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            tts_max_text_chars=text_chars,
        )


@pytest.mark.parametrize("gap_ms", [-1, 2001])
def test_invalid_chunk_gap_ms(dummy_api_key_file: Path, gap_ms: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            tts_chunk_gap_ms=gap_ms,
        )


def test_env_loading(dummy_api_key_file: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("IRODORI_BASE_URL", "http://env-host:9000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "env_voice")
    monkeypatch.setenv("PORT", "9750")

    settings = Settings()
    assert str(settings.irodori_base_url).rstrip("/") == "http://env-host:9000"
    assert settings.tts_voice_id == "env_voice"
    assert settings.port == 9750
