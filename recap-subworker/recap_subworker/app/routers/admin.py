"""Administrative endpoints."""

from __future__ import annotations

from fastapi import APIRouter, Depends

from ...domain.models import WarmupResponse
from ..deps import get_pipeline_dep


router = APIRouter(tags=["admin"])


@router.post("/warmup", response_model=WarmupResponse)
async def warmup(pipeline=Depends(get_pipeline_dep)) -> WarmupResponse:
    return pipeline.warmup()
