"""Health check endpoints."""

from __future__ import annotations

from fastapi import APIRouter, Depends

from ...domain.models import HealthResponse
from ...infra.config import Settings
from ...services.embedder import Embedder
from ..deps import get_embedder_dep, get_settings_dep


router = APIRouter(tags=["health"])


@router.get("/health", response_model=HealthResponse)
async def health(
    settings: Settings = Depends(get_settings_dep),
    embedder: Embedder = Depends(get_embedder_dep),
) -> HealthResponse:
    return HealthResponse(status="ok", model_id=embedder.config.model_id, backend=embedder.config.backend)
