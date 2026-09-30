"""Speech synthesis endpoint handler."""

import logging
from decimal import ROUND_HALF_UP, Decimal

from fastapi import APIRouter, HTTPException, Request, Response, status
from pydantic import BaseModel, ConfigDict, Field

from tts_speaker.domain.errors import (
    AudioFormatError,
    EmptyTextError,
    SynthesisBusyError,
    TextTooLongError,
    UpstreamAuthError,
    UpstreamRejectedError,
    UpstreamUnavailableError,
)
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase

logger = logging.getLogger(__name__)


class SynthesizeRequest(BaseModel):
    """Request payload for /v1/synthesize."""

    model_config = ConfigDict(extra="forbid")

    text: str = Field(min_length=1)
    speed: float = Field(default=1.0, ge=0.5, le=2.0)


router = APIRouter()


@router.post("/v1/synthesize")
async def synthesize(request_body: SynthesizeRequest, request: Request) -> Response:
    """Synthesize text into WAV speech audio."""
    usecase: SynthesizeUsecase = request.app.state.usecase

    try:
        result = await usecase.execute(request_body.text, request_body.speed)
    except (EmptyTextError, TextTooLongError) as e:
        raise HTTPException(
            status_code=status.HTTP_422_UNPROCESSABLE_CONTENT,
            detail=str(e),
        ) from e
    except UpstreamUnavailableError:
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="Upstream TTS service is currently unavailable",
        ) from None
    except SynthesisBusyError as e:
        logger.warning("TTS synthesis service busy: %s", e)
        raise HTTPException(
            status_code=status.HTTP_503_SERVICE_UNAVAILABLE,
            detail="TTS synthesis service is busy",
        ) from None
    except AudioFormatError as e:
        logger.warning("Audio format error: %s", e)
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="Upstream TTS service error",
        ) from None
    except (UpstreamAuthError, UpstreamRejectedError):
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="Upstream TTS service error",
        ) from None

    duration_str = str(Decimal(str(result.duration_seconds)).quantize(Decimal("0.001"), rounding=ROUND_HALF_UP))
    headers = {
        "X-TTS-Chunk-Count": str(result.chunk_count),
        "X-TTS-Duration-Seconds": duration_str,
    }
    return Response(
        content=result.wav,
        media_type="audio/wav",
        headers=headers,
    )
