"""Evidence pipeline HTTP endpoint."""

from __future__ import annotations

import asyncio
from typing import Annotated

from fastapi import APIRouter, Depends

from ...domain.models import EvidenceRequest, EvidenceResponse
from ...services.pipeline import EvidencePipeline
from ...services.pipeline_runner import PipelineTaskRunner
from ..deps import get_pipeline_dep, get_pipeline_runner_dep

router = APIRouter(tags=["evidence"])


@router.post("/evidence/cluster", response_model=EvidenceResponse)
async def cluster_evidence(
    payload: EvidenceRequest,
    pipeline: Annotated[EvidencePipeline, Depends(get_pipeline_dep)],
    runner: Annotated[PipelineTaskRunner | None, Depends(get_pipeline_runner_dep)],
) -> EvidenceResponse:
    if runner is not None:
        return await runner.run(payload)
    return await asyncio.to_thread(pipeline.run, payload)
