"""Application factory and entrypoint for tts-speaker."""

from typing import Any

from fastapi import FastAPI

from tts_speaker.handler.health_handler import router as health_router
from tts_speaker.handler.synthesize_handler import router as synthesize_router


def create_app(usecase: Any = None) -> FastAPI:
    """Create FastAPI application with routes and dependencies."""
    app = FastAPI(title="tts-speaker")
    app.state.usecase = usecase
    app.include_router(health_router)
    app.include_router(synthesize_router)
    return app


def run() -> None:
    """Composition root for running tts-speaker service."""
    raise NotImplementedError
