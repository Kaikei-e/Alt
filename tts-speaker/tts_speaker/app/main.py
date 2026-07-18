"""Application factory for tts-speaker."""

from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager
from typing import TYPE_CHECKING

from starlette.applications import Starlette
from starlette.responses import JSONResponse
from starlette.routing import Mount, Route

from ..core.factory import build_engine
from ..core.pipeline import TTSPipeline
from ..gen.proto.alt.tts.v1.tts_connect import TTSServiceASGIApplication
from ..infra.config import Settings, get_settings
from .connect_service import TTSConnectService

if TYPE_CHECKING:
    from collections.abc import AsyncGenerator

    from starlette.requests import Request

logger = logging.getLogger(__name__)


def configure_logging(log_level: str) -> None:
    """Configure root logging once at process entry (not inside create_app)."""
    level = getattr(logging, log_level.upper(), None)
    if not isinstance(level, int):
        level = logging.INFO
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)s %(name)s: %(message)s",
    )


async def _gpu_keepalive_loop(pipeline: TTSPipeline, interval_sec: float) -> None:
    """Periodically run a tiny GPU op to keep AMD DPM out of idle downclock.

    Strix Point / Strix Halo iGPUs drop to ~400 MHz when idle. Without this
    matmul, the first chunk of every request after a quiet period pays a
    ramp-up tax. Loop exits on cancel.
    """
    if interval_sec <= 0:
        return
    while True:
        try:
            await asyncio.sleep(interval_sec)
            await pipeline.keepalive_tick()
        except asyncio.CancelledError:
            return
        except (RuntimeError, OSError):
            logger.exception("gpu keepalive loop iteration failed (continuing)")


@asynccontextmanager
async def lifespan(app: Starlette) -> "AsyncGenerator[None]":
    """Manage TTSPipeline lifecycle and the GPU keepalive task."""
    pipeline: TTSPipeline = app.state.pipeline
    settings: Settings = app.state.settings
    configure_logging(settings.log_level)

    # Only load if not overridden (test mock)
    if not pipeline.is_ready:
        await pipeline.load()

    keepalive_task: asyncio.Task[None] | None = None
    if pipeline.device == "cuda":
        keepalive_task = asyncio.create_task(
            _gpu_keepalive_loop(pipeline, settings.qwen_keepalive_interval_sec)
        )
        logger.info(
            "GPU keepalive task started (interval=%.1fs)",
            settings.qwen_keepalive_interval_sec,
        )

    try:
        yield
    finally:
        if keepalive_task is not None:
            keepalive_task.cancel()
            try:
                await keepalive_task
            except asyncio.CancelledError:
                pass
        pipeline.unload()


async def health_endpoint(request: "Request") -> JSONResponse:
    """Health check endpoint for Docker healthcheck."""
    pipeline: TTSPipeline = request.app.state.pipeline
    model_name = pipeline.engine.name

    if not pipeline.is_ready:
        return JSONResponse(
            status_code=503,
            content={"status": "loading", "model": model_name, "lang": "ja"},
        )

    content: dict[str, str] = {
        "status": "ok",
        "model": model_name,
        "lang": "ja",
        "device": pipeline.device,
    }
    if pipeline.gpu_name:
        content["gpu_name"] = pipeline.gpu_name
    return JSONResponse(content=content)


def create_app(*, pipeline_override: TTSPipeline | None = None) -> Starlette:
    """Create a Starlette ASGI application instance.

    Args:
        pipeline_override: Optional mock pipeline for testing.
    """
    settings = get_settings() if pipeline_override is None else Settings()

    pipeline = pipeline_override or TTSPipeline(engine=build_engine(settings))

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
