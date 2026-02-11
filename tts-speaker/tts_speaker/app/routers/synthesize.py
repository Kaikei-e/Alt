"""POST /v1/synthesize endpoint."""

from __future__ import annotations

import io
import logging

import numpy as np
import soundfile as sf
from fastapi import APIRouter, Depends, HTTPException, Request
from pydantic import BaseModel, Field
from starlette.responses import Response

from ...infra.auth import verify_service_token

logger = logging.getLogger(__name__)

router = APIRouter()


class SynthesizeRequest(BaseModel):
    """Request body for text-to-speech synthesis."""

    text: str = Field(..., min_length=1, max_length=5000)
    voice: str | None = None
    speed: float | None = Field(default=None, ge=0.5, le=2.0)


@router.post("/v1/synthesize", dependencies=[Depends(verify_service_token)])
async def synthesize(body: SynthesizeRequest, request: Request) -> Response:
    """Synthesize Japanese text to WAV audio."""
    pipeline = request.app.state.pipeline
    settings = request.app.state.settings

    if not pipeline.is_ready:
        raise HTTPException(status_code=503, detail="TTS pipeline not ready")

    voice = body.voice or settings.default_voice
    speed = body.speed if body.speed is not None else settings.default_speed

    try:
        audio = await pipeline.synthesize(text=body.text, voice=voice, speed=speed)
    except Exception:
        logger.exception("TTS synthesis failed")
        raise HTTPException(status_code=500, detail="Synthesis failed")

    buf = io.BytesIO()
    sf.write(buf, audio, samplerate=24000, format="WAV", subtype="FLOAT")
    buf.seek(0)

    return Response(
        content=buf.read(),
        media_type="audio/wav",
        headers={"Content-Disposition": 'attachment; filename="speech.wav"'},
    )
