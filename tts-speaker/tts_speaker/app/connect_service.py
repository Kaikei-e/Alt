"""Connect-RPC TTSService implementation."""

from __future__ import annotations

import io
import logging
from typing import TYPE_CHECKING

import soundfile as sf
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from ..core.pipeline import VOICES, TTSPipeline, VOICE_IDS
from ..core.preprocess import preprocess_for_tts
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
        if not text:
            raise ConnectError(Code.INVALID_ARGUMENT, "text must not be empty")

        text = preprocess_for_tts(text)
        logger.debug("Preprocessed text: %r", text)

        if len(text) > 5000:
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

    async def synthesize_stream(
        self,
        request: tts_pb2.SynthesizeRequest,
        ctx: RequestContext,
    ):  # -> AsyncGenerator[tts_pb2.SynthesizeResponse, None]
        """Synthesize text to WAV audio stream."""
        self._verify_token(ctx)

        if not self._pipeline.is_ready:
            raise ConnectError(Code.UNAVAILABLE, "TTS pipeline not ready")

        text = request.text
        if not text:
            raise ConnectError(Code.INVALID_ARGUMENT, "text must not be empty")

        text = preprocess_for_tts(text)
        logger.debug("Preprocessed text: %r", text)

        if len(text) > 5000:
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
            async for chunk in self._pipeline.synthesize_stream(
                text=text, voice=voice, speed=speed
            ):
                # Encode chunk as WAV (headerless for streaming? or just raw PCM?)
                # Usually streaming audio sends headers in first chunk or just raw samples if format is agreed.
                # But here we probably want to send minimal WAV containers or just raw PCM float32/int16.
                # Re-reading the proto: "bytes audio_wav = 1;"
                # If we send multiple WAV files effectively, the client has to handle concatenation.
                # A common pattern is to send a header in the first message, then raw PCM.
                # Or just raw PCM and let client wrap.
                # However, the previous implementation sent a full WAV file.
                # Let's try to send raw PCM in bytes, or small WAV chunks?
                # "audio_wav" suggests it contains WAV formatted data.
                # If we send a WAV header in every chunk, it's valid individually but not concatenatable.
                # Let's stick to sending a WAV header in the first chunk (if possible) or just raw bytes
                # and assume the client knows what to do. The existing field is 'audio_wav'.
                # Actually, soundfile.write with a file-like object writes a header.
                # If we write small chunks, we might be writing headers every time.

                # Let's look at how we can just send the float bytes.
                # Using sf.write might be overhead.
                # But we should respect the contract "audio_wav".
                # If it expects a WAV file, maybe we should just send PCM bytes and let client handle header?
                # Or use a streaming wav writer?

                # For simplicity and given standard Connect/gRPC patterns:
                # We often send a header in the first chunk.
                # But calculating duration requires knowing total length.

                # Let's write raw float32 bytes and let the client assume 24kHz.
                # OR use soundfile to write to buffer.

                buf = io.BytesIO()
                # Writing raw float32 bytes for now as it's most robust for streaming
                # But the field is 'audio_wav'.
                # Let's write valid WAV for the chunk. The client can strip headers if needed.
                sf.write(buf, chunk, samplerate=SAMPLE_RATE, format="WAV", subtype="FLOAT")
                wav_bytes = buf.getvalue()

                duration = len(chunk) / SAMPLE_RATE

                yield tts_pb2.SynthesizeResponse(
                    audio_wav=wav_bytes,
                    sample_rate=SAMPLE_RATE,
                    duration_seconds=duration,
                )

        except Exception:
            logger.exception("TTS streaming failed")
            raise ConnectError(Code.INTERNAL, "Streaming synthesis failed")
