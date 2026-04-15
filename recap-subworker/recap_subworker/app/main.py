"""Application factory for recap-subworker."""

from __future__ import annotations

import os
import warnings

# Configure threading to avoid contention in container environments with high concurrency
# We have 12 concurrent workers (50% of 24 cores), so each should use 1 thread to avoid thrashing
os.environ.setdefault("OMP_NUM_THREADS", "1")
os.environ.setdefault("MKL_NUM_THREADS", "1")
os.environ.setdefault("OPENBLAS_NUM_THREADS", "1")
os.environ.setdefault("NUMBA_THREADING_LAYER", "tbb")
# Silence HuggingFace tokenizers fork-safety warning and avoid a potential
# deadlock when spawn ProcessPoolExecutor bootstraps re-import tokenizers.
os.environ.setdefault("TOKENIZERS_PARALLELISM", "false")

from collections.abc import AsyncIterator
from contextlib import asynccontextmanager

from fastapi import FastAPI
from starlette.status import HTTP_413_REQUEST_ENTITY_TOO_LARGE
from starlette.types import ASGIApp, Receive, Scope, Send

from ..infra.config import get_settings
from ..infra.logging import configure_logging
from ..infra.telemetry import setup_metrics
from .container import ServiceContainer
from .routers import (
    admin,
    classification,
    classification_runs,
    evaluation,
    health,
    preprocessing,
    runs,
)

# 10 MB request body limit
_MAX_REQUEST_BODY_BYTES = 10 * 1024 * 1024


class RequestSizeLimitMiddleware:
    """Reject request bodies exceeding the configured size limit.

    Pure ASGI middleware (not BaseHTTPMiddleware) to avoid Starlette's
    known POST-body re-read deadlock on dependency-injection paths that
    materialize a second ``Request`` object (see starlette #847 / #1320).
    Only the Content-Length header is inspected; the body stream is not
    consumed by this middleware.
    """

    def __init__(self, app: ASGIApp, max_bytes: int = _MAX_REQUEST_BODY_BYTES) -> None:
        self.app = app
        self.max_bytes = max_bytes

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        content_length_raw: bytes | None = None
        for name, value in scope.get("headers", []):
            if name == b"content-length":
                content_length_raw = value
                break

        if content_length_raw is not None:
            try:
                length = int(content_length_raw)
            except ValueError:
                length = 0
            if length > self.max_bytes:
                await send(
                    {
                        "type": "http.response.start",
                        "status": HTTP_413_REQUEST_ENTITY_TOO_LARGE,
                        "headers": [(b"content-type", b"application/json")],
                    }
                )
                await send(
                    {
                        "type": "http.response.body",
                        "body": b'{"detail":"Request body too large"}',
                    }
                )
                return

        await self.app(scope, receive, send)


@asynccontextmanager
async def _lifespan(app: FastAPI) -> AsyncIterator[None]:
    """Own the ServiceContainer lifecycle for the app instance."""
    settings = get_settings()
    container = ServiceContainer(settings)
    app.state.container = container
    try:
        yield
    finally:
        await container.shutdown()


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
        lifespan=_lifespan,
    )

    app.add_middleware(RequestSizeLimitMiddleware)

    # peer-identity capture for mTLS audit (ADR-000737).
    from recap_subworker.app.infra.peer_identity import (
        PeerIdentityMiddleware,
        allowed_peers_from_env,
    )
    app.add_middleware(
        PeerIdentityMiddleware,
        allowed=allowed_peers_from_env(),
        strict=False,
    )

    setup_metrics(app, settings)

    app.include_router(health.router)
    app.include_router(admin.router, prefix="/admin")
    app.include_router(runs.router)
    app.include_router(evaluation.router)
    app.include_router(classification.router, prefix="/v1")
    app.include_router(preprocessing.router, prefix="/v1")
    app.include_router(classification_runs.router)

    return app
