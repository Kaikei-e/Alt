"""Connect service handler for alt.tts.v1.TTSService."""

import contextlib
import logging
from collections.abc import AsyncIterator

from connectrpc.code import Code
from connectrpc.errors import ConnectError
from connectrpc.request import RequestContext

import tts_speaker.gen  # noqa: F401
from tts_speaker.domain.errors import (
    AudioFormatError,
    EmptyTextError,
    SynthesisBusyError,
    TextTooLongError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)
from tts_speaker.domain.speed import MAX_SPEED, MIN_SPEED
from tts_speaker.gen.proto.alt.tts.v1.tts_pb2 import (
    SynthesizeStreamRequest,
    SynthesizeStreamResponse,
)
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase

logger = logging.getLogger(__name__)


class TTSConnectService:
    """Connect service implementation for alt.tts.v1.TTSService."""

    def __init__(self, usecase: SynthesizeUsecase) -> None:
        self._usecase = usecase

    async def synthesize_stream(
        self,
        request: SynthesizeStreamRequest,
        ctx: RequestContext,
    ) -> AsyncIterator[SynthesizeStreamResponse]:
        """Synthesize speech audio stream from request text."""
        speed: float | None = None
        if request.HasField("speed"):
            if not (MIN_SPEED <= request.speed <= MAX_SPEED):
                raise ConnectError(
                    Code.INVALID_ARGUMENT,
                    f"speed must be between {MIN_SPEED} and {MAX_SPEED}, got {request.speed}",
                )
            speed = request.speed

        try:
            async with contextlib.aclosing(self._usecase.stream(request.text, speed=speed)) as chunks:
                async for chunk in chunks:
                    yield SynthesizeStreamResponse(
                        audio_wav=chunk.wav,
                        sample_rate=chunk.sample_rate,
                        duration_seconds=chunk.duration_seconds,
                    )
        except (EmptyTextError, TextTooLongError) as err:
            logger.info("Caller error in TTS synthesis: %s", type(err).__name__)
            raise ConnectError(Code.INVALID_ARGUMENT, str(err)) from err
        except UpstreamUnavailableError as err:
            logger.warning("TTS domain error occurred: %s", type(err).__name__)
            raise ConnectError(Code.UNAVAILABLE, "Upstream TTS service is currently unavailable") from err
        except SynthesisBusyError as err:
            logger.warning("TTS domain error occurred: %s", type(err).__name__)
            raise ConnectError(Code.UNAVAILABLE, "TTS synthesis service is busy") from err
        except (UpstreamAuthError, UpstreamRejectedError) as err:
            logger.warning("TTS domain error occurred: %s", type(err).__name__)
            raise ConnectError(Code.INTERNAL, "Upstream TTS service error") from err
        except AudioFormatError as err:
            logger.warning("TTS domain error occurred: %s", type(err).__name__)
            raise ConnectError(Code.INTERNAL, "Audio format error") from err
        except Exception as err:
            logger.exception("Unexpected error in TTS synthesis: %s", err)
            raise ConnectError(Code.INTERNAL, "Internal server error") from err
