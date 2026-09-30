"""Usecase for synthesizing speech from text."""

import asyncio
from dataclasses import dataclass

from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort


@dataclass(frozen=True)
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
    ) -> None:
        self._synthesizer = synthesizer
        self._max_chunk_chars = max_chunk_chars
        self._max_text_chars = max_text_chars
        self._chunk_gap_ms = chunk_gap_ms
        self._lock = asyncio.Lock()

    async def execute(self, text: str, speed: float = 1.0) -> SynthesisResult:
        """Synthesize input text into combined speech audio."""
        raise NotImplementedError
