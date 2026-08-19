"""Peer identity ASGI middleware.

Authenticated caller identity comes from the TLS client certificate when
Wave 4 in-process mTLS is serving :9443. The inbound ``X-Alt-Peer-Identity``
header is never trusted on that path — the listener strips it and injects
the verified CN into ``scope["extensions"]["tls"]["client_cn"]``.

During Pattern B dual-run the pki-agent sidecar still proxies to the
plaintext health port and injects the header. That header is honoured only
when ``PEER_IDENTITY_TRUSTED=on`` AND the transport peer is loopback.

In strict mode, rejects requests whose CN is not in the allowlist.
"""

from __future__ import annotations

import ipaddress
import os
from typing import TYPE_CHECKING

import structlog
from starlette.datastructures import MutableHeaders
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import PlainTextResponse

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Iterable

    from starlette.requests import Request
    from starlette.responses import Response
    from starlette.types import ASGIApp

PEER_IDENTITY_HEADER = "x-alt-peer-identity"

logger = structlog.get_logger(__name__)


def allowed_peers_from_env(env_var: str = "MTLS_ALLOWED_PEERS") -> list[str]:
    """Parse MTLS_ALLOWED_PEERS=csv into a list. Empty CSV → empty list."""
    raw = os.getenv(env_var, "")
    return [p.strip() for p in raw.split(",") if p.strip()]


def arrived_via_sidecar(request: Request) -> bool:
    """Report whether the request could have come from the mTLS sidecar.

    pki-agent runs in this container's network namespace
    (`network_mode: "service:acolyte-orchestrator"`) and proxies to
    `http://127.0.0.1:8090`, so a loopback transport peer is the only one that
    could have terminated mTLS and set the identity header. Every other peer
    reached the plaintext port directly and wrote whatever header it liked.
    Wave 4 in-process TLS does not use this path: peer CN comes from the
    client certificate, not the header.
    """
    client = request.client
    if client is None:
        return False
    try:
        return ipaddress.ip_address(client.host).is_loopback
    except ValueError:
        return False


def tls_authenticated_cn(request: Request) -> str:
    """CN injected by the in-process mTLS listener after client-cert verify."""
    extensions = request.scope.get("extensions") or {}
    tls = extensions.get("tls") or {}
    return str(tls.get("client_cn") or "").strip()


def resolve_authenticated_peer(request: Request) -> str:
    """Return the verified peer CN, or empty when identity is unauthenticated.

    TLS-authenticated CN always wins. The inbound header is used only for the
    Pattern B sidecar dual-run (trusted env + loopback transport).
    """
    tls_cn = tls_authenticated_cn(request)
    if tls_cn:
        return tls_cn
    header = request.headers.get(PEER_IDENTITY_HEADER, "").strip()
    mtls_on = os.getenv("PEER_IDENTITY_TRUSTED", "off") == "on"
    if mtls_on and arrived_via_sidecar(request):
        return header
    return ""


class PeerIdentityMiddleware(BaseHTTPMiddleware):
    """Attach authenticated peer CN to request.state.peer_identity and logs.

    - allowed: list of CNs that may call this service. Empty means "any
      alt-CA-signed cert is accepted" (enforcement happens at the nginx
      sidecar; the middleware only strips spoofed headers).
    - strict: when True, requests without a verified peer CN, or whose CN
      is not in the allowlist, return 403. Only set strict=True once all
      callers are running with mTLS client certs.
    """

    def __init__(
        self,
        app: ASGIApp,
        allowed: Iterable[str] | None = None,
        *,
        strict: bool = False,
    ) -> None:
        super().__init__(app)
        self._allowed = {c for c in (allowed or []) if c}
        self._strict = strict

    async def dispatch(
        self,
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        peer = resolve_authenticated_peer(request)

        if self._strict:
            if not peer:
                logger.warning("peer_identity.missing", path=request.url.path)
                return PlainTextResponse("unauthenticated peer", status_code=401)
            if self._allowed and peer not in self._allowed:
                logger.warning("peer_identity.forbidden", peer=peer, path=request.url.path)
                return PlainTextResponse("peer not allowlisted", status_code=403)

        request.state.peer_identity = peer or None

        # Normalize the header downstream handlers see, stripping any spoofed
        # value when not trusted.
        mutable = MutableHeaders(scope=request.scope)
        if peer:
            mutable[PEER_IDENTITY_HEADER] = peer
        elif PEER_IDENTITY_HEADER in mutable:
            del mutable[PEER_IDENTITY_HEADER]

        with structlog.contextvars.bound_contextvars(peer=peer or "anon"):
            return await call_next(request)
