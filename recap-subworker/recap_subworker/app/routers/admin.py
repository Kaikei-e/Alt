"""Administrative endpoints."""

from __future__ import annotations

from fastapi import APIRouter, Depends

from ...domain.models import WarmupResponse
from ..deps import get_pipeline_dep, get_pipeline_runner_dep


router = APIRouter(tags=["admin"])


@router.post("/warmup", response_model=WarmupResponse)
async def warmup(
    pipeline=Depends(get_pipeline_dep),
    runner=Depends(get_pipeline_runner_dep),
) -> WarmupResponse:
    if runner is not None:
        return await runner.warmup()
    return pipeline.warmup()
