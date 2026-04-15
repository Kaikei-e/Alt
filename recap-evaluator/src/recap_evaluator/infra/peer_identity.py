"""Peer identity ASGI middleware (recap-evaluator).

See tag-generator/app/tag_generator/infra/peer_identity.py for the canonical
version — they are copies. Extract into a shared package when this spreads
to a fifth service.
"""

from __future__ import annotations

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

PEER_IDENTITY_HEADER = "x-alt-peer-identity"

logger = structlog.get_logger(__name__)


def allowed_peers_from_env(env_var: str = "MTLS_ALLOWED_PEERS") -> list[str]:
    raw = os.getenv(env_var, "")
    return [p.strip() for p in raw.split(",") if p.strip()]


class PeerIdentityMiddleware(BaseHTTPMiddleware):
    def __init__(self, app, allowed: Iterable[str] | None = None, *, strict: bool = False) -> None:
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
                logger.warning("peer_identity.missing", path=request.url.path)
                return PlainTextResponse("unauthenticated peer", status_code=401)
            if self._allowed and peer not in self._allowed:
                logger.warning("peer_identity.forbidden", peer=peer, path=request.url.path)
                return PlainTextResponse("peer not allowlisted", status_code=403)
        request.state.peer_identity = peer or None
        mutable = MutableHeaders(scope=request.scope)
        if peer:
            mutable[PEER_IDENTITY_HEADER] = peer
        elif PEER_IDENTITY_HEADER in mutable:
            del mutable[PEER_IDENTITY_HEADER]
        with structlog.contextvars.bound_contextvars(peer=peer or "anon"):
            return await call_next(request)
