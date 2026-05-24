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
    assert s.engine == "qwen"
    assert s.default_voice == "qwen-ja-1"
    assert s.default_speed == 1.0
    assert s.log_level == "INFO"
    assert s.tts_max_stream_text_length == 30_000
    assert s.qwen_model_id == "Qwen/Qwen3-TTS-12Hz-0.6B-CustomVoice"
    assert s.qwen_dtype == "bfloat16"
    assert s.qwen_attn_implementation == "sdpa"
    assert s.qwen_keepalive_interval_sec == 15.0


def test_env_override():
    """Settings can be overridden via environment variables."""
    env = {
        "TTS_DEFAULT_VOICE": "qwen-ja-2",
        "TTS_DEFAULT_SPEED": "1.5",
        "LOG_LEVEL": "DEBUG",
        "TTS_QWEN_MODEL_ID": "Qwen/Qwen3-TTS-12Hz-1.7B-CustomVoice",
        "TTS_QWEN_DTYPE": "float16",
        "TTS_QWEN_ATTN": "eager",
        "TTS_QWEN_KEEPALIVE_INTERVAL_SEC": "0",
    }
    with patch.dict("os.environ", env, clear=True):
        s = Settings()
    assert s.default_voice == "qwen-ja-2"
    assert s.default_speed == 1.5
    assert s.log_level == "DEBUG"
    assert s.qwen_model_id == "Qwen/Qwen3-TTS-12Hz-1.7B-CustomVoice"
    assert s.qwen_dtype == "float16"
    assert s.qwen_attn_implementation == "eager"
    assert s.qwen_keepalive_interval_sec == 0.0


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


def test_engine_supertonic_via_env():
    """TTS_ENGINE=supertonic is accepted by the Literal."""
    with patch.dict("os.environ", {"TTS_ENGINE": "supertonic"}, clear=True):
        s = Settings()
    assert s.engine == "supertonic"


def test_sup_total_steps_default_and_override():
    with patch.dict("os.environ", {}, clear=True):
        assert Settings().sup_total_steps == 8
    with patch.dict("os.environ", {"TTS_SUP_TOTAL_STEPS": "12"}, clear=True):
        assert Settings().sup_total_steps == 12
