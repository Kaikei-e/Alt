"""Tests for sentence/comma splitting + pipeline orchestration.

Engine-specific synthesis assertions live in `test_engine_qwen.py`. These
tests pin the orchestration contract: chunking on Japanese punctuation, the
COMMA_SPLIT_THRESHOLD_CHARS fallback, and per-sentence dispatch to the
injected engine.
"""

from __future__ import annotations

import re
from unittest.mock import MagicMock

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
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "最初の文。次の文。") if s]
        assert result == ["最初の文。", "次の文。"]

    def test_splits_on_exclamation(self):
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "すごい！本当に！") if s]
        assert result == ["すごい！", "本当に！"]

    def test_splits_on_question(self):
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "何ですか？分かりません。") if s]
        assert result == ["何ですか？", "分かりません。"]

    def test_splits_on_newline(self):
        result = [s for s in re.split(JAPANESE_SPLIT_PATTERN, "一行目\n二行目\n") if s]
        assert result == ["一行目\n", "二行目\n"]

    def test_no_split_on_touten(self):
        text = "りんご、みかん、ぶどう"
        assert re.split(JAPANESE_SPLIT_PATTERN, text) == [text]

    def test_no_split_on_ascii_period(self):
        text = "version 1.0 released"
        assert re.split(JAPANESE_SPLIT_PATTERN, text) == [text]

    def test_punctuation_stays_attached(self):
        for segment in re.split(JAPANESE_SPLIT_PATTERN, "前半。後半。"):
            assert not segment.startswith("。")


class TestCommaLevelFallback:
    """Verify _split_sentences applies 「、」 fallback above the char threshold."""

    def test_short_sentence_with_comma_not_split(self):
        text = "りんご、みかん。"
        assert len(text) < COMMA_SPLIT_THRESHOLD_CHARS
        assert TTSPipeline._split_sentences(text) == ["りんご、みかん。"]

    def test_long_sentence_with_comma_split_on_comma(self):
        text = (
            "今日は良い天気ですので散歩をしてから本屋に立ち寄ろうかと考えています、"
            "そのあとカフェでゆっくり読書したいです。"
        )
        assert len(text) >= COMMA_SPLIT_THRESHOLD_CHARS
        result = TTSPipeline._split_sentences(text)
        assert len(result) >= 2
        assert any(seg.endswith("、") for seg in result)

    def test_long_sentence_without_comma_unchanged(self):
        text = "あ" * (COMMA_SPLIT_THRESHOLD_CHARS + 20) + "。"
        assert TTSPipeline._split_sentences(text) == [text]

    def test_comma_split_preserves_punctuation_attachment(self):
        text = "あ" * COMMA_SPLIT_THRESHOLD_CHARS + "、つづき。"
        result = TTSPipeline._split_sentences(text)
        assert len(result) >= 2
        assert result[0].endswith("、")
        assert not result[1].startswith("、")


def _make_pipeline_with_mock_engine() -> tuple[TTSPipeline, MagicMock]:
    """Build a TTSPipeline backed by a MagicMock engine returning fixed audio."""
    mock_engine = MagicMock()
    mock_audio = np.zeros(100, dtype=np.float32)
    mock_engine.synth_one.return_value = (mock_audio, 24000)
    mock_engine.is_ready = True
    mock_engine.device = "cpu"
    mock_engine.gpu_name = None
    return TTSPipeline(engine=mock_engine), mock_engine


class TestSynthesizeOrchestration:
    """Pipeline dispatches one engine.synth_one per chunked sentence."""

    def test_synthesize_sync_calls_per_sentence(self):
        pipeline, mock_engine = _make_pipeline_with_mock_engine()
        audio, sr = pipeline._synthesize_sync("最初の文。次の文。", "qwen-ja-1", 1.0)

        assert mock_engine.synth_one.call_count == 2
        assert sr == 24000
        assert audio.dtype == np.float32

    def test_synthesize_sync_single_sentence_no_split(self):
        pipeline, mock_engine = _make_pipeline_with_mock_engine()
        audio, sr = pipeline._synthesize_sync("句点なし", "qwen-ja-1", 1.0)

        assert mock_engine.synth_one.call_count == 1
        assert sr == 24000
        assert audio.size > 0

    def test_synthesize_sync_passes_voice_and_speed_to_engine(self):
        pipeline, mock_engine = _make_pipeline_with_mock_engine()
        pipeline._synthesize_sync("テスト文。", "qwen-ja-2", 1.25)

        call_kwargs = mock_engine.synth_one.call_args.kwargs
        assert call_kwargs["voice"] == "qwen-ja-2"
        assert call_kwargs["speed"] == 1.25
        assert call_kwargs["sentence"] == "テスト文。"

    def test_synthesize_sync_engine_value_error_propagates(self):
        pipeline, mock_engine = _make_pipeline_with_mock_engine()
        mock_engine.synth_one.side_effect = ValueError("unknown voice: foo")

        with pytest.raises(ValueError, match="unknown voice"):
            pipeline._synthesize_sync("テスト文。", "foo", 1.0)

    @pytest.mark.asyncio
    async def test_synthesize_stream_yields_per_sentence(self):
        pipeline, mock_engine = _make_pipeline_with_mock_engine()
        chunks = []
        async for chunk in pipeline.synthesize_stream(
            text="最初の文。次の文。", voice="qwen-ja-1", speed=1.0
        ):
            chunks.append(chunk)

        assert mock_engine.synth_one.call_count == 2
        assert len(chunks) == 2
        for audio, sr in chunks:
            assert sr == 24000
            assert audio.dtype == np.float32
