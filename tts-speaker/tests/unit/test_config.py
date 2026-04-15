"""Tests for configuration loading."""

from __future__ import annotations

from unittest.mock import patch

from tts_speaker.infra.config import Settings


def test_default_values():
    """Settings have sensible defaults."""
    with patch.dict("os.environ", {}, clear=True):
        s = Settings()
    assert s.host == "0.0.0.0"
    assert s.port == 9700
    assert s.default_voice == "jf_alpha"
    assert s.default_speed == 1.0
    assert s.log_level == "INFO"
    assert s.tts_max_stream_text_length == 30_000


def test_env_override():
    """Settings can be overridden via environment variables."""
    env = {
        "TTS_DEFAULT_VOICE": "jm_kumo",
        "TTS_DEFAULT_SPEED": "1.5",
        "LOG_LEVEL": "DEBUG",
    }
    with patch.dict("os.environ", env, clear=True):
        s = Settings()
    assert s.default_voice == "jm_kumo"
    assert s.default_speed == 1.5
    assert s.log_level == "DEBUG"


def test_speed_range():
    """Speed must be between 0.5 and 2.0."""
    with patch.dict("os.environ", {"TTS_DEFAULT_SPEED": "0.5"}, clear=True):
        s = Settings()
    assert s.default_speed == 0.5

    with patch.dict("os.environ", {"TTS_DEFAULT_SPEED": "2.0"}, clear=True):
        s = Settings()
    assert s.default_speed == 2.0


def test_tts_max_stream_text_length_env_override():
    """tts_max_stream_text_length can be overridden via environment variable."""
    with patch.dict("os.environ", {"TTS_MAX_STREAM_TEXT_LENGTH": "50000"}, clear=True):
        s = Settings()
    assert s.tts_max_stream_text_length == 50_000
