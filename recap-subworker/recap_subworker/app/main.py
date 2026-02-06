"""Application factory for recap-subworker."""

from __future__ import annotations

import os
import warnings

# Configure threading to avoid contention in container environments with high concurrency
# We have 12 concurrent workers (50% of 24 cores), so each should use 1 thread to avoid thrashing
os.environ.setdefault("OMP_NUM_THREADS", "1")
os.environ.setdefault("MKL_NUM_THREADS", "1")
os.environ.setdefault("OPENBLAS_NUM_THREADS", "1")

from fastapi import FastAPI, Request, Response
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.status import HTTP_413_REQUEST_ENTITY_TOO_LARGE

from ..infra.config import get_settings
from ..infra.logging import configure_logging
from ..infra.telemetry import setup_metrics
from . import deps
from .routers import admin, evaluation, health, runs, classification, classification_runs, preprocessing

# 10 MB request body limit
_MAX_REQUEST_BODY_BYTES = 10 * 1024 * 1024


class RequestSizeLimitMiddleware(BaseHTTPMiddleware):
    """Reject request bodies exceeding the configured size limit."""

    async def dispatch(self, request: Request, call_next):
        content_length = request.headers.get("content-length")
        if content_length and int(content_length) > _MAX_REQUEST_BODY_BYTES:
            return Response(
                content='{"detail":"Request body too large"}',
                status_code=HTTP_413_REQUEST_ENTITY_TOO_LARGE,
                media_type="application/json",
            )
        return await call_next(request)


def create_app() -> FastAPI:
    """Create a FastAPI application instance."""

    # Suppress sklearn FutureWarning about force_all_finite -> ensure_all_finite
    # This is an internal sklearn change (deprecated in 1.6, will be removed in 1.8)
    # and doesn't require code changes in our application
    warnings.filterwarnings(
        "ignore",
        message=".*force_all_finite.*was renamed to.*ensure_all_finite.*",
        category=FutureWarning,
    )

    settings = get_settings()
    configure_logging(settings.log_level)

    app = FastAPI(
        title="recap-subworker",
        version="0.1.0",
    )

    app.add_middleware(RequestSizeLimitMiddleware)
    setup_metrics(app, settings)

    app.include_router(health.router)
    app.include_router(admin.router, prefix="/admin")
    app.include_router(runs.router)
    app.include_router(evaluation.router)
    app.include_router(classification.router, prefix="/v1")
    app.include_router(preprocessing.router, prefix="/v1")
    app.include_router(classification_runs.router)

    deps.register_lifecycle(app)

    return app
