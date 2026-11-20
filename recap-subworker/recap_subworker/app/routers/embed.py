"""Embedding endpoint router."""

from __future__ import annotations

import asyncio
from typing import Annotated

import structlog
from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel, Field, field_validator

from ...services.embed_service import EmbedderError, EmbedResponse, EmbedService
from ..deps import get_embed_service_dep

logger = structlog.get_logger(__name__)

router = APIRouter()
_MAX_EMBED_TEXTS = 256


class EmbedRequest(BaseModel):
    """Request payload for text embedding."""

    texts: list[str] = Field(
        ...,
        description="List of 1..256 text strings to embed",
    )
    normalize: bool = Field(
        default=True,
        description="Whether to L2-normalize the resulting embeddings",
    )

    @field_validator("texts")
    @classmethod
    def validate_texts(cls, v: list[str]) -> list[str]:
        if not v:
            msg = "texts must contain at least 1 item"
            raise ValueError(msg)
        if len(v) > _MAX_EMBED_TEXTS:
            msg = "texts must not exceed 256 items (payload exceeds batch size limit)"
            raise ValueError(msg)
        for idx, text in enumerate(v):
            if not text or not text.strip():
                msg = f"text at index {idx} must not be empty or whitespace"
                raise ValueError(msg)
        return v


@router.post("/embed", response_model=EmbedResponse)
async def embed_texts(
    request: EmbedRequest,
    service: Annotated[EmbedService, Depends(get_embed_service_dep)],
) -> EmbedResponse:
    """Generate sentence embeddings for 1..256 texts."""
    try:
        return await asyncio.to_thread(
            service.embed,
            texts=request.texts,
            normalize=request.normalize,
        )
    except EmbedderError as exc:
        logger.error(  # noqa: TRY400  # structured error field without traceback
            "embedder backend failed during text embedding", error=str(exc)
        )
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Embedding service unavailable"},
        ) from exc
    except Exception as exc:
        logger.exception("unexpected error during text embedding", error=str(exc))
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Embedding service unavailable"},
        ) from exc
