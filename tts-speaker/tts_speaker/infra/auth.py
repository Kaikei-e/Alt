"""X-Service-Token authentication dependency."""

from __future__ import annotations

from fastapi import HTTPException, Request
from starlette.status import HTTP_401_UNAUTHORIZED


def verify_service_token(request: Request) -> None:
    """Verify X-Service-Token header against SERVICE_SECRET.

    Reads the secret from app.state.settings so it works consistently
    in both production (get_settings cache) and tests (Settings override).
    If SERVICE_SECRET is empty (dev mode), authentication is skipped.
    """
    secret = request.app.state.settings.service_secret
    if not secret:
        return

    token = request.headers.get("X-Service-Token")
    if not token or token != secret:
        raise HTTPException(
            status_code=HTTP_401_UNAUTHORIZED,
            detail="Invalid or missing service token",
        )
