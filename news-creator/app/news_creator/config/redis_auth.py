"""Redis cache authentication password resolution.

Mirrors the Go services' ResolveRedisPassword (e.g. mq-hub/app/config/config.go):
REDIS_AUTH=disabled (case-insensitive) is the explicit "auth disabled" mode,
logged once per process; REDIS_PASSWORD_FILE set but unreadable or empty is a
startup error rather than a silent fallback to an unauthenticated connection.
When neither REDIS_PASSWORD_FILE nor REDIS_AUTH=disabled is set, raises
RuntimeError naming both variables (CLAUDE.md rules 8 and 9).
"""

from __future__ import annotations

import logging
import os
from pathlib import Path

logger = logging.getLogger(__name__)

_disabled_logged = False


def resolve_redis_password() -> str | None:
    """Resolve the Redis cache auth password from REDIS_PASSWORD_FILE or REDIS_AUTH.

    Returns None when REDIS_AUTH=disabled (auth explicitly disabled).
    Raises RuntimeError when neither REDIS_PASSWORD_FILE nor REDIS_AUTH=disabled is set,
    or when REDIS_PASSWORD_FILE is set but the file is missing, unreadable, or
    resolves to an empty password.
    """
    global _disabled_logged

    auth_mode = os.environ.get("REDIS_AUTH", "").strip()
    if auth_mode.lower() == "disabled":
        if not _disabled_logged:
            logger.warning(
                "redis_auth_disabled: REDIS_AUTH=disabled was set explicitly"
            )
            _disabled_logged = True
        return None

    file_path = os.environ.get("REDIS_PASSWORD_FILE")
    if file_path is None:
        raise RuntimeError(
            "redis authentication requires REDIS_PASSWORD_FILE or REDIS_AUTH=disabled"
        )

    trimmed_path = file_path.strip()
    if not trimmed_path:
        raise RuntimeError(
            "REDIS_PASSWORD_FILE is set but empty; set REDIS_AUTH=disabled to run with redis auth disabled explicitly"
        )

    try:
        content = Path(trimmed_path).read_text()
    except OSError as exc:
        raise RuntimeError(f"read REDIS_PASSWORD_FILE {trimmed_path}: {exc}") from exc

    password = content.strip()
    if not password:
        raise RuntimeError(
            f"REDIS_PASSWORD_FILE {trimmed_path} resolved to an empty password"
        )

    return password
