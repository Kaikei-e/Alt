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

    audio, sample_rate = await pipeline.synthesize(
        text="テスト音声です。", voice="qwen-ja-1", speed=1.0
    )
    assert audio is not None
    assert len(audio) > 0
    assert audio.dtype.name == "float32"
    # Qwen3-TTS-12Hz default decoder; tolerate either 24 kHz or 44.1 kHz output.
    assert sample_rate in (24000, 44100)

    pipeline.unload()
    assert not pipeline.is_ready
