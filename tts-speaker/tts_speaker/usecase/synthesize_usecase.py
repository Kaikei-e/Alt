"""Usecase for synthesizing speech from text."""

import asyncio
from collections.abc import AsyncGenerator
from dataclasses import dataclass

from tts_speaker.domain.chunker import split_into_chunks
from tts_speaker.domain.errors import (
    EmptyTextError,
    SynthesisBusyError,
    TextTooLongError,
)
from tts_speaker.domain.text_normalizer import normalize_for_tts
from tts_speaker.domain.wav import append_silence, wav_info
from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort


@dataclass(frozen=True, slots=True)
class ChunkAudio:
    """Synthesized audio chunk."""

    wav: bytes
    sample_rate: int
    duration_seconds: float


class SynthesizeUsecase:
    """Orchestrates speech synthesis workflow."""

    def __init__(
        self,
        synthesizer: SpeechSynthesizerPort,
        max_chunk_chars: int,
        max_text_chars: int,
        chunk_gap_ms: int,
        queue_timeout_seconds: float,
        default_speed: float,
    ) -> None:
        self._synthesizer = synthesizer
        self._max_chunk_chars = max_chunk_chars
        self._max_text_chars = max_text_chars
        self._chunk_gap_ms = chunk_gap_ms
        self._queue_timeout_seconds = queue_timeout_seconds
        self._default_speed = default_speed
        self._lock = asyncio.Lock()

    async def stream(self, text: str, speed: float | None = None) -> AsyncGenerator[ChunkAudio]:
        """Synthesize input text into streamed speech audio chunks."""
        if not text or not text.strip():
            raise EmptyTextError("Input text cannot be empty")
        if len(text) > self._max_text_chars:
            raise TextTooLongError(f"Input text length {len(text)} exceeds maximum of {self._max_text_chars}")

        normalized = normalize_for_tts(text)
        chunks = split_into_chunks(normalized, max_chars=self._max_chunk_chars)
        if not chunks:
            raise EmptyTextError("Input text contains no synthesizable content")

        effective_speed = self._default_speed if speed is None else speed

        try:
            async with asyncio.timeout(self._queue_timeout_seconds):
                await self._lock.acquire()
        except TimeoutError as err:
            raise SynthesisBusyError(f"Queue wait exceeded {self._queue_timeout_seconds}s deadline") from err

        try:
            for i, chunk_text in enumerate(chunks):
                raw_wav = await self._synthesizer.synthesize_chunk(chunk_text, speed=effective_speed)
                is_last = i == len(chunks) - 1
                if not is_last and self._chunk_gap_ms > 0:
                    final_wav = append_silence(raw_wav, ms=self._chunk_gap_ms)
                else:
                    final_wav = raw_wav

                sample_rate, duration = wav_info(final_wav)

                yield ChunkAudio(
                    wav=final_wav,
                    sample_rate=sample_rate,
                    duration_seconds=duration,
                )
        finally:
            self._lock.release()
