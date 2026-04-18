"""Outbound mTLS helper for httpx-based callers.

Kept in sync with similar helpers in other Python services (acolyte,
recap-evaluator, recap-subworker). Extract into a shared package when this
has spread to five or more services.
"""

from __future__ import annotations

import asyncio
import os
import ssl
from pathlib import Path
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


class SslContextReloader:
    """Re-loads the leaf cert/key into a long-lived ``ssl.SSLContext`` when
    the on-disk files' mtimes advance, so new TLS handshakes pick up the
    cert rotated by pki-agent without a process restart.

    Mirrors the ``certReloader`` pattern in
    ``alt-backend/app/tlsutil/tlsutil.go``. A transient read / parse error
    (truncated file during atomic rotation) is swallowed so the existing
    cert keeps being served — the next successful call picks up the new
    one.
    """

    def __init__(self, ctx: ssl.SSLContext, cert_path: str, key_path: str) -> None:
        self._ctx = ctx
        self._cert_path = cert_path
        self._key_path = key_path
        try:
            self._cert_mtime = Path(cert_path).stat().st_mtime
            self._key_mtime = Path(key_path).stat().st_mtime
        except OSError:
            self._cert_mtime = 0.0
            self._key_mtime = 0.0

    def maybe_reload(self) -> bool:
        """Reload the cert chain if either file's mtime advanced.

        Returns True when a reload actually happened, False when the
        cached cert was kept (either because mtime hasn't advanced or
        because the fresh read failed).
        """
        try:
            cm = Path(self._cert_path).stat().st_mtime
            km = Path(self._key_path).stat().st_mtime
        except OSError:
            return False
        if cm <= self._cert_mtime and km <= self._key_mtime:
            return False
        try:
            self._ctx.load_cert_chain(certfile=self._cert_path, keyfile=self._key_path)
        except (ssl.SSLError, OSError):
            return False
        self._cert_mtime = cm
        self._key_mtime = km
        return True


async def watch_cert_rotation(
    reloader: SslContextReloader,
    interval_seconds: float = 30.0,
) -> None:
    """Background task that polls for cert rotations.

    Designed to be spawned once via ``asyncio.create_task`` in the
    application startup and cancelled at shutdown. Errors inside the loop
    are suppressed so a transient filesystem hiccup does not kill the
    task.
    """
    while True:
        try:
            await asyncio.sleep(interval_seconds)
            reloader.maybe_reload()
        except asyncio.CancelledError:
            raise
        except Exception:
            continue
