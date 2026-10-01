"""Usecase for synthesizing speech from text."""

import asyncio
from dataclasses import dataclass

from tts_speaker.domain.chunker import split_into_chunks
from tts_speaker.domain.errors import EmptyTextError, SynthesisBusyError, TextTooLongError
from tts_speaker.domain.text_normalizer import normalize_for_tts
from tts_speaker.domain.wav import concat_wav, wav_duration_seconds
from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort


@dataclass(frozen=True, slots=True)
class SynthesisResult:
    """Result of speech synthesis."""

    wav: bytes
    chunk_count: int
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

    @property
    def default_speed(self) -> float:
        """Configured default playback speed."""
        return self._default_speed

    async def execute(self, text: str, speed: float | None = None) -> SynthesisResult:
        """Synthesize input text into combined speech audio."""
        if not text or not text.strip():
            raise EmptyTextError("Input text cannot be empty")
        if len(text) > self._max_text_chars:
            raise TextTooLongError(f"Input text length {len(text)} exceeds maximum of {self._max_text_chars}")

        effective_speed = self._default_speed if speed is None else speed

        try:
            async with asyncio.timeout(self._queue_timeout_seconds):
                await self._lock.acquire()
        except TimeoutError as err:
            raise SynthesisBusyError(f"Queue wait exceeded {self._queue_timeout_seconds}s deadline") from err

        try:
            normalized = normalize_for_tts(text)
            chunks = split_into_chunks(normalized, max_chars=self._max_chunk_chars)
            if not chunks:
                raise EmptyTextError("Input text contains no synthesizable content")

            parts: list[bytes] = []
            for chunk in chunks:
                audio = await self._synthesizer.synthesize_chunk(chunk, speed=effective_speed)
                parts.append(audio)

            combined_wav = concat_wav(parts, gap_ms=self._chunk_gap_ms)
            duration = wav_duration_seconds(combined_wav)
            return SynthesisResult(
                wav=combined_wav,
                chunk_count=len(chunks),
                duration_seconds=duration,
            )
        finally:
            self._lock.release()
