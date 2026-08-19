"""ASGI listener wiring for optional in-process inbound mTLS.

news-creator FastAPI on :11434 is the listener owner. Ollama
(news-creator-backend :11435) is not.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
from typing import Any

import uvicorn
from uvicorn.protocols.http.h11_impl import H11Protocol

from news_creator.infra.inbound_tls import (
    InboundCertReloader,
    inbound_listener_timeouts,
    inbound_mtls_port,
    load_inbound_ssl_context_from_env,
    ssl_context_factory_for,
    watch_inbound_cert_rotation,
    wrap_http_protocol,
)
from news_creator.infra.pki.start import start as start_pki_enrollment

logger = logging.getLogger(__name__)

_APP_TARGET = "main:app"
DEFAULT_HOST = "0.0.0.0"
DEFAULT_PORT = 11434


def serve_app(
    *, host: str = DEFAULT_HOST, port: int = DEFAULT_PORT, log_level: str = "info"
) -> None:
    """Bind plaintext ``port`` always; also bind :9443 when INBOUND_MTLS=true.

    Plaintext stays the cheap liveness / sidecar-upstream listener. The mTLS
    port has no HTTP fallback. There is no response-header timeout — the
    sidecar's 960s bound must not be reintroduced on this path.
    """
    timeouts = inbound_listener_timeouts()
    keep_alive = int(timeouts.timeout_keep_alive)
    handle = start_pki_enrollment("news-creator")
    try:
        _serve_after_enrollment(
            host=host,
            port=port,
            log_level=log_level,
            keep_alive=keep_alive,
        )
    finally:
        if handle is not None:
            handle.stop()


def main() -> None:
    serve_app()


async def _serve_dual(
    *,
    host: str,
    plaintext_port: int,
    log_level: str,
    keep_alive: int,
    ctx: Any,
    reloader: InboundCertReloader,
) -> None:
    mtls_port = inbound_mtls_port()
    plain = uvicorn.Server(
        uvicorn.Config(
            _APP_TARGET,
            host=host,
            port=plaintext_port,
            log_level=log_level,
            timeout_keep_alive=keep_alive,
        )
    )
    mtls = uvicorn.Server(
        uvicorn.Config(
            _APP_TARGET,
            host=host,
            port=mtls_port,
            log_level=log_level,
            lifespan="off",
            http=wrap_http_protocol(H11Protocol),
            ssl_context_factory=ssl_context_factory_for(ctx),
            timeout_keep_alive=keep_alive,
        )
    )
    logger.info(
        "inbound_mtls_listener plaintext=%s mtls=%s response_header_timeout=%s",
        plaintext_port,
        mtls_port,
        inbound_listener_timeouts().response_header_timeout,
    )
    reload_task = asyncio.create_task(watch_inbound_cert_rotation(reloader))
    try:
        await asyncio.gather(plain.serve(), mtls.serve())
    finally:
        reload_task.cancel()
        with contextlib.suppress(asyncio.CancelledError):
            await reload_task


def _serve_after_enrollment(
    *, host: str, port: int, log_level: str, keep_alive: int
) -> None:
    ctx = load_inbound_ssl_context_from_env()
    if ctx is None:
        uvicorn.run(
            _APP_TARGET,
            host=host,
            port=port,
            reload=False,
            log_level=log_level,
            timeout_keep_alive=keep_alive,
        )
        return
    cert = os.environ["MTLS_CERT_FILE"]
    key = os.environ["MTLS_KEY_FILE"]
    reloader = InboundCertReloader(ctx, cert, key)
    asyncio.run(
        _serve_dual(
            host=host,
            plaintext_port=port,
            log_level=log_level,
            keep_alive=keep_alive,
            ctx=ctx,
            reloader=reloader,
        )
    )
