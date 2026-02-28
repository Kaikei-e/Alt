"""Tests for Japanese split pattern in TTSPipeline."""

from __future__ import annotations

import re
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from tts_speaker.core.pipeline import JAPANESE_SPLIT_PATTERN, TTSPipeline


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


def _make_pipeline_with_mock() -> tuple[TTSPipeline, MagicMock]:
    """Create a TTSPipeline with a mocked KPipeline."""
    mock_kpipeline = MagicMock()
    mock_audio = np.zeros(100, dtype=np.float32)
    mock_kpipeline.return_value = [("graphemes", "phonemes", mock_audio)]

    with patch.object(TTSPipeline, "__init__", lambda self: None):
        pipeline = TTSPipeline()

    pipeline._pipeline = mock_kpipeline
    pipeline._ready = True
    return pipeline, mock_kpipeline


class TestSynthesizeSplitPattern:
    """Verify split_pattern=JAPANESE_SPLIT_PATTERN is passed to KPipeline."""

    def test_synthesize_sync_passes_split_pattern(self):
        """_synthesize_sync passes split_pattern to KPipeline.__call__."""
        pipeline, mock_kpipeline = _make_pipeline_with_mock()

        pipeline._synthesize_sync("テスト文。", "jf_alpha", 1.0)

        mock_kpipeline.assert_called_once_with(
            "テスト文。",
            voice="jf_alpha",
            speed=1.0,
            split_pattern=JAPANESE_SPLIT_PATTERN,
        )

    @pytest.mark.asyncio
    async def test_synthesize_stream_passes_split_pattern(self):
        """synthesize_stream passes split_pattern to KPipeline.__call__."""
        pipeline, mock_kpipeline = _make_pipeline_with_mock()

        chunks = []
        async for chunk in pipeline.synthesize_stream(
            text="テスト文。", voice="jf_alpha", speed=1.0
        ):
            chunks.append(chunk)

        mock_kpipeline.assert_called_once_with(
            "テスト文。",
            voice="jf_alpha",
            speed=1.0,
            split_pattern=JAPANESE_SPLIT_PATTERN,
        )
        assert len(chunks) == 1
