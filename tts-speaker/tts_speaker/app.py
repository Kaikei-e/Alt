"""FastAPI application factory for tts-speaker."""

from collections.abc import Callable
from contextlib import AbstractAsyncContextManager

from fastapi import FastAPI

import tts_speaker.gen  # noqa: F401
from tts_speaker.gen.proto.alt.tts.v1.tts_connect import TTSServiceASGIApplication
from tts_speaker.handler.health_handler import router as health_router
from tts_speaker.handler.tts_connect_service import TTSConnectService
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase


def create_app(
    usecase: SynthesizeUsecase,
    lifespan: Callable[[FastAPI], AbstractAsyncContextManager[None]] | None = None,
) -> FastAPI:
    """Create FastAPI application with routes and dependencies."""
    app = FastAPI(title="tts-speaker", lifespan=lifespan)
    app.state.usecase = usecase

    connect_service = TTSConnectService(usecase=usecase)
    app.state.connect_service = connect_service
    asgi_app = TTSServiceASGIApplication(connect_service)
    app.mount(asgi_app.path, asgi_app)

    app.include_router(health_router)
    return app
