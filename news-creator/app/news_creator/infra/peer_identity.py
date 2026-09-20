"""Peer identity ASGI middleware (news-creator).

Reads X-Alt-Peer-Identity from the mTLS sidecar (ADR-000737) and attaches to
request.state + structlog context. Kept in sync with acolyte / tag-generator
/ recap-subworker / recap-evaluator copies — extract into shared package when
this spreads to a fifth service (done).
"""

from __future__ import annotations

import logging
import ipaddress
import os
from typing import TYPE_CHECKING

from starlette.datastructures import MutableHeaders
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import PlainTextResponse

from news_creator.infra.inbound_tls import is_tls_peer, verified_peer_cn

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Iterable

    from starlette.requests import Request
    from starlette.responses import Response

PEER_IDENTITY_HEADER = "x-alt-peer-identity"

# Docker healthchecks and Prometheus scrape :9443 without a caller-specific
# client cert; a TLS-authenticated request still needs a cert to reach here
# at all, so these paths stay reachable without also being in the allowlist.
_TLS_ALLOWLIST_EXEMPT_PATH_PREFIXES = ("/health", "/metrics")

logger = logging.getLogger(__name__)


def allowed_peers_from_env(env_var: str = "MTLS_ALLOWED_PEERS") -> list[str]:
    raw = os.getenv(env_var, "")
    return [p.strip() for p in raw.split(",") if p.strip()]


def strict_from_env(env_var: str = "PEER_IDENTITY_STRICT") -> bool:
    """Plaintext-side strict switch. Unset/false keeps today's default.

    Only gates the header-based plaintext path — TLS-origin allowlist
    enforcement above runs unconditionally regardless of this flag.
    """
    return os.getenv(env_var, "false").strip().lower() in {"true", "1", "on", "yes"}


def arrived_via_sidecar(request: Request) -> bool:
    """Report whether the request could have come from the mTLS sidecar.

    pki-agent runs in this container's network namespace
    (`network_mode: "service:news-creator"`) and proxies to
    `http://127.0.0.1:11434`, so a loopback transport peer is the only one that
    could have terminated mTLS and set the identity header. Every other peer
    reached the plaintext port directly and wrote whatever header it liked.
    """
    client = request.client
    if client is None:
        return False
    try:
        return ipaddress.ip_address(client.host).is_loopback
    except ValueError:
        return False


class PeerIdentityMiddleware(BaseHTTPMiddleware):
    def __init__(
        self, app, allowed: Iterable[str] | None = None, *, strict: bool = False
    ) -> None:
        super().__init__(app)
        self._allowed = {c for c in (allowed or []) if c}
        self._strict = strict

    async def dispatch(
        self,
        request: Request,
        call_next: Callable[[Request], Awaitable[Response]],
    ) -> Response:
        if is_tls_peer(request.client):
            # In-process mTLS: the verified leaf CN is the only identity.
            # A caller-supplied X-Alt-Peer-Identity is attacker-controlled.
            peer = verified_peer_cn(request.client)
            # The caller already proved possession of a client cert at the
            # :9443 handshake, so CN allowlist enforcement for this traffic
            # must not depend on `strict` — a hardcoded strict=False on the
            # plaintext side would otherwise turn MTLS_ALLOWED_PEERS into
            # documentation for the one listener it is supposed to gate.
            exempt = request.url.path.startswith(_TLS_ALLOWLIST_EXEMPT_PATH_PREFIXES)
            if not exempt:
                if not peer:
                    logger.warning(
                        "peer_identity.forbidden "
                        "reason=peer_cn_missing peer=%s path=%s",
                        peer,
                        request.url.path,
                    )
                    return PlainTextResponse("peer not allowlisted", status_code=403)
                if self._allowed and peer not in self._allowed:
                    logger.warning(
                        "peer_identity.forbidden peer=%s path=%s",
                        peer,
                        request.url.path,
                    )
                    return PlainTextResponse("peer not allowlisted", status_code=403)
        else:
            peer = request.headers.get(PEER_IDENTITY_HEADER, "").strip()
            # Two conditions, and the header is honoured only under both.
            # PEER_IDENTITY_TRUSTED is set to "on" by compose only when the
            # perimeter sidecar enforces client certs; unset means the trust
            # boundary was never configured, so fail closed. The transport check
            # is what makes that claim binding: config alone cannot tell the
            # sidecar's traffic apart from a caller that skipped it, and the
            # plaintext port is reachable without any credential.
            mtls_on = os.getenv("PEER_IDENTITY_TRUSTED", "off") == "on"
            if not mtls_on or not arrived_via_sidecar(request):
                peer = ""
            if self._strict:
                if not peer:
                    logger.warning("peer_identity.missing path=%s", request.url.path)
                    return PlainTextResponse("unauthenticated peer", status_code=401)
                if self._allowed and peer not in self._allowed:
                    logger.warning(
                        "peer_identity.forbidden peer=%s path=%s",
                        peer,
                        request.url.path,
                    )
                    return PlainTextResponse("peer not allowlisted", status_code=403)
        request.state.peer_identity = peer or None
        mutable = MutableHeaders(scope=request.scope)
        if peer:
            mutable[PEER_IDENTITY_HEADER] = peer
        elif PEER_IDENTITY_HEADER in mutable:
            del mutable[PEER_IDENTITY_HEADER]
        return await call_next(request)
