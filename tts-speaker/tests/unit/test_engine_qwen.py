"""Tests for QwenEngine — voice resolution, speed-hint mapping, synth_one shape."""

from __future__ import annotations

from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from tts_speaker.core.engines.qwen import (
    MODEL_NAME,
    VOICE_IDS,
    VOICES,
    QwenEngine,
    VoiceConfig,
)
from tts_speaker.infra.config import Settings


def _engine_with_mock_model() -> tuple[QwenEngine, MagicMock]:
    """Construct a QwenEngine bypassing _detect_device + with a stub model."""
    with patch.object(QwenEngine, "_detect_device", return_value=("cpu", None)):
        engine = QwenEngine(Settings())
    mock_model = MagicMock()
    mock_audio = np.zeros(100, dtype=np.float32)
    mock_model.generate_custom_voice.return_value = ([mock_audio], 24000)
    engine._model = mock_model
    engine._ready = True
    return engine, mock_model


class TestQwenVoiceCatalogue:
    def test_voices_shape(self):
        assert len(VOICES) == 3
        for v in VOICES:
            assert set(v) == {"id", "name", "gender"}

    def test_voice_ids_match(self):
        assert VOICE_IDS == {"qwen-ja-1", "qwen-ja-2", "qwen-ja-3"}

    def test_resolve_voice_unknown_raises(self):
        with pytest.raises(ValueError, match="unknown voice"):
            QwenEngine._resolve_voice("no-such-voice")

    def test_resolve_voice_returns_config(self):
        cfg = QwenEngine._resolve_voice("qwen-ja-2")
        assert isinstance(cfg, VoiceConfig)
        assert cfg.id == "qwen-ja-2"
        assert cfg.qwen_speaker


class TestQwenSpeedHint:
    def test_slow_appends_yukkuri(self):
        out = QwenEngine._apply_speed_hint("base", 0.8)
        assert "ゆっくり" in out

    def test_fast_appends_hayame(self):
        out = QwenEngine._apply_speed_hint("base", 1.25)
        assert "速め" in out

    def test_neutral_leaves_base_unchanged(self):
        out = QwenEngine._apply_speed_hint("base", 1.0)
        assert out == "base"


class TestQwenSynthOne:
    def test_synth_one_calls_generate_custom_voice(self):
        engine, model = _engine_with_mock_model()
        audio, sr = engine.synth_one(sentence="テスト文。", voice="qwen-ja-1", speed=1.0)

        assert model.generate_custom_voice.call_count == 1
        assert sr == 24000
        assert audio.dtype == np.float32

    def test_synth_one_unknown_voice_raises(self):
        engine, model = _engine_with_mock_model()
        with pytest.raises(ValueError, match="unknown voice"):
            engine.synth_one(sentence="テスト文。", voice="no-such-voice", speed=1.0)
        model.generate_custom_voice.assert_not_called()

    def test_synth_one_passes_speaker_and_language(self):
        engine, model = _engine_with_mock_model()
        engine.synth_one(sentence="テスト文。", voice="qwen-ja-2", speed=1.0)

        kwargs = model.generate_custom_voice.call_args.kwargs
        assert kwargs["text"] == "テスト文。"
        # Qwen3-TTS get_supported_languages() returns lowercase names.
        assert kwargs["language"] == "japanese"
        assert kwargs["speaker"]
        assert kwargs["instruct"]

    def test_synth_one_speed_hint_slow(self):
        engine, model = _engine_with_mock_model()
        engine.synth_one(sentence="テスト文。", voice="qwen-ja-1", speed=0.8)
        assert "ゆっくり" in model.generate_custom_voice.call_args.kwargs["instruct"]

    def test_synth_one_speed_hint_fast(self):
        engine, model = _engine_with_mock_model()
        engine.synth_one(sentence="テスト文。", voice="qwen-ja-1", speed=1.25)
        assert "速め" in model.generate_custom_voice.call_args.kwargs["instruct"]

    def test_synth_one_before_load_raises(self):
        with patch.object(QwenEngine, "_detect_device", return_value=("cpu", None)):
            engine = QwenEngine(Settings())
        with pytest.raises(RuntimeError, match="not loaded"):
            engine.synth_one(sentence="テスト", voice="qwen-ja-1", speed=1.0)


class TestQwenEngineProperties:
    def test_name(self):
        with patch.object(QwenEngine, "_detect_device", return_value=("cpu", None)):
            engine = QwenEngine(Settings())
        assert engine.name == MODEL_NAME

    def test_voice_ids_and_voices_exposed(self):
        with patch.object(QwenEngine, "_detect_device", return_value=("cpu", None)):
            engine = QwenEngine(Settings())
        assert engine.voice_ids == VOICE_IDS
        assert engine.voices == VOICES

    def test_device_reflects_detect(self):
        with patch.object(QwenEngine, "_detect_device", return_value=("cuda", "AMD GPU")):
            engine = QwenEngine(Settings())
        assert engine.device == "cuda"
        assert engine.gpu_name == "AMD GPU"
