import os
import re
import hmac
from typing import Annotated

from fastapi import Depends, HTTPException, Request
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
import structlog

logger = structlog.get_logger()

# RFC 6750 b64 alphabet: must match `^[A-Za-z0-9\-._~+/]+=*$` (non-empty)
TOKEN_PATTERN = re.compile(r"^[A-Za-z0-9\-._~+/]+=*\Z")

def load_bearer_token_from_file(
    env_var: str = "EVALUATOR_API_TOKEN_FILE",
    default_path: str = "/run/secrets/evaluator_api_token"
) -> str:
    path = os.environ.get(env_var, default_path)
    if not os.path.exists(path):
        raise RuntimeError(f"Bearer token file not found at {path}")

    try:
        with open(path, "r", encoding="utf-8") as f:
            token = f.read().strip()
    except UnicodeDecodeError:
        raise RuntimeError("Bearer token file contains invalid non-ASCII characters") from None

    if not token:
        raise RuntimeError("Bearer token file is empty")

    if len(token) < 16:
        raise RuntimeError("Bearer token must be at least 16 characters long")

    if not TOKEN_PATTERN.match(token):
        raise RuntimeError("Bearer token contains invalid characters (must be RFC 6750 b64 alphabet)")

    return token


EVALUATOR_AUTH_DISABLED = os.environ.get("EVALUATOR_AUTH", "").strip().lower() == "disabled"
security = HTTPBearer(auto_error=False)

def require_bearer_token(
    request: Request,
    credentials: Annotated[HTTPAuthorizationCredentials | None, Depends(security)]
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
