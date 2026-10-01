"""Unit tests for SynthesizeUsecase streaming API."""

import asyncio
from unittest.mock import AsyncMock, call

import pytest

from tts_speaker.domain.errors import (
    EmptyTextError,
    SynthesisBusyError,
    TextTooLongError,
    UpstreamUnavailableError,
)
from tts_speaker.domain.wav import wav_info
from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort
from tts_speaker.usecase.synthesize_usecase import ChunkAudio, SynthesizeUsecase


async def test_empty_text_raises_error(mock_synthesizer: AsyncMock) -> None:
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    with pytest.raises(EmptyTextError):
        async for _ in usecase.stream(""):
            pass

    with pytest.raises(EmptyTextError):
        async for _ in usecase.stream("   \n\t  "):
            pass

    with pytest.raises(EmptyTextError):
        async for _ in usecase.stream("。。。\n！？"):
            pass


async def test_text_too_long_raises_error(mock_synthesizer: AsyncMock) -> None:
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=10,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    with pytest.raises(TextTooLongError):
        async for _ in usecase.stream("This is longer than 10 characters"):
            pass


async def test_successful_synthesis_single_chunk(mock_synthesizer: AsyncMock, sample_wav_bytes: bytes) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    chunks = [chunk async for chunk in usecase.stream("こんにちは", speed=1.2)]
    assert len(chunks) == 1
    chunk = chunks[0]
    assert isinstance(chunk, ChunkAudio)
    assert chunk.wav == sample_wav_bytes
    assert chunk.sample_rate == 48000
    assert pytest.approx(chunk.duration_seconds, abs=0.01) == 0.1
    mock_synthesizer.synthesize_chunk.assert_called_once_with("こんにちは", speed=1.2)


async def test_successful_synthesis_multiple_chunks_in_order(
    mock_synthesizer: AsyncMock, sample_wav_bytes: bytes
) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=10,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    text = "吾輩は猫である。名前はまだ無い。"
    speed = 1.3
    chunks = [chunk async for chunk in usecase.stream(text, speed=speed)]
    assert len(chunks) == 2
    assert all(isinstance(c, ChunkAudio) for c in chunks)
    assert mock_synthesizer.synthesize_chunk.call_args_list == [
        call("吾輩は猫である。", speed=speed),
        call("名前はまだ無い。", speed=speed),
    ]


async def test_trailing_silence_appended_to_all_except_last_chunk(
    mock_synthesizer: AsyncMock, sample_wav_bytes: bytes
) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=10,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    text = "吾輩は猫である。名前はまだ無い。"
    chunks = [chunk async for chunk in usecase.stream(text, speed=1.0)]
    assert len(chunks) == 2

    # Chunk 0 (not last) should have 200ms silence appended -> 0.1 + 0.2 = 0.3s
    assert pytest.approx(wav_info(chunks[0].wav)[1], abs=0.01) == 0.3
    assert pytest.approx(chunks[0].duration_seconds, abs=0.01) == 0.3

    # Chunk 1 (last) should NOT have trailing silence -> 0.1s
    assert pytest.approx(wav_info(chunks[1].wav)[1], abs=0.01) == 0.1
    assert pytest.approx(chunks[1].duration_seconds, abs=0.01) == 0.1


async def test_lock_wait_timeout_raises_synthesis_busy_error(mock_synthesizer: AsyncMock) -> None:
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=0.01,
        default_speed=1.25,
    )
    await usecase._lock.acquire()
    try:
        with pytest.raises(SynthesisBusyError):
            async for _ in usecase.stream("こんにちは"):
                pass
    finally:
        usecase._lock.release()


async def test_port_error_propagates(mock_synthesizer: AsyncMock) -> None:
    mock_synthesizer.synthesize_chunk.side_effect = UpstreamUnavailableError("upstream down")
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    with pytest.raises(UpstreamUnavailableError, match="upstream down"):
        async for _ in usecase.stream("こんにちは"):
            pass


async def test_concurrent_streams_do_not_interleave(sample_wav_bytes: bytes) -> None:
    """Test that two concurrent stream requests are serialized by the lock and do not interleave chunks."""
    call_log: list[str] = []

    class SlowSynthesizer(SpeechSynthesizerPort):
        async def synthesize_chunk(self, text: str, speed: float) -> bytes:
            req_id = text.split(":")[0]
            call_log.append(f"{req_id}-start")
            await asyncio.sleep(0.01)
            call_log.append(f"{req_id}-end")
            return sample_wav_bytes

    synthesizer = SlowSynthesizer()
    usecase = SynthesizeUsecase(
        synthesizer=synthesizer,
        max_chunk_chars=10,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )

    req_a = "reqA:文1。reqA:文2。"
    req_b = "reqB:文1。reqB:文2。"

    async def consume(text: str) -> list[ChunkAudio]:
        return [c async for c in usecase.stream(text)]

    task_a = asyncio.create_task(consume(req_a))
    task_b = asyncio.create_task(consume(req_b))
    await asyncio.gather(task_a, task_b)

    first_prefix = call_log[0].split("-")[0]
    other_prefix = "reqB" if first_prefix == "reqA" else "reqA"

    first_req_calls = [x for x in call_log if x.startswith(first_prefix)]
    other_req_calls = [x for x in call_log if x.startswith(other_prefix)]

    first_req_indices = [call_log.index(x) for x in first_req_calls]
    other_req_indices = [call_log.index(x) for x in other_req_calls]

    assert max(first_req_indices) < min(other_req_indices), f"Calls were interleaved: {call_log}"


async def test_default_speed_used_when_speed_is_none(mock_synthesizer: AsyncMock, sample_wav_bytes: bytes) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    chunks = [c async for c in usecase.stream("こんにちは", speed=None)]
    assert len(chunks) == 1
    mock_synthesizer.synthesize_chunk.assert_called_once_with("こんにちは", speed=1.25)


async def test_default_speed_used_when_speed_omitted(mock_synthesizer: AsyncMock, sample_wav_bytes: bytes) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    chunks = [c async for c in usecase.stream("こんにちは")]
    assert len(chunks) == 1
    mock_synthesizer.synthesize_chunk.assert_called_once_with("こんにちは", speed=1.25)


async def test_default_speed_forwarded_to_all_chunks(mock_synthesizer: AsyncMock, sample_wav_bytes: bytes) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=10,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    text = "吾輩は猫である。名前はまだ無い。"
    chunks = [c async for c in usecase.stream(text, speed=None)]
    assert len(chunks) == 2
    assert mock_synthesizer.synthesize_chunk.call_args_list == [
        call("吾輩は猫である。", speed=1.25),
        call("名前はまだ無い。", speed=1.25),
    ]


async def test_explicit_speed_overrides_default_speed(mock_synthesizer: AsyncMock, sample_wav_bytes: bytes) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
        default_speed=1.25,
    )
    chunks = [c async for c in usecase.stream("こんにちは", speed=0.8)]
    assert len(chunks) == 1
    mock_synthesizer.synthesize_chunk.assert_called_once_with("こんにちは", speed=0.8)
