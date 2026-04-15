"""Outbound mTLS helper for httpx-based callers.

Kept in sync with similar helpers in other Python services (acolyte,
recap-evaluator, recap-subworker). Extract into a shared package when this
has spread to five or more services.
"""

from __future__ import annotations

import os
import ssl
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    pass


def mtls_enforced() -> bool:
    """Returns True iff MTLS_ENFORCE=true in the environment."""
    return os.getenv("MTLS_ENFORCE", "").lower() == "true"


def build_ssl_context() -> ssl.SSLContext | None:
    """Build an SSLContext that presents the caller's leaf cert.

    Returns None when MTLS_ENFORCE is not set. Raises when enforcement is
    requested but the MTLS_CERT_FILE / MTLS_KEY_FILE / MTLS_CA_FILE env
    vars are missing or the files are unreadable (fail-closed).
    """
    if not mtls_enforced():
        return None
    cert = os.getenv("MTLS_CERT_FILE", "")
    key = os.getenv("MTLS_KEY_FILE", "")
    ca = os.getenv("MTLS_CA_FILE", "")
    if not (cert and key and ca):
        msg = "MTLS_ENFORCE=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set"
        raise RuntimeError(msg)
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH, cafile=ca)
    ctx.load_cert_chain(certfile=cert, keyfile=key)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    return ctx
