"""Factory for creating a typed Connect-RPC client to alt-backend."""

from __future__ import annotations

import os

import structlog
from pyqwest import SyncClient, SyncHTTPTransport

from tag_generator.gen.proto.services.backend.v1.internal_connect import (
    BackendInternalServiceClientSync,
)

logger = structlog.get_logger(__name__)


def create_backend_client() -> tuple[BackendInternalServiceClientSync, dict[str, str]] | None:
    """Create a Connect-RPC client from environment variables.

    Returns ``(client, auth_headers)`` when BACKEND_API_URL is set,
    ``None`` otherwise.  The caller should pass *auth_headers* to every
    RPC call via ``headers=``.

    When MTLS_ENFORCE=true, the client presents the tag-generator leaf
    cert on every request and targets BACKEND_API_MTLS_URL instead.
    """
    mtls_enforce = os.getenv("MTLS_ENFORCE") == "true"
    base_url = os.getenv("BACKEND_API_URL", "")
    if mtls_enforce:
        mtls_url = os.getenv("BACKEND_API_MTLS_URL", "")
        if not mtls_url:
            msg = "MTLS_ENFORCE=true but BACKEND_API_MTLS_URL is unset (fail-closed)"
            logger.error(msg)
            raise RuntimeError(msg)
        base_url = mtls_url
    if not base_url:
        return None

    # Read service token (file takes precedence)
    # Authentication is established at the TLS transport layer (mTLS).
    auth_headers: dict[str, str] = {}

    if mtls_enforce:
        cert_file = os.getenv("MTLS_CERT_FILE", "")
        key_file = os.getenv("MTLS_KEY_FILE", "")
        ca_file = os.getenv("MTLS_CA_FILE", "")
        if not (cert_file and key_file and ca_file):
            msg = "MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set"
            logger.error(msg)
            raise RuntimeError(msg)
        with open(cert_file, "rb") as f:
            cert_pem = f.read()
        with open(key_file, "rb") as f:
            key_pem = f.read()
        with open(ca_file, "rb") as f:
            ca_pem = f.read()
        transport = SyncHTTPTransport(
            use_system_dns=True,
            tls_cert=cert_pem,
            tls_key=key_pem,
            tls_ca_cert=ca_pem,
        )
    else:
        transport = SyncHTTPTransport(use_system_dns=True)
    http_client = SyncClient(transport=transport)

    client = BackendInternalServiceClientSync(
        base_url.rstrip("/"),
        proto_json=True,
        timeout_ms=30000,
        http_client=http_client,
    )

    logger.info(
        "Connect-RPC backend client initialized",
        base_url=base_url,
        mtls_enforce=mtls_enforce,
    )
    return client, auth_headers
