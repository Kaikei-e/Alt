"""Wave 4 in-process inbound mTLS (Pattern B replacement).

Pattern B currently terminates TLS in the pki-agent sidecar
(``network_mode: service:<parent>``, ``PROXY_LISTEN=:9443``). Wave 4 moves
inbound TLS into the Python process so a parent-only ``--force-recreate``
cannot orphan ``:9443``, and so cert rotation is a handshake-time reload
rather than a process restart.

uvicorn has no Go-style ``GetCertificate`` hook. The contract is a
long-lived server ``SSLContext`` passed into the listener (not
``ssl_certfile``, which is startup-only) plus ``InboundCertReloader`` which
calls ``load_cert_chain`` after an atomic replace. No plaintext fallback on
the mTLS port.
"""

from __future__ import annotations

import asyncio
import logging
import os
import ssl
from dataclasses import dataclass
from pathlib import Path
from typing import Any

WAVE4_INBOUND_TLS = "in-process inbound TLS"

logger = logging.getLogger(__name__)

# Connection-local map of verified client CNs. uvicorn does not put the
# peer certificate in the ASGI scope, so the HTTP protocol records it at
# handshake and PeerIdentityMiddleware reads it here. Never source this
# from X-Alt-Peer-Identity.
_verified_peers: dict[tuple[str, int], str] = {}


@dataclass(frozen=True, slots=True)
class InboundListenerTimeouts:
    """Timeouts the in-process :9443 listener is allowed to impose.

    ``response_header_timeout`` is None: there is no proxy hop, so the
    sidecar's PROXY_RESPONSE_HEADER_TIMEOUT (recap 1560s / news 960s)
    must not be reintroduced. uvicorn's keep-alive bound only applies
    between requests, never to an in-flight RPC/LLM call waiting on the
    first response byte.
    """

    response_header_timeout: float | None = None
    timeout_keep_alive: float = 5.0


def inbound_listener_timeouts() -> InboundListenerTimeouts:
    """Return the in-process listener timeout contract (no header timeout)."""
    return InboundListenerTimeouts()


def inbound_mtls_enabled() -> bool:
    """True iff INBOUND_MTLS is an explicit true. Unset stays sidecar mode."""
    return os.getenv("INBOUND_MTLS", "").lower() in {"true", "1", "on"}


def inbound_mtls_port() -> int:
    raw = os.getenv("INBOUND_MTLS_PORT", "9443")
    try:
        port = int(raw)
    except ValueError:
        msg = f"INBOUND_MTLS_PORT={raw!r} is not an integer"
        raise RuntimeError(msg) from None
    if not 1 <= port <= 65535:
        msg = f"INBOUND_MTLS_PORT={port} is out of range"
        raise RuntimeError(msg)
    return port


def build_inbound_ssl_context(cert_path: str, key_path: str, ca_path: str) -> ssl.SSLContext:
    """Return a TLS 1.3 server context that presents the leaf and verifies clients.

    The returned context MUST pick up an atomic cert/key replace on the next
    handshake (or via ``InboundCertReloader.maybe_reload``) without the
    process restarting.
    """
    cert = Path(cert_path)
    key = Path(key_path)
    ca = Path(ca_path)
    if not cert.is_file() or not key.is_file() or not ca.is_file():
        msg = "inbound TLS cert/key/ca path is missing or not a file"
        raise OSError(msg)
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.maximum_version = ssl.TLSVersion.TLSv1_3
    ctx.verify_mode = ssl.CERT_REQUIRED
    ctx.check_hostname = False
    ctx.load_verify_locations(cafile=str(ca))
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))
    return ctx


