import hmac
import os
import re
from pathlib import Path
from typing import Annotated

import structlog
from fastapi import Depends, HTTPException, Request
from fastapi.security import HTTPAuthorizationCredentials, HTTPBearer

logger = structlog.get_logger()

# RFC 6750 b64 alphabet: must match `^[A-Za-z0-9\-._~+/]+=*$` (non-empty)
TOKEN_PATTERN = re.compile(r"^[A-Za-z0-9\-._~+/]+=*\Z")
_MIN_TOKEN_LENGTH = 16


def load_bearer_token_from_file(
    env_var: str = "EVALUATOR_API_TOKEN_FILE",
    default_path: str = "/run/secrets/evaluator_api_token",
) -> str:
    path = Path(os.environ.get(env_var, default_path))
    if not path.exists():
        msg = f"Bearer token file not found at {path}"
        raise RuntimeError(msg)

    try:
        with path.open(encoding="utf-8") as f:
            token = f.read().strip()
    except UnicodeDecodeError:
        msg = "Bearer token file contains invalid non-ASCII characters"
        raise RuntimeError(msg) from None

    if not token:
        msg = "Bearer token file is empty"
        raise RuntimeError(msg)

    if len(token) < _MIN_TOKEN_LENGTH:
        msg = "Bearer token must be at least 16 characters long"
        raise RuntimeError(msg)

    if not TOKEN_PATTERN.match(token):
        msg = "Bearer token contains invalid characters (must be RFC 6750 b64 alphabet)"
        raise RuntimeError(msg)

    return token


EVALUATOR_AUTH_DISABLED = os.environ.get("EVALUATOR_AUTH", "").strip().lower() == "disabled"
security = HTTPBearer(auto_error=False)


def require_bearer_token(
    request: Request, credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(security)]
) -> None:
    if EVALUATOR_AUTH_DISABLED:
        return

    if credentials is None:
        raise HTTPException(
            status_code=401,
            detail="Not authenticated",
            headers={"WWW-Authenticate": "Bearer"},
        )

    expected_token = getattr(request.app.state, "api_token", "")
    if not expected_token:
        # Fallback if somehow missing
        raise HTTPException(status_code=500, detail="Server missing api_token")

    untrusted = credentials.credentials
    if not untrusted or not TOKEN_PATTERN.match(untrusted):
        raise HTTPException(
            status_code=401,
            detail="Invalid authentication credentials",
            headers={"WWW-Authenticate": "Bearer"},
        )

    # Constant-time comparison
    if not hmac.compare_digest(untrusted, expected_token):
        raise HTTPException(
            status_code=401,
            detail="Invalid authentication credentials",
            headers={"WWW-Authenticate": "Bearer"},
        )
