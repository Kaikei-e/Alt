"""Peer identity ASGI middleware (news-creator).

Reads X-Alt-Peer-Identity from the mTLS sidecar (ADR-000737) and attaches to
request.state + structlog context. Kept in sync with acolyte / tag-generator
/ recap-subworker / recap-evaluator copies — extract into shared package when
this spreads to a fifth service (done).
"""

from __future__ import annotations

import logging
import os
from typing import TYPE_CHECKING

from starlette.datastructures import MutableHeaders
from starlette.middleware.base import BaseHTTPMiddleware
from starlette.responses import PlainTextResponse

if TYPE_CHECKING:
    from collections.abc import Awaitable, Callable, Iterable

    from starlette.requests import Request
    from starlette.responses import Response

PEER_IDENTITY_HEADER = "x-alt-peer-identity"

logger = logging.getLogger(__name__)


def allowed_peers_from_env(env_var: str = "MTLS_ALLOWED_PEERS") -> list[str]:
    raw = os.getenv(env_var, "")
    return [p.strip() for p in raw.split(",") if p.strip()]


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
        peer = request.headers.get(PEER_IDENTITY_HEADER, "").strip()
        if os.getenv("PEER_IDENTITY_TRUSTED", "on") != "on":
            peer = ""
        if self._strict:
            if not peer:
                logger.warning("peer_identity.missing path=%s", request.url.path)
                return PlainTextResponse("unauthenticated peer", status_code=401)
            if self._allowed and peer not in self._allowed:
                logger.warning(
                    "peer_identity.forbidden peer=%s path=%s", peer, request.url.path
                )
                return PlainTextResponse("peer not allowlisted", status_code=403)
        request.state.peer_identity = peer or None
        mutable = MutableHeaders(scope=request.scope)
        if peer:
            mutable[PEER_IDENTITY_HEADER] = peer
        elif PEER_IDENTITY_HEADER in mutable:
            del mutable[PEER_IDENTITY_HEADER]
        return await call_next(request)
