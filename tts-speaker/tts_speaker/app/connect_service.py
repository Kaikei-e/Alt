"""Connect-RPC TTSService implementation."""

from __future__ import annotations

import io
import logging
from typing import TYPE_CHECKING

import soundfile as sf
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from ..core.pipeline import TTSPipeline
from ..core.preprocess import preprocess_for_tts
from ..gen.proto.alt.tts.v1 import tts_pb2

if TYPE_CHECKING:
    from connectrpc.request import RequestContext

    from ..infra.config import Settings

logger = logging.getLogger(__name__)

MAX_TEXT_LENGTH = 5000


class TTSConnectService:
    """Connect-RPC implementation of alt.tts.v1.TTSService."""

    def __init__(self, pipeline: TTSPipeline, settings: Settings) -> None:
        self._pipeline = pipeline
        self._settings = settings

    def _verify_token(self, ctx: RequestContext) -> None:
        """No-op: authentication is established at the TLS transport layer."""
        _ = ctx

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
        if not text:
            raise ConnectError(Code.INVALID_ARGUMENT, "text must not be empty")

        text = preprocess_for_tts(text)
        logger.debug("Preprocessed text: %r", text)

        if len(text) > MAX_TEXT_LENGTH:
            raise ConnectError(
                Code.INVALID_ARGUMENT,
                f"text must be between 1 and {MAX_TEXT_LENGTH} characters",
            )

        voice = request.voice or self._settings.default_voice
        if voice not in self._pipeline.voice_ids:
            raise ConnectError(Code.INVALID_ARGUMENT, f"unknown voice: {voice}")

        speed = request.speed or self._settings.default_speed
        if speed < 0.5 or speed > 2.0:
            raise ConnectError(
                Code.INVALID_ARGUMENT,
                "speed must be between 0.5 and 2.0",
            )

        try:
            audio, sample_rate = await self._pipeline.synthesize(
                text=text, voice=voice, speed=speed
            )
        except Exception:
            logger.exception("TTS synthesis failed")
            raise ConnectError(Code.INTERNAL, "Synthesis failed")

        buf = io.BytesIO()
        sf.write(buf, audio, samplerate=sample_rate, format="WAV", subtype="FLOAT")
        wav_bytes = buf.getvalue()
        duration = len(audio) / sample_rate

        return tts_pb2.SynthesizeResponse(
            audio_wav=wav_bytes,
            sample_rate=sample_rate,
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
            for v in self._pipeline.voices
        ]
        return tts_pb2.ListVoicesResponse(voices=voices)

    async def synthesize_stream(
        self,
        request: tts_pb2.SynthesizeStreamRequest,
        ctx: RequestContext,
    ):  # -> AsyncGenerator[tts_pb2.SynthesizeStreamResponse, None]
        """Synthesize text to WAV audio stream."""
        self._verify_token(ctx)

        if not self._pipeline.is_ready:
            raise ConnectError(Code.UNAVAILABLE, "TTS pipeline not ready")

        text = request.text
        if not text:
            raise ConnectError(Code.INVALID_ARGUMENT, "text must not be empty")

        text = preprocess_for_tts(text)
        logger.debug("Preprocessed text: %r", text)

        stream_max = self._settings.tts_max_stream_text_length
        if len(text) > stream_max:
            raise ConnectError(
                Code.INVALID_ARGUMENT,
                f"text must be between 1 and {stream_max} characters",
            )
        logger.info("Text length: %d chars (after preprocess)", len(text))

        voice = request.voice or self._settings.default_voice
        if voice not in self._pipeline.voice_ids:
            raise ConnectError(Code.INVALID_ARGUMENT, f"unknown voice: {voice}")

        speed = request.speed or self._settings.default_speed
        if speed < 0.5 or speed > 2.0:
            raise ConnectError(
                Code.INVALID_ARGUMENT,
                "speed must be between 0.5 and 2.0",
            )

        try:
            async for chunk, sample_rate in self._pipeline.synthesize_stream(
                text=text, voice=voice, speed=speed
            ):
                buf = io.BytesIO()
                sf.write(buf, chunk, samplerate=sample_rate, format="WAV", subtype="FLOAT")
                wav_bytes = buf.getvalue()
                duration = len(chunk) / sample_rate

                yield tts_pb2.SynthesizeStreamResponse(
                    audio_wav=wav_bytes,
                    sample_rate=sample_rate,
                    duration_seconds=duration,
                )

        except Exception:
            logger.exception("TTS streaming failed")
            raise ConnectError(Code.INTERNAL, "Streaming synthesis failed")
