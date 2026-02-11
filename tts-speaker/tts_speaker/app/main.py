"""Application factory for tts-speaker."""

from __future__ import annotations

import logging
from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

from starlette.applications import Starlette
from starlette.routing import Mount, Route
from starlette.responses import JSONResponse

from ..core.pipeline import TTSPipeline
from ..infra.config import Settings, get_settings
from .connect_service import TTSConnectService
from ..gen.proto.alt.tts.v1.tts_connect import TTSServiceASGIApplication

if TYPE_CHECKING:
    from collections.abc import AsyncIterator
    from starlette.requests import Request

logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: Starlette) -> "AsyncIterator[None]":
    """Manage TTSPipeline lifecycle."""
    pipeline: TTSPipeline = app.state.pipeline

    # Only load if not overridden (test mock)
    if not pipeline.is_ready:
        await pipeline.load()

    yield

    pipeline.unload()


async def health_endpoint(request: "Request") -> JSONResponse:
    """Health check endpoint for Docker healthcheck."""
    pipeline: TTSPipeline = request.app.state.pipeline

    if not pipeline.is_ready:
        return JSONResponse(
            status_code=503,
            content={"status": "loading", "model": "kokoro-82m", "lang": "ja"},
        )

    device = getattr(pipeline, "_device", "cpu")
    return JSONResponse(
        content={"status": "ok", "model": "kokoro-82m", "lang": "ja", "device": device},
    )


def create_app(*, pipeline_override: TTSPipeline | None = None) -> Starlette:
    """Create a Starlette ASGI application instance.

    Args:
        pipeline_override: Optional mock pipeline for testing.
    """
    settings = get_settings() if pipeline_override is None else Settings()

    logging.basicConfig(
        level=getattr(logging, settings.log_level.upper(), logging.INFO),
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )

    pipeline = pipeline_override or TTSPipeline()

    # Create connect-rpc TTS service
    tts_service = TTSConnectService(pipeline, settings)
    tts_asgi = TTSServiceASGIApplication(tts_service)

    app = Starlette(
        lifespan=lifespan if pipeline_override is None else None,
        routes=[
            Route("/health", health_endpoint),
            Mount(tts_asgi.path, app=tts_asgi),
        ],
    )

    app.state.pipeline = pipeline
    app.state.settings = settings

    return app
