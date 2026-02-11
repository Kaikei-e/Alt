"""Application factory for tts-speaker."""

from __future__ import annotations

import logging
from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

from fastapi import FastAPI

from ..core.pipeline import TTSPipeline
from ..infra.config import Settings, get_settings
from .routers import health, synthesize, voices

if TYPE_CHECKING:
    from collections.abc import AsyncIterator

logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI) -> "AsyncIterator[None]":
    """Manage TTSPipeline lifecycle."""
    pipeline: TTSPipeline = app.state.pipeline

    # Only load if not overridden (test mock)
    if not pipeline.is_ready:
        await pipeline.load()

    yield

    pipeline.unload()


def create_app(*, pipeline_override: TTSPipeline | None = None) -> FastAPI:
    """Create a FastAPI application instance.

    Args:
        pipeline_override: Optional mock pipeline for testing.
    """
    settings = get_settings() if pipeline_override is None else Settings()

    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    app = FastAPI(
        title="tts-speaker",
        version="0.1.0",
        lifespan=lifespan if pipeline_override is None else None,
    )

    app.state.pipeline = pipeline_override or TTSPipeline()
    app.state.settings = settings

    app.include_router(health.router)
    app.include_router(synthesize.router)
    app.include_router(voices.router)

    return app
