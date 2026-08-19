"""Health check endpoints."""

from __future__ import annotations

import asyncio

from fastapi import APIRouter, Depends, Request
from fastapi.responses import JSONResponse

from ...domain.models import HealthResponse
from ...infra.artefact_health import assert_classifier_artefacts
from ...infra.config import Settings
from ...infra.health_deep import Check, DeepHealthRunner
from ...services.embedder import Embedder
from ..deps import get_embedder_dep, get_settings_dep

router = APIRouter(tags=["health"])

_bind_lock = asyncio.Lock()


def build_deep_health_runner(settings: Settings) -> DeepHealthRunner:
    """Process-scoped runner: cache and singleflight live for the app lifetime."""

    async def probe() -> None:
        assert_classifier_artefacts(settings)

    return DeepHealthRunner(
        "recap-subworker",
        [Check(name="classifier_artefacts", critical=True, probe=probe)],
    )


async def get_deep_health_runner(
    request: Request,
    settings: Settings = Depends(get_settings_dep),
) -> DeepHealthRunner:
    existing = getattr(request.app.state, "deep_health_runner", None)
    if existing is not None:
        return existing
    async with _bind_lock:
        existing = getattr(request.app.state, "deep_health_runner", None)
        if existing is not None:
            return existing
        runner = build_deep_health_runner(settings)
        request.app.state.deep_health_runner = runner
        return runner


@router.get("/health", response_model=HealthResponse)
async def health(
    settings: Settings = Depends(get_settings_dep),
    embedder: Embedder = Depends(get_embedder_dep),
) -> HealthResponse:
    return HealthResponse(
        status="ok", model_id=embedder.config.model_id, backend=embedder.config.backend
    )


@router.get("/health/deep")
async def health_deep(
    runner: DeepHealthRunner = Depends(get_deep_health_runner),
) -> JSONResponse:
    """Classifier / artefact readiness. Compose probes must not hit this path."""
    report = await runner.run()
    return JSONResponse(status_code=report.http_status, content=report.as_dict())
