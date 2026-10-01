"""Port interface for speech synthesizer."""

from typing import Protocol


class SpeechSynthesizerPort(Protocol):
    """Abstract port for synthesizing a single chunk of speech."""

    async def synthesize_chunk(self, text: str, speed: float) -> bytes:
        """Synthesize a text chunk to WAV audio bytes.

        Args:
            text: Single text chunk to synthesize.
            speed: Speech speed factor (0.5 to 2.0).

        Returns:
            WAV audio bytes.
        """
        ...
