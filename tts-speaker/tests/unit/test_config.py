"""Tests for configuration loading."""

from __future__ import annotations

import os
import tempfile
from unittest.mock import patch

import pytest

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
    assert s.service_secret == ""


def test_env_override():
    """Settings can be overridden via environment variables."""
    env = {
        "TTS_DEFAULT_VOICE": "jm_kumo",
        "TTS_DEFAULT_SPEED": "1.5",
        "LOG_LEVEL": "DEBUG",
        "SERVICE_SECRET": "my-secret",
    }
    with patch.dict("os.environ", env, clear=True):
        s = Settings()
    assert s.default_voice == "jm_kumo"
    assert s.default_speed == 1.5
    assert s.log_level == "DEBUG"
    assert s.service_secret == "my-secret"


def test_service_secret_file():
    """SERVICE_SECRET_FILE takes precedence over SERVICE_SECRET env var."""
    with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
        f.write("file-secret\n")
        f.flush()
        env = {
            "SERVICE_SECRET": "env-secret",
            "SERVICE_SECRET_FILE": f.name,
        }
        try:
            with patch.dict("os.environ", env, clear=True):
                s = Settings()
            assert s.service_secret == "file-secret"
        finally:
            os.unlink(f.name)


def test_speed_range():
    """Speed must be between 0.5 and 2.0."""
    with patch.dict("os.environ", {"TTS_DEFAULT_SPEED": "0.5"}, clear=True):
        s = Settings()
    assert s.default_speed == 0.5

    with patch.dict("os.environ", {"TTS_DEFAULT_SPEED": "2.0"}, clear=True):
        s = Settings()
    assert s.default_speed == 2.0
