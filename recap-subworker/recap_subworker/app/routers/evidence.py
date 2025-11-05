"""Evidence pipeline HTTP endpoint."""

from __future__ import annotations

from fastapi import APIRouter, Depends

from ...domain.models import EvidenceRequest, EvidenceResponse
from ..deps import get_pipeline_dep


router = APIRouter(tags=["evidence"])


@router.post("/evidence/cluster", response_model=EvidenceResponse)
async def cluster_evidence(
    payload: EvidenceRequest,
    pipeline=Depends(get_pipeline_dep),
) -> EvidenceResponse:
    return pipeline.run(payload)
