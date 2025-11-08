"""Application factory for recap-subworker."""

from __future__ import annotations

from fastapi import FastAPI

from ..infra.config import get_settings
from ..infra.logging import configure_logging
from ..infra.telemetry import setup_metrics
from . import deps
from .routers import admin, health, runs


def create_app() -> FastAPI:
    """Create a FastAPI application instance."""

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

    deps.register_lifecycle(app)

    return app
