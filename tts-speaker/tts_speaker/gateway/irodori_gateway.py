"""Gateway adapting IrodoriHttpDriver to SpeechSynthesizerPort."""

import asyncio
import logging
from collections.abc import Awaitable, Callable

import httpx

from tts_speaker.domain.errors import (
    AudioFormatError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)
from tts_speaker.driver.irodori_http_driver import IrodoriHttpDriver
from tts_speaker.port.speech_synthesizer_port import SpeechSynthesizerPort

logger = logging.getLogger(__name__)

_ACCEPTED_CONTENT_TYPES = {"audio/wav", "audio/x-wav", "audio/wave"}


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
        payload = {
            "model": self._model_name,
            "input": text,
            "voice": self._voice_id,
            "response_format": "wav",
            "speed": speed,
            "irodori": {"chunking_enabled": False},
        }

        for attempt in range(1, self._max_attempts + 1):
            try:
                response = await self._driver.post_speech(payload)
            except httpx.ReadTimeout as err:
                logger.warning(
                    "Upstream TTS failure attempt %d/%d: read timeout",
                    attempt,
                    self._max_attempts,
                )
                raise UpstreamUnavailableError("Upstream read timeout; service busy past deadline") from err
            except httpx.TransportError as err:
                logger.warning(
                    "Upstream TTS failure attempt %d/%d: transport error %s",
                    attempt,
                    self._max_attempts,
                    err,
                )
                if attempt < self._max_attempts:
                    await self._sleep(self._backoff_seconds * (2 ** (attempt - 1)))
                    continue
                raise UpstreamUnavailableError(
                    f"Upstream TTS service unavailable after {self._max_attempts} attempts: transport error"
                ) from err

            if response.status_code == 200:
                raw_ct = response.headers.get("content-type", "")
                content_type = raw_ct.split(";")[0].strip()
                if content_type not in _ACCEPTED_CONTENT_TYPES:
                    raise AudioFormatError(f"Unexpected content type from upstream: {raw_ct}")
                return response.content

            logger.warning(
                "Upstream TTS failure attempt %d/%d status=%d body=%s",
                attempt,
                self._max_attempts,
                response.status_code,
                response.text[:200],
            )

            if 300 <= response.status_code < 400:
                raise UpstreamRejectedError(status_code=response.status_code, detail=response.text[:200])

            if response.status_code == 401:
                raise UpstreamAuthError("Upstream authentication failed (401 Unauthorized)")

            if response.status_code in (408, 429) or response.status_code >= 500:
                if attempt < self._max_attempts:
                    await self._sleep(self._backoff_seconds * (2 ** (attempt - 1)))
                    continue
                raise UpstreamUnavailableError(
                    f"Upstream server error ({response.status_code}) after {self._max_attempts} attempts"
                )

            if 400 <= response.status_code < 500:
                raise UpstreamRejectedError(status_code=response.status_code, detail=response.text[:200])

            raise UpstreamUnavailableError(f"Unexpected status code from upstream: {response.status_code}")

        raise UpstreamUnavailableError(f"Upstream TTS service unavailable after {self._max_attempts} attempts")
