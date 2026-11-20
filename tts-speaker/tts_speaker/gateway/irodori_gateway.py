"""Gateway adapting IrodoriHttpDriver to SpeechSynthesizerPort."""

import asyncio
import logging
from collections.abc import Awaitable, Callable
from dataclasses import dataclass

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
_HTTP_OK = 200
_HTTP_REDIRECT_MIN = 300
_HTTP_CLIENT_ERROR_MIN = 400
_HTTP_UNAUTHORIZED = 401
_HTTP_SERVER_ERROR_MIN = 500


@dataclass(frozen=True, slots=True)
class RetryConfig:
    """Retry policy configuration for upstream calls."""

    max_attempts: int
    backoff_seconds: float


class IrodoriGateway(SpeechSynthesizerPort):
    """Gateway for synthesizing speech using Irodori-TTS-Server."""

    def __init__(
        self,
        driver: IrodoriHttpDriver,
        model_name: str,
        voice_id: str,
        retry: RetryConfig,
        sleep: Callable[[float], Awaitable[None]] = asyncio.sleep,
    ) -> None:
        self._driver = driver
        self._model_name = model_name
        self._voice_id = voice_id
        self._max_attempts = retry.max_attempts
        self._backoff_seconds = retry.backoff_seconds
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
                msg = "Upstream read timeout; service busy past deadline"
                raise UpstreamUnavailableError(msg) from err
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
                msg = f"Upstream TTS service unavailable after {self._max_attempts} attempts: transport error"
                raise UpstreamUnavailableError(msg) from err

            if response.status_code == _HTTP_OK:
                raw_ct = response.headers.get("content-type", "")
                content_type = raw_ct.split(";")[0].strip()
                if content_type not in _ACCEPTED_CONTENT_TYPES:
                    msg = f"Unexpected content type from upstream: {raw_ct}"
                    raise AudioFormatError(msg)
                return response.content

            logger.warning(
                "Upstream TTS failure attempt %d/%d status=%d body=%s",
                attempt,
                self._max_attempts,
                response.status_code,
                response.text[:200],
            )

            if _HTTP_REDIRECT_MIN <= response.status_code < _HTTP_CLIENT_ERROR_MIN:
                raise UpstreamRejectedError(status_code=response.status_code, detail=response.text[:200])

            if response.status_code == _HTTP_UNAUTHORIZED:
                msg = "Upstream authentication failed (401 Unauthorized)"
                raise UpstreamAuthError(msg)

            if response.status_code in (408, 429) or response.status_code >= _HTTP_SERVER_ERROR_MIN:
                if attempt < self._max_attempts:
                    await self._sleep(self._backoff_seconds * (2 ** (attempt - 1)))
                    continue
                msg = f"Upstream server error ({response.status_code}) after {self._max_attempts} attempts"
                raise UpstreamUnavailableError(msg)

            if _HTTP_CLIENT_ERROR_MIN <= response.status_code < _HTTP_SERVER_ERROR_MIN:
                raise UpstreamRejectedError(status_code=response.status_code, detail=response.text[:200])

            msg = f"Unexpected status code from upstream: {response.status_code}"
            raise UpstreamUnavailableError(msg)

        msg = f"Upstream TTS service unavailable after {self._max_attempts} attempts"
        raise UpstreamUnavailableError(msg)
