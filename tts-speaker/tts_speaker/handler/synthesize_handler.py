"""Speech synthesis endpoint handler."""

from fastapi import APIRouter, Request, Response
from pydantic import BaseModel, ConfigDict, Field


class SynthesizeRequest(BaseModel):
    """Request payload for /v1/synthesize."""

    model_config = ConfigDict(extra="forbid")

    text: str = Field(min_length=1)
    speed: float = Field(default=1.0, ge=0.5, le=2.0)


router = APIRouter()


@router.post("/v1/synthesize")
async def synthesize(request_body: SynthesizeRequest, request: Request) -> Response:
    """Synthesize text into WAV speech audio."""
    raise NotImplementedError
