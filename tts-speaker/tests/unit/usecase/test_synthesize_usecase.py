"""Unit tests for SynthesizeUsecase."""

import asyncio
from unittest.mock import AsyncMock, call

import pytest

from tts_speaker.domain.errors import (
    EmptyTextError,
    SynthesisBusyError,
    TextTooLongError,
    UpstreamUnavailableError,
)
from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort
from tts_speaker.usecase.synthesize_usecase import SynthesisResult, SynthesizeUsecase


async def test_empty_text_raises_error(mock_synthesizer: AsyncMock) -> None:
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
    )
    with pytest.raises(EmptyTextError):
        await usecase.execute("")

    with pytest.raises(EmptyTextError):
        await usecase.execute("   \n\t  ")

    with pytest.raises(EmptyTextError):
        await usecase.execute("。。。\n！？")


async def test_text_too_long_raises_error(mock_synthesizer: AsyncMock) -> None:
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=10,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
    )
    with pytest.raises(TextTooLongError):
        await usecase.execute("This is longer than 10 characters")


async def test_successful_synthesis_single_chunk(mock_synthesizer: AsyncMock, sample_wav_bytes: bytes) -> None:
    mock_synthesizer.synthesize_chunk.return_value = sample_wav_bytes
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=600.0,
    )
    result = await usecase.execute("こんにちは", speed=1.2)
    assert isinstance(result, SynthesisResult)
    assert result.chunk_count == 1
    assert result.duration_seconds > 0
    assert result.wav == sample_wav_bytes
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
    )
    text = "吾輩は猫である。名前はまだ無い。"
    speed = 1.3
    result = await usecase.execute(text, speed=speed)
    assert isinstance(result, SynthesisResult)
    assert result.chunk_count == 2
    assert mock_synthesizer.synthesize_chunk.call_args_list == [
        call("吾輩は猫である。", speed=speed),
        call("名前はまだ無い。", speed=speed),
    ]


async def test_lock_wait_timeout_raises_synthesis_busy_error(mock_synthesizer: AsyncMock) -> None:
    usecase = SynthesizeUsecase(
        synthesizer=mock_synthesizer,
        max_chunk_chars=100,
        max_text_chars=5000,
        chunk_gap_ms=200,
        queue_timeout_seconds=0.01,
    )
    await usecase._lock.acquire()
    try:
        with pytest.raises(SynthesisBusyError):
            await usecase.execute("こんにちは")
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
    )
    with pytest.raises(UpstreamUnavailableError, match="upstream down"):
        await usecase.execute("こんにちは")


async def test_concurrent_requests_do_not_interleave(sample_wav_bytes: bytes) -> None:
    """Test that two concurrent execute requests are serialized by the lock and do not interleave chunks."""
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
    )

    # Each text has 2 sentences that split into separate chunks
    req_a = "reqA:文1。reqA:文2。"
    req_b = "reqB:文1。reqB:文2。"

    task_a = asyncio.create_task(usecase.execute(req_a))
    task_b = asyncio.create_task(usecase.execute(req_b))
    await asyncio.gather(task_a, task_b)

    # The calls for reqA should be grouped together, and reqB grouped together
    # No interleaving of A and B calls
    first_prefix = call_log[0].split("-")[0]
    other_prefix = "reqB" if first_prefix == "reqA" else "reqA"

    # All calls for the first request must complete before any call of the second request
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
    result = await usecase.execute("こんにちは", speed=None)
    assert isinstance(result, SynthesisResult)
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
    result = await usecase.execute("こんにちは")
    assert isinstance(result, SynthesisResult)
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
    result = await usecase.execute(text, speed=None)
    assert isinstance(result, SynthesisResult)
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
    result = await usecase.execute("こんにちは", speed=0.8)
    assert isinstance(result, SynthesisResult)
    mock_synthesizer.synthesize_chunk.assert_called_once_with("こんにちは", speed=0.8)
