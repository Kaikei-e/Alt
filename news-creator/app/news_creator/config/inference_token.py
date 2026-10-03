"""Bearer token resolution for the token-protected generation-proxy.

INFERENCE_AUTH=disabled (case-insensitive) is the explicit "no Authorization
header" mode, logged once per process. Otherwise INFERENCE_SERVICE_TOKEN_FILE
must name a readable file holding a well-formed token; anything else is a
startup error rather than a silent unauthenticated request (CLAUDE.md rules 8
and 9).
"""

from __future__ import annotations

import logging
import os
import re
from pathlib import Path

logger = logging.getLogger(__name__)

# Strict RFC token helper 1+ alphabet A-Za-z0-9._~+/- +optionalterminal=
_TOKEN_PATTERN = re.compile(r"^[A-Za-z0-9._~+/-]+={0,2}$")

_disabled_logged = False


def resolve_inference_token() -> str | None:
    """Resolve the generation-proxy bearer from INFERENCE_SERVICE_TOKEN_FILE.

    Returns None only when INFERENCE_AUTH=disabled. Raises ValueError when the
    file variable is unset or blank, or the file is missing, unreadable, empty
    or holds a malformed token.
    """
    global _disabled_logged

    if os.environ.get("INFERENCE_AUTH", "").strip().lower() == "disabled":
        if not _disabled_logged:
            logger.warning(
                "inference_auth_disabled: INFERENCE_AUTH=disabled was set "
                "explicitly; LLM requests carry no Authorization header"
            )
            _disabled_logged = True
        return None

    token_file = os.environ.get("INFERENCE_SERVICE_TOKEN_FILE", "").strip()
    if not token_file:
        raise ValueError(
            "inference proxy authentication requires INFERENCE_SERVICE_TOKEN_FILE "
            "or INFERENCE_AUTH=disabled"
        )

    try:
        token = Path(token_file).read_text(encoding="utf-8").strip()
    except OSError as exc:
        raise ValueError(
            f"Inference token file {token_file} not found or unreadable: {exc}"
        ) from exc

    if not token:
        raise ValueError(f"Inference token file {token_file} is empty")
    if not _TOKEN_PATTERN.match(token):
        raise ValueError(f"Invalid token format in {token_file}")

    logger.info("inference_auth_enabled", extra={"token_file": token_file})
    return token
