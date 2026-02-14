"""Factory for creating a typed Connect-RPC client to alt-backend."""

from __future__ import annotations

import os

import structlog

from tag_generator.gen.proto.services.backend.v1.internal_connect import (
    BackendInternalServiceClientSync,
)

logger = structlog.get_logger(__name__)


def create_backend_client() -> tuple[BackendInternalServiceClientSync, dict[str, str]] | None:
    """Create a Connect-RPC client from environment variables.

    Returns ``(client, auth_headers)`` when BACKEND_API_URL is set,
    ``None`` otherwise.  The caller should pass *auth_headers* to every
    RPC call via ``headers=``.
    """
    base_url = os.getenv("BACKEND_API_URL", "")
    if not base_url:
        return None

    # Read service token (file takes precedence)
    service_token = ""
    token_file = os.getenv("SERVICE_TOKEN_FILE", "")
    if token_file:
        try:
            with open(token_file) as f:
                service_token = f.read().strip()
        except OSError:
            pass
    if not service_token:
        service_token = os.getenv("SERVICE_TOKEN", "")

    auth_headers: dict[str, str] = {}
    if service_token:
        auth_headers["X-Service-Token"] = service_token

    client = BackendInternalServiceClientSync(
        base_url.rstrip("/"),
        proto_json=True,
        timeout_ms=30000,
    )

    logger.info("Connect-RPC backend client initialized", base_url=base_url)
    return client, auth_headers
