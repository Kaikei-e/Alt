"""FastAPI application factory for tts-speaker."""

import asyncio
import contextlib
import logging
from collections.abc import Callable
from contextlib import AbstractAsyncContextManager
from typing import Any

from connectrpc.code import Code
from connectrpc.errors import ConnectError
from fastapi import FastAPI
from starlette.types import ASGIApp, Receive, Scope, Send

import tts_speaker.gen  # noqa: F401
from tts_speaker.gen.proto.alt.tts.v1.tts_connect import TTSServiceASGIApplication
from tts_speaker.handler.health_handler import router as health_router
from tts_speaker.handler.tts_connect_service import TTSConnectService
from tts_speaker.usecase.synthesize_usecase import SynthesizeUsecase

logger = logging.getLogger(__name__)


class DisconnectCancellingASGIApp:
    """ASGI shim that watches receive() for http.disconnect and cancels request handling."""

    def __init__(self, app: ASGIApp) -> None:
        self._app = app

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self._app(scope, receive, send)
            return

        receive_queue: asyncio.Queue[Any] = asyncio.Queue()

        async def watcher() -> None:
            while True:
                msg = await receive()
                await receive_queue.put(msg)
                if msg["type"] == "http.disconnect":
                    return

        async def run_handler() -> None:
            await self._app(scope, receive_queue.get, send)

        watcher_task = asyncio.create_task(watcher())
        handler_task = asyncio.create_task(run_handler())

        try:
            done, _ = await asyncio.wait(
                [handler_task, watcher_task],
                return_when=asyncio.FIRST_COMPLETED,
            )

            if watcher_task in done:
                watcher_exc = watcher_task.exception() if not watcher_task.cancelled() else None
                if watcher_exc is not None:
                    logger.error("Watcher task failed: %s", watcher_exc, exc_info=watcher_exc)
                    raise watcher_exc

                handler_task.cancel()
                try:
                    await handler_task
                except asyncio.CancelledError:
                    pass
                except ConnectError as ce:
                    if ce.code != Code.CANCELED:
                        raise
            else:
                watcher_task.cancel()
                with contextlib.suppress(asyncio.CancelledError):
                    await watcher_task
                await handler_task
        finally:
            pending = [t for t in (handler_task, watcher_task) if not t.done()]
            for task in pending:
                task.cancel()
            for task in pending:
                with contextlib.suppress(asyncio.CancelledError):
                    await task


def create_app(
    usecase: SynthesizeUsecase,
    lifespan: Callable[[FastAPI], AbstractAsyncContextManager[None]] | None = None,
) -> FastAPI:
    """Create FastAPI application with routes and dependencies."""
    app = FastAPI(title="tts-speaker", lifespan=lifespan)

    connect_service = TTSConnectService(usecase=usecase)
    asgi_app = TTSServiceASGIApplication(connect_service, read_max_bytes=65536)
    app.mount(asgi_app.path, DisconnectCancellingASGIApp(asgi_app))

    app.include_router(health_router)
    return app
