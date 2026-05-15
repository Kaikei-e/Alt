"""Tests for Japanese split pattern in TTSPipeline."""

from __future__ import annotations

import re
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from tts_speaker.core.pipeline import (
    COMMA_SPLIT_THRESHOLD_CHARS,
    JAPANESE_SPLIT_PATTERN,
    TTSPipeline,
)


class TestJapaneseSplitPattern:
    """Verify JAPANESE_SPLIT_PATTERN regex behavior."""

    def test_splits_on_maru(self):
        """Splits on 。 (full stop)."""
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "最初の文。次の文。") if s]
        assert result == ["最初の文。", "次の文。"]

    def test_splits_on_exclamation(self):
        """Splits on ！ (exclamation mark)."""
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "すごい！本当に！") if s]
        assert result == ["すごい！", "本当に！"]

    def test_splits_on_question(self):
        """Splits on ？ (question mark)."""
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "何ですか？分かりません。") if s]
        assert result == ["何ですか？", "分かりません。"]

    def test_splits_on_newline(self):
        """Splits on newline."""
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "一行目\n二行目\n") if s]
        assert result == ["一行目\n", "二行目\n"]

    def test_no_split_on_touten(self):
        """Does NOT split on 、 (comma)."""
        text = "りんご、みかん、ぶどう"
        result = re.split(JAPANESE_SPLIT_PATTERN, text)
        assert result == [text]

    def test_no_split_on_ascii_period(self):
        """Does NOT split on ASCII period."""
        text = "version 1.0 released"
        result = re.split(JAPANESE_SPLIT_PATTERN, text)
        assert result == [text]

    def test_punctuation_stays_attached(self):
        """Punctuation remains attached to the preceding segment (lookbehind)."""
        result = re.split(JAPANESE_SPLIT_PATTERN, "前半。後半。")
        for segment in result:
            assert not segment.startswith("。")


class TestCommaLevelFallback:
    """Verify _split_sentences applies 「、」 fallback above the char threshold."""

    def test_short_sentence_with_comma_not_split(self):
        """Below threshold, `、` stays inside the sentence for natural prosody."""
        text = "りんご、みかん。"
        assert len(text) < COMMA_SPLIT_THRESHOLD_CHARS
        result = TTSPipeline._split_sentences(text)
        assert result == ["りんご、みかん。"]

    def test_long_sentence_with_comma_split_on_comma(self):
        """Above threshold, `、` becomes a chunk boundary."""
        text = (
            "今日は良い天気ですので散歩をしてから本屋に立ち寄ろうかと考えています、"
            "そのあとカフェでゆっくり読書したいです。"
        )
        assert len(text) >= COMMA_SPLIT_THRESHOLD_CHARS
        result = TTSPipeline._split_sentences(text)
        # First comma triggers split; the original 。 still ends the last chunk.
        assert len(result) >= 2
        assert any(seg.endswith("、") for seg in result)

    def test_long_sentence_without_comma_unchanged(self):
        """Long sentence with no `、` stays as one chunk."""
        text = "あ" * (COMMA_SPLIT_THRESHOLD_CHARS + 20) + "。"
        result = TTSPipeline._split_sentences(text)
        assert result == [text]

    def test_comma_split_preserves_punctuation_attachment(self):
        """Each `、`-level chunk keeps the comma attached to the preceding word."""
        text = "あ" * COMMA_SPLIT_THRESHOLD_CHARS + "、つづき。"
        result = TTSPipeline._split_sentences(text)
        assert len(result) >= 2
        assert result[0].endswith("、")
        assert not result[1].startswith("、")


def _make_pipeline_with_mock() -> tuple[TTSPipeline, MagicMock]:
    """Create a TTSPipeline with a mocked Qwen3TTSModel."""
    mock_model = MagicMock()
    mock_audio = np.zeros(100, dtype=np.float32)
    # generate_custom_voice returns (wavs: list[np.ndarray], sr: int)
    mock_model.generate_custom_voice.return_value = ([mock_audio], 24000)

    with patch.object(TTSPipeline, "__init__", lambda self: None):
        pipeline = TTSPipeline()

    pipeline._model = mock_model
    pipeline._ready = True
    pipeline._device = "cpu"
    pipeline._gpu_name = None
    return pipeline, mock_model


