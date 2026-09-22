"""Bearer-token guard for `/admin/*`, `/v1/runs`, `/v1/embed`, `/v1/cluster-stories`, and `/v1/verify`.

recap-worker is the only caller of these routes (admin job triggers, clustering,
embedding, and verification endpoints), and it is updated to send
`Authorization: Bearer <token>` from the same `recap_admin_token`
secret this service reads via `ADMIN_TOKEN_FILE`.

Deliberately independent of `infra.config.Settings`: that module's fields
are read by dozens of tests that construct `Settings(...)` directly without
going through env, and making this a required `Settings` field would break
all of them. `load_admin_auth_config()` reads the environment directly, is
called once from `create_app`'s lifespan, and is stored on `app.state` —
the FastAPI dependency below reads it from there.
"""

from __future__ import annotations

import hmac
import os
from dataclasses import dataclass
from pathlib import Path

import structlog
from fastapi import Depends, Header, HTTPException, Request, status

logger = structlog.get_logger(__name__)

_MIN_TOKEN_LENGTH = 24


@dataclass(frozen=True)
class AdminAuthConfig:
    """Resolved admin-token guard. `token=None` means `ADMIN_AUTH=disabled`."""

    token: str | None


def load_admin_auth_config() -> AdminAuthConfig:
    """Resolve the admin-token guard from `ADMIN_AUTH` / `ADMIN_TOKEN_FILE`.

    `ADMIN_AUTH=disabled` is the only way to leave protected routes
    (`/admin/*`, `/v1/runs`, `/v1/embed`, `/v1/cluster-stories`, `/v1/verify`)
    unauthenticated; an unset var is treated as enabled, so a deployment
    that forgets to mount `ADMIN_TOKEN_FILE` fails startup instead of
    silently serving those routes unauthenticated (CLAUDE.md rule 9).
    """
    auth_mode = os.getenv("ADMIN_AUTH", "").strip().lower()
    if auth_mode == "disabled":
        logger.warning(
            "recap_admin_auth_disabled",
            detail="ADMIN_AUTH=disabled was set explicitly; protected endpoints accept unauthenticated requests",
        )
        return AdminAuthConfig(token=None)

    token_file = os.getenv("ADMIN_TOKEN_FILE", "").strip()
    if not token_file:
        raise RuntimeError(
            "ADMIN_TOKEN_FILE is unset or empty; set it or set ADMIN_AUTH=disabled explicitly"
        )
    try:
        token = Path(token_file).read_text().strip()
    except OSError as exc:
        raise RuntimeError(f"failed to read token file '{token_file}': {exc}") from exc
    if not token:
        raise RuntimeError(f"token file '{token_file}' resolved to an empty token")
    if len(token) < _MIN_TOKEN_LENGTH:
        raise RuntimeError(
            f"token from '{token_file}' must be at least {_MIN_TOKEN_LENGTH} characters (got {len(token)})"
        )

    logger.info("recap_admin_auth_enabled")
    return AdminAuthConfig(token=token)


def get_admin_auth_config(request: Request) -> AdminAuthConfig:
    config = getattr(request.app.state, "admin_auth", None)
    if config is None:
        raise RuntimeError(
            "AdminAuthConfig is not initialized on app.state. Ensure create_app()'s lifespan is active."
        )
    return config


async def require_admin_token(
    authorization: str | None = Header(default=None),
    auth_config: AdminAuthConfig = Depends(get_admin_auth_config),
) -> None:
    """Router-level dependency guarding protected routes.

    Guards `/admin/*`, `/v1/runs`, `/v1/embed`, `/v1/cluster-stories`, and
    `/v1/verify`.

    Missing/malformed `Authorization` header and a present-but-wrong token
    both -> 401 with `WWW-Authenticate: Bearer` and the same detail, so a
    caller can't use the response to tell "no token sent" apart from
    "wrong token sent". `auth_config.token is None` (ADMIN_AUTH=disabled)
    -> pass-through. Compared as encoded bytes: `hmac.compare_digest`
    raises `TypeError` on non-ASCII `str` arguments, and Starlette decodes
    the raw header as latin-1, so a non-ASCII Authorization header must
    not reach it as `str`.
    """
    if auth_config.token is None:
        return

    presented = None
    if authorization and authorization.startswith("Bearer "):
        presented = authorization.removeprefix("Bearer ").strip()

    unauthorized = HTTPException(
        status_code=status.HTTP_401_UNAUTHORIZED,
        detail="unauthorized",
        headers={"WWW-Authenticate": "Bearer"},
    )
    if presented is None:
        raise unauthorized
    if not hmac.compare_digest(presented.encode(), auth_config.token.encode()):
        raise unauthorized
