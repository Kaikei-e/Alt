"""Integration tests for TTSPipeline (requires model download)."""

from __future__ import annotations

import pytest


@pytest.mark.integration
@pytest.mark.asyncio
async def test_pipeline_synthesize():
    """TTSPipeline produces audio output for Japanese text."""
    from tts_speaker.core.pipeline import TTSPipeline

    pipeline = TTSPipeline()
    await pipeline.load()
    assert pipeline.is_ready

    audio = await pipeline.synthesize(text="テスト音声です。", voice="jf_alpha", speed=1.0)
    assert audio is not None
    assert len(audio) > 0
    assert audio.dtype.name == "float32"

    pipeline.unload()
    assert not pipeline.is_ready
