"""User identity verification and interceptor for Connect-RPC."""

from __future__ import annotations

import contextvars
from typing import TYPE_CHECKING
from uuid import UUID

import jwt
import structlog
from connectrpc.code import Code
from connectrpc.errors import ConnectError

if TYPE_CHECKING:
    from collections.abc import AsyncIterator, Awaitable, Callable

    from connectrpc.request import RequestContext

    from acolyte.config.settings import Settings

BACKEND_TOKEN_HEADER = "x-alt-backend-token"  # noqa: S105 — HTTP header name, not a secret token value

logger = structlog.get_logger(__name__)

# Request-scoped acting user UUID.
current_user_id: contextvars.ContextVar[UUID | None] = contextvars.ContextVar("current_user_id", default=None)


def attach_acting_user_id(ctx: RequestContext, user_id: UUID) -> None:
    """Attach the authenticated user UUID to the request context.

    RequestContext is a third-party class with no slot for application state,
    so the instance dict is written directly; ``ctx.user_id`` then reads back
    as an ordinary attribute for handlers.
    """
    ctx.__dict__["user_id"] = user_id


def get_acting_user_id(ctx: RequestContext | None = None) -> UUID | None:
    """Return the acting user UUID from the request context, else the contextvar."""
    if ctx is not None:
        uid = getattr(ctx, "user_id", None)
        if isinstance(uid, UUID):
            return uid
    return current_user_id.get()


def resolve_backend_token_secret(settings: Settings) -> bytes | None:
    """Resolve and validate the backend token signing secret from Settings."""
    return settings.resolve_backend_token_secret()


class UserIdentityInterceptor:
    """Connect-RPC interceptor validating X-Alt-Backend-Token JWT or applying dev user identity."""

    def __init__(
        self,
        secret: bytes | None = None,
        issuer: str = "auth-hub",
        audience: str = "alt-backend",
        dev_user_id: UUID | None = None,
    ) -> None:
        if secret is None and dev_user_id is None:
            msg = "Either JWT secret or dev_user_id must be provided"
            raise ValueError(msg)
        self._secret = secret
        self._issuer = issuer
        self._audience = audience
        self._dev_user_id = dev_user_id

    def authenticate(self, ctx: RequestContext) -> UUID:
        """Validate the JWT token in request headers or return dev user UUID when disabled."""
        if self._dev_user_id is not None:
            return self._dev_user_id

        secret = self._secret
        if secret is None:
            # __init__ refuses this combination; reaching it means the interceptor
            # was mutated after construction, which must not authenticate anyone.
            msg = "UserIdentityInterceptor has neither a JWT secret nor a dev user id"
            raise RuntimeError(msg)

        token = ctx.request_headers().get(BACKEND_TOKEN_HEADER)
        if not token:
            raise ConnectError(Code.UNAUTHENTICATED, "missing backend token")

        try:
            payload = jwt.decode(
                token,
                secret,
                algorithms=["HS256"],
                issuer=self._issuer,
                audience=self._audience,
                options={
                    "verify_signature": True,
                    "verify_exp": True,
                    "verify_iss": True,
                    "verify_aud": True,
                    "require": ["exp", "iss", "aud", "sub"],
                },
            )
        except jwt.ExpiredSignatureError as exc:
            raise ConnectError(Code.UNAUTHENTICATED, "backend token has expired") from exc
        except (jwt.InvalidIssuerError, jwt.InvalidAudienceError) as exc:
            raise ConnectError(Code.UNAUTHENTICATED, "invalid token issuer or audience") from exc
        except jwt.PyJWTError as exc:
            raise ConnectError(Code.UNAUTHENTICATED, f"invalid backend token: {exc}") from exc

        sub = payload.get("sub")
        try:
            return UUID(str(sub))
        except (ValueError, TypeError) as exc:
            raise ConnectError(Code.UNAUTHENTICATED, "invalid user id in token") from exc

    async def intercept_unary[REQ, RES](
        self,
        call_next: Callable[[REQ, RequestContext], Awaitable[RES]],
        request: REQ,
        ctx: RequestContext,
    ) -> RES:
        """Intercept unary RPC methods, authenticating user unless health check."""
        if ctx.method().name == "HealthCheck":
            return await call_next(request, ctx)

        user_id = self.authenticate(ctx)
        attach_acting_user_id(ctx, user_id)
        c_tok = current_user_id.set(user_id)
        try:
            with structlog.contextvars.bound_contextvars(user_id=str(user_id)):
                return await call_next(request, ctx)
        finally:
            current_user_id.reset(c_tok)

    async def intercept_server_stream[REQ, RES](
        self,
        call_next: Callable[[REQ, RequestContext], AsyncIterator[RES]],
        request: REQ,
        ctx: RequestContext,
    ) -> AsyncIterator[RES]:
        """Intercept server-streaming RPC methods, authenticating user unless health check."""
        if ctx.method().name == "HealthCheck":
            async for resp in call_next(request, ctx):
                yield resp
            return

        user_id = self.authenticate(ctx)
        attach_acting_user_id(ctx, user_id)
        c_tok = current_user_id.set(user_id)
        try:
            with structlog.contextvars.bound_contextvars(user_id=str(user_id)):
                async for resp in call_next(request, ctx):
                    yield resp
        finally:
            current_user_id.reset(c_tok)
