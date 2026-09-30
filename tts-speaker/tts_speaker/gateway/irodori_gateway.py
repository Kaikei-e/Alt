"""Gateway adapting IrodoriHttpDriver to SpeechSynthesizerPort."""

import asyncio
from collections.abc import Awaitable, Callable

from tts_speaker.driver.irodori_http_driver import IrodoriHttpDriver
from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort


class IrodoriGateway(SpeechSynthesizerPort):
    """Gateway for synthesizing speech using Irodori-TTS-Server."""

    def __init__(
        self,
        driver: IrodoriHttpDriver,
        model_name: str,
        voice_id: str,
        max_attempts: int,
        backoff_seconds: float,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
    ) -> None:
        self._driver = driver
        self._model_name = model_name
        self._voice_id = voice_id
        self._max_attempts = max_attempts
        self._backoff_seconds = backoff_seconds
        self._sleep = sleep

    async def synthesize_chunk(self, text: str, speed: float) -> bytes:
        """Synthesize a single text chunk with retry policy."""
        raise NotImplementedError
