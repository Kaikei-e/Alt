"""Outbound mTLS helper for httpx-based callers.

Kept in sync with similar helpers in other Python services (acolyte,
recap-evaluator, recap-subworker). Extract into a shared package when this
has spread to five or more services.

httpx never reads REQUESTS_CA_BUNDLE, so the internal CA reaches the client
only through the SSLContext built here.
"""

from __future__ import annotations

import asyncio
import contextlib
import os
import ssl
from collections.abc import AsyncIterator, Mapping
from pathlib import Path
from urllib.parse import urlparse

import structlog

from recap_evaluator.infra.pki.start import Handle
from recap_evaluator.infra.pki.start import start as start_pki_enrollment

logger = structlog.get_logger(__name__)


def mtls_enforced() -> bool:
    """Read MTLS_ENFORCE, which must be explicitly ``true`` or ``false``."""
    raw = os.getenv("MTLS_ENFORCE")
    if raw is None:
        msg = "MTLS_ENFORCE must be set to 'true' or 'false'"
        raise RuntimeError(msg)
    value = raw.strip().lower()
    if value == "true":
        return True
    if value == "false":
        return False
    msg = f"MTLS_ENFORCE must be 'true' or 'false' (got {raw!r})"
    raise RuntimeError(msg)


def build_ssl_context() -> ssl.SSLContext | None:
    """Build an SSLContext that presents the caller's leaf cert.

    Returns None only when MTLS_ENFORCE=false. Raises when the flag is
    unset or garbage, or when enforcement is requested but the
    MTLS_CERT_FILE / MTLS_KEY_FILE / MTLS_CA_FILE env vars are missing or
    the files are unreadable (fail-closed).
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
    cert rotated by the in-process enrollment loop without a restart.

    This mirrors the ``certReloader`` pattern in
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
            # Best-effort: if we can't stat at construction, force a reload
            # on first maybe_reload invocation.
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
            # Keep the previously loaded cert; try again next tick.
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
            # Never let a transient error take the task down.
            logger.debug("mtls_cert_rotation_watch_error", exc_info=True)
            continue


def require_client_cert_for_https(upstreams: Mapping[str, str], *, enforced: bool) -> None:
    """Refuse to start when an https upstream would be called without a
    client certificate: internal TLS peers reject such handshakes."""
    if enforced:
        return
    https = sorted(name for name, url in upstreams.items() if urlparse(url).scheme == "https")
    if https:
        msg = (
            f"{', '.join(https)} use https but MTLS_ENFORCE=false; internal TLS "
            "upstreams require the client certificate"
        )
        raise RuntimeError(msg)


def require_enrolled_leaf(handle: Handle | None) -> None:
    """Refuse a client leaf that the in-process enrollment does not renew."""
    if handle is None:
        msg = (
            "MTLS_ENFORCE=true requires PKI_ENROLLMENT=enabled: nothing else "
            "renews the client leaf"
        )
        raise RuntimeError(msg)
    for env_name, enrolled_name, enrolled in (
        ("MTLS_CERT_FILE", "CERT_PATH", handle.cert_path),
        ("MTLS_KEY_FILE", "KEY_PATH", handle.key_path),
    ):
        configured = os.getenv(env_name, "")
        if Path(configured) != Path(enrolled):
            msg = (
                f"{env_name}={configured!r} must equal the enrolled {enrolled_name}="
                f"{enrolled!r}; any other file is never renewed"
            )
            raise RuntimeError(msg)


@contextlib.asynccontextmanager
async def outbound_mtls(
    service_name: str, *, upstreams: Mapping[str, str]
) -> AsyncIterator[ssl.SSLContext | None]:
    """Own the outbound client identity for the application's lifetime.

    Enrollment writes the leaf before the SSLContext loads it, the rotation
    watcher keeps the context current, and enrollment is stopped on every
    exit path because its renewal thread is non-daemon.
    """
    enforced = mtls_enforced()
    require_client_cert_for_https(upstreams, enforced=enforced)
    handle = await asyncio.to_thread(start_pki_enrollment, service_name)
    try:
        if enforced:
            require_enrolled_leaf(handle)
        ctx = build_ssl_context()
        if ctx is None:
            logger.info(
                "outbound_mtls_disabled",
                reason="MTLS_ENFORCE=false",
                upstreams=dict(upstreams),
            )
            yield None
            return
        cert_path = os.environ["MTLS_CERT_FILE"]
        key_path = os.environ["MTLS_KEY_FILE"]
        logger.info(
            "outbound_mtls_enabled",
            cert_file=cert_path,
            ca_file=os.environ["MTLS_CA_FILE"],
            upstreams=dict(upstreams),
        )
        watcher = asyncio.create_task(
            watch_cert_rotation(SslContextReloader(ctx, cert_path, key_path)),
            name="mtls-cert-rotation-watch",
        )
        try:
            yield ctx
        finally:
            watcher.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await watcher
    finally:
        if handle is not None:
            await asyncio.to_thread(handle.stop)
