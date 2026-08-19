"""ASGI listener wiring for optional in-process inbound mTLS."""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
from typing import Any

import uvicorn
from uvicorn.protocols.http.h11_impl import H11Protocol

from recap_subworker.app.infra.inbound_tls import (
    InboundCertReloader,
    inbound_listener_timeouts,
    inbound_mtls_port,
    load_inbound_ssl_context_from_env,
    ssl_context_factory_for,
    watch_inbound_cert_rotation,
    wrap_http_protocol,
)
from recap_subworker.app.infra.pki.start import start as start_pki_enrollment
from recap_subworker.app.main import create_app

logger = logging.getLogger(__name__)


def serve_app(*, host: str, port: int, log_level: str) -> None:
    """Bind plaintext ``port`` always; also bind :9443 when INBOUND_MTLS=true.

    Plaintext stays the cheap liveness / sidecar-upstream listener. The mTLS
    port has no HTTP fallback. There is no response-header timeout — the
    sidecar's 1560s bound must not be reintroduced on this path.
    """
    timeouts = inbound_listener_timeouts()
    keep_alive = int(timeouts.timeout_keep_alive)
    handle = start_pki_enrollment("recap-subworker")
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


async def _serve_dual(
    *,
    app: Any,
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
            app,
            host=host,
            port=plaintext_port,
            log_level=log_level,
            timeout_keep_alive=keep_alive,
        )
    )
    mtls = uvicorn.Server(
        uvicorn.Config(
            app,
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


def _serve_after_enrollment(*, host: str, port: int, log_level: str, keep_alive: int) -> None:
    ctx = load_inbound_ssl_context_from_env()
    app = create_app()
    if ctx is None:
        uvicorn.run(
            app,
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
            app=app,
            host=host,
            plaintext_port=port,
            log_level=log_level,
            keep_alive=keep_alive,
            ctx=ctx,
            reloader=reloader,
        )
    )
