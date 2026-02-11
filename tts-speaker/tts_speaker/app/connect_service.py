"""Connect-RPC TTSService implementation."""

from __future__ import annotations

import io
import logging
from typing import TYPE_CHECKING

import soundfile as sf
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from ..core.pipeline import VOICES, TTSPipeline, VOICE_IDS
from ..gen.proto.alt.tts.v1 import tts_pb2

if TYPE_CHECKING:
    from connectrpc.request import RequestContext

    from ..infra.config import Settings

logger = logging.getLogger(__name__)

SAMPLE_RATE = 24000


class TTSConnectService:
    """Connect-RPC implementation of alt.tts.v1.TTSService."""

    def __init__(self, pipeline: TTSPipeline, settings: Settings) -> None:
        self._pipeline = pipeline
        self._settings = settings

    def _verify_token(self, ctx: RequestContext) -> None:
        """Verify X-Service-Token header. Skip if secret is empty (dev mode)."""
        secret = self._settings.service_secret
        if not secret:
            return
        token = ctx.request_headers().get("x-service-token")
        if not token or token != secret:
            raise ConnectError(Code.UNAUTHENTICATED, "Invalid or missing service token")

    async def synthesize(
        self,
        request: tts_pb2.SynthesizeRequest,
        ctx: RequestContext,
    ) -> tts_pb2.SynthesizeResponse:
        """Synthesize text to WAV audio."""
        self._verify_token(ctx)

        if not self._pipeline.is_ready:
            raise ConnectError(Code.UNAVAILABLE, "TTS pipeline not ready")

        text = request.text
        if not text or len(text) > 5000:
            raise ConnectError(
                Code.INVALID_ARGUMENT,
                "text must be between 1 and 5000 characters",
            )

        voice = request.voice or self._settings.default_voice
        if voice not in VOICE_IDS:
            raise ConnectError(Code.INVALID_ARGUMENT, f"unknown voice: {voice}")

        speed = request.speed or self._settings.default_speed
        if speed < 0.5 or speed > 2.0:
            raise ConnectError(
                Code.INVALID_ARGUMENT,
                "speed must be between 0.5 and 2.0",
            )

        try:
            audio = await self._pipeline.synthesize(text=text, voice=voice, speed=speed)
        except Exception:
            logger.exception("TTS synthesis failed")
            raise ConnectError(Code.INTERNAL, "Synthesis failed")

        buf = io.BytesIO()
        sf.write(buf, audio, samplerate=SAMPLE_RATE, format="WAV", subtype="FLOAT")
        wav_bytes = buf.getvalue()
        duration = len(audio) / SAMPLE_RATE

        return tts_pb2.SynthesizeResponse(
            audio_wav=wav_bytes,
            sample_rate=SAMPLE_RATE,
            duration_seconds=duration,
        )

    async def list_voices(
        self,
        request: tts_pb2.ListVoicesRequest,
        ctx: RequestContext,
    ) -> tts_pb2.ListVoicesResponse:
        """Return available Japanese voices."""
        self._verify_token(ctx)

        voices = [
            tts_pb2.Voice(id=v["id"], name=v["name"], gender=v["gender"])
            for v in VOICES
        ]
        return tts_pb2.ListVoicesResponse(voices=voices)
