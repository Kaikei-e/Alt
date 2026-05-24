"""Tests for `core.factory.build_engine` engine selection."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import pytest

from tts_speaker.core.engines.qwen import QwenEngine
from tts_speaker.core.engines.supertonic import SupertonicEngine
from tts_speaker.core.factory import build_engine
from tts_speaker.infra.config import Settings


def test_build_engine_qwen_default():
    """Default settings select the Qwen engine."""
    with patch.object(QwenEngine, "_detect_device", return_value=("cpu", None)):
        engine = build_engine(Settings())
    assert isinstance(engine, QwenEngine)


def test_build_engine_supertonic():
    """TTS_ENGINE=supertonic selects the Supertonic engine."""
    settings = MagicMock(spec=Settings)
    settings.engine = "supertonic"
    engine = build_engine(settings)
    assert isinstance(engine, SupertonicEngine)


def test_build_engine_unknown_raises():
    """An engine name outside the Literal raises ValueError at selection time."""
    settings = MagicMock(spec=Settings)
    settings.engine = "not-an-engine"
    with pytest.raises(ValueError, match="unknown TTS engine"):
        build_engine(settings)
