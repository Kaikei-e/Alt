"""Tests for SupertonicEngine — voice catalogue, synth_one shape, lifecycle."""

from __future__ import annotations

from unittest.mock import MagicMock

import numpy as np
import pytest

from tts_speaker.core.engines.supertonic import (
    MODEL_NAME,
    SAMPLE_RATE,
    VOICE_IDS,
    VOICES,
    SupertonicEngine,
)
from tts_speaker.infra.config import Settings


def _loaded_engine() -> tuple[SupertonicEngine, MagicMock]:
    """Build a SupertonicEngine with a stub `_tts` and pre-resolved styles."""
    engine = SupertonicEngine(Settings())
    mock_tts = MagicMock()
    mock_audio = np.zeros((1, 4410), dtype=np.float32)
    mock_tts.synthesize.return_value = (mock_audio, np.array([0.1]))
    mock_tts.get_voice_style.return_value = MagicMock(name="Style")
    engine._tts = mock_tts
    engine._styles = {cfg_id: mock_tts.get_voice_style.return_value for cfg_id in VOICE_IDS}
    engine._ready = True
    return engine, mock_tts


class TestSupertonicVoiceCatalogue:
    def test_voices_shape(self):
        assert len(VOICES) == 1
        for v in VOICES:
            assert set(v) == {"id", "name", "gender"}

    def test_list_voices_advertises_only_sup_f4(self):
        # Legacy Qwen aliases are accepted by synth_one but not surfaced
        # through ListVoices — see LEGACY_ALIASES.
        assert [v["id"] for v in VOICES] == ["sup-F4"]

    def test_voice_ids_include_legacy_aliases(self):
        assert VOICE_IDS == {"sup-F4", "qwen-ja-1", "qwen-ja-2", "qwen-ja-3"}

    def test_resolve_voice_unknown_raises(self):
        with pytest.raises(ValueError, match="unknown voice"):
            SupertonicEngine._resolve_voice("sup-none")

    def test_resolve_voice_returns_config(self):
        cfg = SupertonicEngine._resolve_voice("sup-F4")
        assert cfg.id == "sup-F4"
        assert cfg.sup_voice == "F4"

    @pytest.mark.parametrize("alias", ["qwen-ja-1", "qwen-ja-2", "qwen-ja-3"])
    def test_legacy_qwen_alias_resolves_to_sup_f4(self, alias: str):
        cfg = SupertonicEngine._resolve_voice(alias)
        assert cfg.id == "sup-F4"
        assert cfg.sup_voice == "F4"


class TestSupertonicSynthOne:
    def test_synth_one_returns_float32_44100(self):
        engine, mock_tts = _loaded_engine()
        audio, sr = engine.synth_one(sentence="テスト。", voice="sup-F4", speed=1.0)
        assert audio.dtype == np.float32
        assert audio.ndim == 1  # flattened from (1, N) to (N,)
        assert sr == SAMPLE_RATE
        assert mock_tts.synthesize.call_count == 1

    def test_synth_one_passes_speed_and_total_steps(self):
        engine, mock_tts = _loaded_engine()
        engine.synth_one(sentence="テスト。", voice="sup-F4", speed=1.25)
        kwargs = mock_tts.synthesize.call_args.kwargs
        assert kwargs["speed"] == 1.25
        assert kwargs["total_steps"] == 8  # Settings default
        assert (
            kwargs["silence_duration"] == 0.05
        )  # Settings default — tight join for FE seamless playback
        assert mock_tts.synthesize.call_args.args[0] == "テスト。"

    def test_synth_one_unknown_voice_raises(self):
        engine, mock_tts = _loaded_engine()
        with pytest.raises(ValueError, match="unknown voice"):
            engine.synth_one(sentence="テスト。", voice="no-such", speed=1.0)
        mock_tts.synthesize.assert_not_called()

    def test_synth_one_before_load_raises(self):
        engine = SupertonicEngine(Settings())
        with pytest.raises(RuntimeError, match="not loaded"):
            engine.synth_one(sentence="テスト。", voice="sup-F4", speed=1.0)


class TestSupertonicEngineProperties:
    def test_name(self):
        engine = SupertonicEngine(Settings())
        assert engine.name == MODEL_NAME

    def test_device_is_cpu(self):
        engine = SupertonicEngine(Settings())
        assert engine.device == "cpu"
        assert engine.gpu_name is None

    def test_voices_and_voice_ids(self):
        engine = SupertonicEngine(Settings())
        assert engine.voices == VOICES
        assert engine.voice_ids == VOICE_IDS

    def test_is_ready_false_before_load(self):
        engine = SupertonicEngine(Settings())
        assert engine.is_ready is False

    @pytest.mark.asyncio
    async def test_keepalive_tick_is_noop(self):
        engine = SupertonicEngine(Settings())
        # Must not raise even when no model is loaded.
        await engine.keepalive_tick()

    def test_unload_resets_state(self):
        engine, _ = _loaded_engine()
        engine.unload()
        assert engine.is_ready is False
        assert engine._tts is None
        assert engine._styles == {}
