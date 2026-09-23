"""Application factory for recap-subworker."""

from __future__ import annotations

import os
import re
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

import structlog
from fastapi import Depends, FastAPI
from starlette.status import (
    HTTP_400_BAD_REQUEST,
    HTTP_411_LENGTH_REQUIRED,
    HTTP_413_CONTENT_TOO_LARGE,
)
from starlette.types import ASGIApp, Receive, Scope, Send

from ..infra.config import get_settings
from ..infra.logging import configure_logging
from ..infra.telemetry import setup_metrics
from .container import ServiceContainer
from .infra.admin_auth import load_admin_auth_config, require_admin_token
from .routers import (
    admin,
    classification,
    classification_runs,
    embed,
    evaluation,
    health,
    preprocessing,
    runs,
    story_clustering,
    verify,
)

logger = structlog.get_logger(__name__)

# Default 64 MiB request body limit
DEFAULT_MAX_REQUEST_BODY_BYTES = 64 * 1024 * 1024
_STRICT_DIGITS_RE = re.compile(rb"[0-9]+")


class RequestSizeLimitMiddleware:
    """Reject request bodies exceeding the configured size limit.

    Pure ASGI middleware (not BaseHTTPMiddleware) to avoid Starlette's
    known POST-body re-read deadlock on dependency-injection paths that
    materialize a second ``Request`` object (see starlette #847 / #1320).
    Only header metadata is inspected; the body stream is not consumed by
    this middleware.

    Contract:
    - A request with a body must carry Content-Length with strictly numeric digits (0-9).
    - Chunked transfer (Transfer-Encoding header present) is answered 411 Length Required
      regardless of Content-Length (no client of this service streams today: recap-worker
      uses reqwest ``.json()``, all pact interactions are fixed bodies).
    - Malformed, non-digit, or differing duplicate Content-Length headers are answered 400 Bad Request.
    - Bodies exceeding max_bytes are answered 413 Content Too Large.
    """

    def __init__(self, app: ASGIApp, max_bytes: int = DEFAULT_MAX_REQUEST_BODY_BYTES) -> None:
        self.app = app
        self.max_bytes = max_bytes

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope["type"] != "http":
            await self.app(scope, receive, send)
            return

        content_lengths: list[bytes] = []
        has_transfer_encoding = False
        for name, value in scope.get("headers", []):
            lower_name = name.lower()
            if lower_name == b"transfer-encoding":
                has_transfer_encoding = True
            elif lower_name == b"content-length":
                content_lengths.append(value)

        if has_transfer_encoding:
            await send(
                {
                    "type": "http.response.start",
                    "status": HTTP_411_LENGTH_REQUIRED,
                    "headers": [(b"content-type", b"application/json")],
                }
            )
            await send(
                {
                    "type": "http.response.body",
                    "body": b'{"detail":"Length Required"}',
                }
            )
            return

        if not content_lengths:
            await self.app(scope, receive, send)
            return

        if len(set(content_lengths)) > 1 or any(
            not _STRICT_DIGITS_RE.fullmatch(val) for val in content_lengths
        ):
            await send(
                {
                    "type": "http.response.start",
                    "status": HTTP_400_BAD_REQUEST,
                    "headers": [(b"content-type", b"application/json")],
                }
            )
            await send(
                {
                    "type": "http.response.body",
                    "body": b'{"detail":"Invalid Content-Length"}',
                }
            )
            return

        length = int(content_lengths[0])
        if length > self.max_bytes:
            await send(
                {
                    "type": "http.response.start",
                    "status": HTTP_413_CONTENT_TOO_LARGE,
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
    app.state.deep_health_runner = health.build_deep_health_runner(settings)
    # Fail-closed: a misconfigured ADMIN_AUTH/ADMIN_TOKEN_FILE aborts
    # startup here rather than serving protected routes either
    # unauthenticated or 500-ing per request (CLAUDE.md rule 9).
    app.state.admin_auth = load_admin_auth_config()

    from ..services.card_verifier import DEFAULT_FILLER_PHRASES

    effective_phrases = (
        [p.strip() for p in settings.filler_phrases.split(",") if p.strip()]
        if getattr(settings, "filler_phrases", None)
        else DEFAULT_FILLER_PHRASES
    )
    structlog.get_logger(__name__).info(
        "effective filler phrases configured at startup",
        count=len(effective_phrases),
    )
    structlog.get_logger(__name__).info(
        "request size limit configured at startup",
        max_request_body_bytes=settings.max_request_body_bytes,
        max_request_body_mib=settings.max_request_body_bytes / (1024 * 1024),
    )

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

    app.add_middleware(RequestSizeLimitMiddleware, max_bytes=settings.max_request_body_bytes)

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
    app.include_router(admin.router, prefix="/admin", dependencies=[Depends(require_admin_token)])
    app.include_router(runs.router, dependencies=[Depends(require_admin_token)])
    app.include_router(evaluation.router)
    app.include_router(classification.router, prefix="/v1")
    app.include_router(preprocessing.router, prefix="/v1")
    app.include_router(classification_runs.router)
    app.include_router(embed.router, prefix="/v1", dependencies=[Depends(require_admin_token)])
    app.include_router(
        story_clustering.router, prefix="/v1", dependencies=[Depends(require_admin_token)]
    )
    app.include_router(verify.router, prefix="/v1", dependencies=[Depends(require_admin_token)])

    return app
