"""Embedding endpoint router."""

from __future__ import annotations

import asyncio

import structlog
from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel, Field, field_validator

from ...services.embed_service import EmbedderError, EmbedResponse, EmbedService
from ..deps import get_embed_service_dep

logger = structlog.get_logger(__name__)

router = APIRouter()


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
            raise ValueError("texts must contain at least 1 item")
        if len(v) > 256:
            raise ValueError("texts must not exceed 256 items (payload exceeds batch size limit)")
        for idx, text in enumerate(v):
            if not text or not text.strip():
                raise ValueError(f"text at index {idx} must not be empty or whitespace")
        return v


@router.post("/embed", response_model=EmbedResponse)
async def embed_texts(
    request: EmbedRequest,
    service: EmbedService = Depends(get_embed_service_dep),
) -> EmbedResponse:
    """Generate sentence embeddings for 1..256 texts."""
    try:
        return await asyncio.to_thread(
            service.embed,
            texts=request.texts,
            normalize=request.normalize,
        )
    except EmbedderError as exc:
        logger.error("embedder backend failed during text embedding", error=str(exc))
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Embedding service unavailable"},
        ) from exc
    except Exception as exc:
        logger.error("unexpected error during text embedding", error=str(exc))
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail={"reason": "Embedding service unavailable"},
        ) from exc
