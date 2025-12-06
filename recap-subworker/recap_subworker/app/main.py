"""Application factory for recap-subworker."""

from __future__ import annotations

import warnings

from fastapi import FastAPI

from ..infra.config import get_settings
from ..infra.logging import configure_logging
from ..infra.telemetry import setup_metrics
from . import deps
from .routers import admin, evaluation, health, runs, classification, classification_runs, preprocessing


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
