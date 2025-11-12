"""Evidence pipeline HTTP endpoint."""

from __future__ import annotations

from fastapi import APIRouter, Depends

from ...domain.models import EvidenceRequest, EvidenceResponse
from ..deps import get_pipeline_dep, get_pipeline_runner_dep


router = APIRouter(tags=["evidence"])


@router.post("/evidence/cluster", response_model=EvidenceResponse)
async def cluster_evidence(
    payload: EvidenceRequest,
    pipeline=Depends(get_pipeline_dep),
    runner=Depends(get_pipeline_runner_dep),
) -> EvidenceResponse:
    if runner is not None:
        return await runner.run(payload)
    return pipeline.run(payload)
