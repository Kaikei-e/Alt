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
        peer_identity_strict=False,
    )
    assert str(settings.irodori_base_url).rstrip("/") == "http://localhost:8000"
    assert settings.irodori_api_key.get_secret_value() == "test-secret-key-12345"
    assert settings.irodori_model_name == "irodori-tts"
    assert settings.irodori_request_timeout_seconds == 330.0
    assert settings.irodori_max_attempts == 3
    assert settings.irodori_retry_backoff_seconds == 1.0
    assert settings.tts_voice_id == "speaker_01"
    assert settings.peer_identity_strict is False
    assert settings.tts_max_chunk_chars == 60
    assert settings.tts_max_text_chars == 5000
    assert settings.tts_chunk_gap_ms == 200
    assert settings.log_level == "INFO"
    assert settings.tts_queue_timeout_seconds == 600.0
    assert "irodori_api_key" not in settings.model_dump()


def test_missing_irodori_base_url(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
        )


def test_invalid_irodori_base_url(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="not-a-valid-http-url",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
        )


def test_missing_api_key_file() -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
        )


def test_nonexistent_api_key_file(tmp_path: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=tmp_path / "does_not_exist.txt",
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
        )


def test_empty_api_key_file(empty_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=empty_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
        )


@pytest.mark.parametrize("timeout", [0.0, -1.0])
def test_invalid_timeout(dummy_api_key_file: Path, timeout: float) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            irodori_request_timeout_seconds=timeout,
        )


@pytest.mark.parametrize("attempts", [0, -1, 6, 10])
def test_invalid_max_attempts(dummy_api_key_file: Path, attempts: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            irodori_max_attempts=attempts,
        )


def test_invalid_retry_backoff(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            irodori_retry_backoff_seconds=-0.1,
        )


@pytest.mark.parametrize("disallowed_voice", ["", "none", "NONE", "no-ref", "No-Ref", "text-only", "TEXT-ONLY"])
def test_disallowed_voice_ids(dummy_api_key_file: Path, disallowed_voice: str) -> None:
    with pytest.raises(ValidationError, match="(?i)reference voice"):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id=disallowed_voice,
            peer_identity_strict=False,
        )


@pytest.mark.parametrize("chunk_chars", [19, 201])
def test_invalid_max_chunk_chars(dummy_api_key_file: Path, chunk_chars: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            tts_max_chunk_chars=chunk_chars,
        )


@pytest.mark.parametrize("text_chars", [0, -1, 30001])
def test_invalid_max_text_chars(dummy_api_key_file: Path, text_chars: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            tts_max_text_chars=text_chars,
        )


@pytest.mark.parametrize("gap_ms", [-1, 2001])
def test_invalid_chunk_gap_ms(dummy_api_key_file: Path, gap_ms: int) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            tts_chunk_gap_ms=gap_ms,
        )


def test_missing_peer_identity_strict(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError) as exc_info:
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
        )
    assert "peer_identity_strict" in str(exc_info.value)


@pytest.mark.parametrize("invalid_val", ["not-a-bool", "2", "maybe"])
def test_invalid_peer_identity_strict(dummy_api_key_file: Path, invalid_val: str) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=invalid_val,  # type: ignore[arg-type]
        )


def test_env_loading(dummy_api_key_file: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("IRODORI_BASE_URL", "http://env-host:9000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "env_voice")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "true")

    settings = Settings()
    assert str(settings.irodori_base_url).rstrip("/") == "http://env-host:9000"
    assert settings.tts_voice_id == "env_voice"
    assert settings.peer_identity_strict is True


def test_voice_id_is_stripped(dummy_api_key_file: Path) -> None:
    settings = Settings(
        irodori_base_url="http://localhost:8000",
        irodori_api_key_file=dummy_api_key_file,
        tts_voice_id="  speaker_01  ",
        peer_identity_strict=False,
    )
    assert settings.tts_voice_id == "speaker_01"


def test_api_key_not_settable_from_env(dummy_api_key_file: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("IRODORI_API_KEY", "injected_hack_key")
    settings = Settings(
        irodori_base_url="http://localhost:8000",
        irodori_api_key_file=dummy_api_key_file,
        tts_voice_id="speaker_01",
        peer_identity_strict=False,
    )
    assert settings.irodori_api_key.get_secret_value() == "test-secret-key-12345"


def test_api_key_property_is_read_only(dummy_api_key_file: Path) -> None:
    settings = Settings(
        irodori_base_url="http://localhost:8000",
        irodori_api_key_file=dummy_api_key_file,
        tts_voice_id="speaker_01",
        peer_identity_strict=False,
    )
    with pytest.raises(AttributeError):
        settings.irodori_api_key = "new-key"  # type: ignore[misc]


def test_invalid_log_level(dummy_api_key_file: Path) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            log_level="VERBOSE",  # type: ignore[arg-type]
        )


@pytest.mark.parametrize("timeout", [0.0, -1.0])
def test_invalid_queue_timeout(dummy_api_key_file: Path, timeout: float) -> None:
    with pytest.raises(ValidationError):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            tts_queue_timeout_seconds=timeout,
        )


def test_default_tts_default_speed(dummy_api_key_file: Path) -> None:
    settings = Settings(
        irodori_base_url="http://localhost:8000",
        irodori_api_key_file=dummy_api_key_file,
        tts_voice_id="speaker_01",
        peer_identity_strict=False,
    )
    assert getattr(settings, "tts_default_speed", None) == 1.25


def test_env_loading_tts_default_speed(dummy_api_key_file: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("IRODORI_BASE_URL", "http://env-host:9000")
    monkeypatch.setenv("IRODORI_API_KEY_FILE", str(dummy_api_key_file))
    monkeypatch.setenv("TTS_VOICE_ID", "env_voice")
    monkeypatch.setenv("PEER_IDENTITY_STRICT", "false")
    monkeypatch.setenv("TTS_DEFAULT_SPEED", "1.4")

    settings = Settings()
    assert getattr(settings, "tts_default_speed", None) == 1.4


@pytest.mark.parametrize("speed", [0.4, 0.49, 1.51, 1.6])
def test_invalid_tts_default_speed(dummy_api_key_file: Path, speed: float) -> None:
    with pytest.raises(
        ValidationError,
        match=r"(?i)greater_than_equal|less_than_equal|greater than or equal|less than or equal",
    ):
        Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            tts_default_speed=speed,
        )


@pytest.mark.parametrize("speed", [0.5, 1.5])
def test_valid_tts_default_speed_bounds(dummy_api_key_file: Path, speed: float) -> None:
    try:
        settings = Settings(
            irodori_base_url="http://localhost:8000",
            irodori_api_key_file=dummy_api_key_file,
            tts_voice_id="speaker_01",
            peer_identity_strict=False,
            tts_default_speed=speed,
        )
    except ValidationError:
        pytest.fail(f"Valid speed {speed} was rejected by Settings validation")
    assert getattr(settings, "tts_default_speed", None) == speed
