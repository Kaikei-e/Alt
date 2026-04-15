"""Authentication compatibility shim.

Authentication has been removed from tts-speaker; callers are authenticated
at the TLS transport layer (mTLS peer-identity enforced by the BFF). This
module retains `verify_service_token` so existing FastAPI / Starlette
`Depends()` call sites compile unchanged — it is a no-op.
"""

from __future__ import annotations

from fastapi import Request


def verify_service_token(request: Request) -> None:
    """No-op: authentication is established at the TLS transport layer."""
    _ = request
