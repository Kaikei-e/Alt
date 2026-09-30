"""FastAPI application factory for tts-speaker."""

from collections.abc import Callable
from contextlib import AbstractAsyncContextManager

from fastapi import FastAPI

from tts_speaker.handler.health_handler import router as health_router
from tts_speaker.handler.synthesize_handler import router as synthesize_router
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase


def create_app(
    usecase: SynthesizeUsecase,
    lifespan: Callable[[FastAPI], AbstractAsyncContextManager[None]] | None = None,
) -> FastAPI:
    """Create FastAPI application with routes and dependencies."""
    app = FastAPI(title="tts-speaker", lifespan=lifespan)
    app.state.usecase = usecase
    app.include_router(health_router)
    app.include_router(synthesize_router)
    return app