class TestSynthesizeSentenceLoop:
    """Verify sentence-by-sentence synthesis with JAPANESE_SPLIT_PATTERN."""

    def test_synthesize_sync_calls_per_sentence(self):
        """_synthesize_sync runs generate_custom_voice once per sentence."""
        pipeline, mock_model = _make_pipeline_with_mock()

        audio, sr = pipeline._synthesize_sync("最初の文。次の文。", "qwen-ja-1", 1.0)

        assert mock_model.generate_custom_voice.call_count == 2
        assert sr == 24000
        assert audio.dtype == np.float32

    def test_synthesize_sync_single_sentence_no_split(self):
        """Single sentence without terminator runs a single generation call."""
        pipeline, mock_model = _make_pipeline_with_mock()

        audio, sr = pipeline._synthesize_sync("句点なし", "qwen-ja-1", 1.0)

        assert mock_model.generate_custom_voice.call_count == 1
        assert sr == 24000
        assert audio.size > 0

    def test_synthesize_sync_passes_voice_preset_and_instruct(self):
        """Voice preset name and instruct string reach generate_custom_voice."""
        pipeline, mock_model = _make_pipeline_with_mock()

        pipeline._synthesize_sync("テスト文。", "qwen-ja-2", 1.0)

        call_kwargs = mock_model.generate_custom_voice.call_args.kwargs
        assert call_kwargs["text"] == "テスト文。"
        # Qwen3-TTS' get_supported_languages() returns lowercase names.
        assert call_kwargs["language"] == "japanese"
        # qwen_speaker for qwen-ja-2 resolves via VOICES_CONFIG
        assert call_kwargs["speaker"]  # non-empty
        assert call_kwargs["instruct"]  # non-empty

    def test_synthesize_sync_speed_hint_slow(self):
        """speed < 0.85 maps to ゆっくり instruct hint."""
        pipeline, mock_model = _make_pipeline_with_mock()

        pipeline._synthesize_sync("テスト文。", "qwen-ja-1", 0.8)

        instruct = mock_model.generate_custom_voice.call_args.kwargs["instruct"]
        assert "ゆっくり" in instruct

    def test_synthesize_sync_speed_hint_fast(self):
        """speed > 1.15 maps to 速め instruct hint."""
        pipeline, mock_model = _make_pipeline_with_mock()

        pipeline._synthesize_sync("テスト文。", "qwen-ja-1", 1.25)

        instruct = mock_model.generate_custom_voice.call_args.kwargs["instruct"]
        assert "速め" in instruct

    def test_synthesize_sync_speed_hint_neutral(self):
        """0.85 <= speed <= 1.15 leaves base instruct alone."""
        pipeline, mock_model = _make_pipeline_with_mock()

        pipeline._synthesize_sync("テスト文。", "qwen-ja-1", 1.0)

        instruct = mock_model.generate_custom_voice.call_args.kwargs["instruct"]
        assert "ゆっくり" not in instruct
        assert "速め" not in instruct

    def test_synthesize_sync_unknown_voice_raises(self):
        """Unknown voice ID raises ValueError before calling the model."""
        pipeline, mock_model = _make_pipeline_with_mock()

        with pytest.raises(ValueError, match="unknown voice"):
            pipeline._synthesize_sync("テスト文。", "no-such-voice", 1.0)

        mock_model.generate_custom_voice.assert_not_called()

    @pytest.mark.asyncio
    async def test_synthesize_stream_yields_per_sentence(self):
        """synthesize_stream yields one (audio, sr) tuple per sentence."""
        pipeline, mock_model = _make_pipeline_with_mock()

        chunks = []
        async for chunk in pipeline.synthesize_stream(
            text="最初の文。次の文。", voice="qwen-ja-1", speed=1.0
        ):
            chunks.append(chunk)

        assert mock_model.generate_custom_voice.call_count == 2
        assert len(chunks) == 2
        for audio, sr in chunks:
            assert sr == 24000
            assert audio.dtype == np.float32