def load_inbound_ssl_context_from_env() -> ssl.SSLContext | None:
    """Fail-closed loader. Missing paths with INBOUND_MTLS=true abort startup."""
    if not inbound_mtls_enabled():
        logger.info("inbound_mtls_disabled")
        return None
    cert = os.getenv("MTLS_CERT_FILE", "")
    key = os.getenv("MTLS_KEY_FILE", "")
    ca = os.getenv("MTLS_CA_FILE", "")
    if not (cert and key and ca):
        msg = "INBOUND_MTLS=true but MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set"
        raise RuntimeError(msg)
    allowed = os.getenv("MTLS_ALLOWED_PEERS", "")
    if not any(p.strip() for p in allowed.split(",")):
        msg = "INBOUND_MTLS=true but MTLS_ALLOWED_PEERS is empty"
        raise RuntimeError(msg)
    ctx = build_inbound_ssl_context(cert, key, ca)
    logger.info("inbound_mtls_enabled port=%s", inbound_mtls_port())
    return ctx


class InboundCertReloader:
    """Reloads the inbound leaf into a long-lived server ``SSLContext``."""

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
        # Handshake-time reload: Python has no GetCertificate, but the SNI
        # callback runs on every handshake and can load_cert_chain in place.
        self._ctx.sni_callback = self._on_handshake

    def _on_handshake(
        self,
        sslobj: ssl.SSLObject | ssl.SSLSocket,
        server_name: str | None,
        ctx: ssl.SSLContext,
    ) -> None:
        del sslobj, server_name, ctx
        self.maybe_reload()

    def maybe_reload(self) -> bool:
        """Reload the cert chain if either file's mtime advanced.

        Returns True when a reload happened. Transient parse errors must
        keep the previous chain (same contract as the outbound reloader).
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
        except ssl.SSLError:
            return False
        except OSError:
            return False
        self._cert_mtime = cm
        self._key_mtime = km
        return True


def common_name_from_ssl(ssl_object: ssl.SSLObject | ssl.SSLSocket | None) -> str:
    """Return the verified client leaf CN, or empty when none was presented."""
    if ssl_object is None:
        return ""
    cert = ssl_object.getpeercert()
    if not cert:
        return ""
    subject = cert.get("subject", ())
    for rdn in subject:
        for key, value in rdn:
            if key == "commonName":
                return str(value)
    return ""


def remember_tls_peer(client: tuple[str, int] | None, cn: str) -> None:
    if client is None:
        return
    if cn:
        _verified_peers[client] = cn
    else:
        _verified_peers.pop(client, None)


def forget_tls_peer(client: tuple[str, int] | None) -> None:
    if client is not None:
        _verified_peers.pop(client, None)


def verified_peer_cn(client: tuple[str, int] | None) -> str:
    """Authenticated CN from the TLS handshake, never from a request header."""
    if client is None:
        return ""
    return _verified_peers.get(client, "")


def wrap_http_protocol(protocol_cls: type[Any]) -> type[Any]:
    """Record the verified client CN on each TLS connection for allowlisting."""

    class InboundMTLSProtocol(protocol_cls):
        def connection_made(self, transport: asyncio.BaseTransport) -> None:
            super().connection_made(transport)  # type: ignore[misc]
            ssl_obj = transport.get_extra_info("ssl_object")
            cn = common_name_from_ssl(ssl_obj)
            remember_tls_peer(getattr(self, "client", None), cn)

        def connection_lost(self, exc: BaseException | None) -> None:
            forget_tls_peer(getattr(self, "client", None))
            super().connection_lost(exc)  # type: ignore[misc]

    InboundMTLSProtocol.__name__ = f"InboundMTLS{protocol_cls.__name__}"
    InboundMTLSProtocol.__qualname__ = InboundMTLSProtocol.__name__
    return InboundMTLSProtocol


def ssl_context_factory_for(ctx: ssl.SSLContext) -> Any:
    """uvicorn ``ssl_context_factory`` that serves a long-lived context."""

    def factory(_config: object, _default_factory: object) -> ssl.SSLContext:
        return ctx

    return factory


async def watch_inbound_cert_rotation(
    reloader: InboundCertReloader,
    interval_seconds: float = 30.0,
) -> None:
    """Background poll so rotation is picked up even without a handshake."""
    while True:
        try:
            await asyncio.sleep(interval_seconds)
            reloader.maybe_reload()
        except asyncio.CancelledError:
            raise
        except ssl.SSLError:
            continue
        except OSError:
            continue
