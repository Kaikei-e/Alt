"""Connect service handler for alt.tts.v1.TTSService."""

from collections.abc import AsyncIterator

from connectrpc.request import RequestContext

import tts_speaker.gen  # noqa: F401
from tts_speaker.gen.proto.alt.tts.v1.tts_pb2 import (
    SynthesizeStreamRequest,
    SynthesizeStreamResponse,
)
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase


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
        raise NotImplementedError
        if False:
            yield
