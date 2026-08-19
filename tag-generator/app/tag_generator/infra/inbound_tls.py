"""Wave 4 in-process inbound mTLS (Pattern B replacement).

Pattern B currently terminates TLS in the pki-agent sidecar
(``network_mode: service:<parent>``, ``PROXY_LISTEN=:9443``). Wave 4 moves
inbound TLS into the Python process so a parent-only ``--force-recreate``
cannot orphan ``:9443``, and so cert rotation is a handshake-time reload
rather than a process restart.

uvicorn has no Go-style ``GetCertificate`` hook. This module builds a
long-lived server ``SSLContext``, hands that same object to the listener
(``config.ssl = ctx``, never startup-only ``ssl_certfile``), and reloads
the chain with ``SSLContext.load_cert_chain`` when the on-disk mtime
advances. No plaintext fallback on the mTLS port.

Bind / address split
--------------------
``INBOUND_TLS_HOST``:``INBOUND_TLS_PORT`` (default ``0.0.0.0:9443``)
    East-west mTLS. This port never speaks plaintext.

Plaintext ``PORT`` (default ``9400``, bound ``0.0.0.0`` inside the container)
    In-container health / sidecar dual-run upstream. Compose publishes it
    on loopback only (``127.0.0.1:9400``). It is not an external mTLS
    fallback.

``INBOUND_TLS_ENABLED``
    ``true``  — bind the mTLS port; missing/unreadable cert/key/CA is a
                startup failure.
    ``false`` — do not bind :9443 (Pattern B sidecar still owns it).
    unset     — same as false, with an ``inbound_tls_disabled`` startup log.
                Compose cutover must set ``true`` and drop ``PROXY_LISTEN``
                / ``network_mode: service:tag-generator``.
"""

from __future__ import annotations

import asyncio
import os
import ssl
from collections.abc import Mapping
from contextlib import suppress
from dataclasses import dataclass
from pathlib import Path
from typing import TYPE_CHECKING

import structlog
import uvicorn
from uvicorn.server import ServerState

if TYPE_CHECKING:
    from collections.abc import Callable

    from uvicorn.protocols.http.h11_impl import H11Protocol

_logger = structlog.get_logger(__name__)

_PEER_HEADER = b"x-alt-peer-identity"


@dataclass(frozen=True, slots=True)
class InboundTLSBind:
    """Listener split between mTLS :9443 and the plaintext health port."""

    enabled: bool
    tls_host: str
    tls_port: int
    plaintext_host: str
    plaintext_port: int
    cert_path: str
    key_path: str
    ca_path: str
    allowed_peers: tuple[str, ...]
    disable_reason: str


@dataclass(slots=True)
class InboundTLSRuntime:
    """Running mTLS listener + cert watcher, closed as a unit on shutdown."""

    server: asyncio.Server
    reloader: InboundCertReloader
    watch_task: asyncio.Task[None]
    ctx: ssl.SSLContext

    async def aclose(self) -> None:
        self.watch_task.cancel()
        with suppress(asyncio.CancelledError):
            await self.watch_task
        self.server.close()
        await self.server.wait_closed()


def _require_readable_file(path: str, label: str) -> Path:
    target = Path(path)
    if not path or not target.is_file():
        msg = f"inbound TLS {label} is missing or not a file: {path!r}"
        raise RuntimeError(msg)  # noqa: TRY003 — fail-closed startup config, single call site
    return target


def require_inbound_tls_when_enrollment_enabled(environ: Mapping[str, str] | None = None) -> None:
    """Parents that enroll in-process own :9443 — inbound TLS must be explicit true."""
    env = os.environ if environ is None else environ
    raw = env.get("INBOUND_TLS_ENABLED", "")
    if raw.lower() in {"true", "1", "on", "yes"}:
        return
    msg = "PKI_ENROLLMENT=enabled requires INBOUND_TLS_ENABLED=true (this process owns :9443); unset/false is invalid"
    raise RuntimeError(msg)  # noqa: TRY003 — fail-closed startup config, single call site


def resolve_inbound_tls_bind(
    *,
    plaintext_host: str,
    plaintext_port: int,
    environ: Mapping[str, str] | None = None,
) -> InboundTLSBind:
    """Parse bind config. Missing certs fail only when inbound TLS is enabled."""
    env = os.environ if environ is None else environ
    if env.get("PKI_ENROLLMENT", "").strip().lower() == "enabled":
        require_inbound_tls_when_enrollment_enabled(env)
    raw = env.get("INBOUND_TLS_ENABLED", "")
    lowered = raw.lower()
    if raw == "":
        enabled = False
        reason = "INBOUND_TLS_ENABLED unset; Pattern B sidecar still owns :9443"
    elif lowered in {"false", "0", "off", "no"}:
        enabled = False
        reason = "INBOUND_TLS_ENABLED=false"
    elif lowered in {"true", "1", "on", "yes"}:
        enabled = True
        reason = ""
    else:
        msg = f"INBOUND_TLS_ENABLED must be true or false, got {raw!r}"
        raise RuntimeError(msg)  # noqa: TRY003 — fail-closed startup config, single call site

    cert = env.get("INBOUND_TLS_CERT_FILE") or env.get("MTLS_CERT_FILE") or ""
    key = env.get("INBOUND_TLS_KEY_FILE") or env.get("MTLS_KEY_FILE") or ""
    ca = env.get("INBOUND_TLS_CA_FILE") or env.get("MTLS_CA_FILE") or ""
    if enabled:
        _require_readable_file(cert, "cert (INBOUND_TLS_CERT_FILE/MTLS_CERT_FILE)")
        _require_readable_file(key, "key (INBOUND_TLS_KEY_FILE/MTLS_KEY_FILE)")
        _require_readable_file(ca, "CA (INBOUND_TLS_CA_FILE/MTLS_CA_FILE)")

    allowed = tuple(p.strip() for p in env.get("MTLS_ALLOWED_PEERS", "").split(",") if p.strip())
    return InboundTLSBind(
        enabled=enabled,
        tls_host=env.get("INBOUND_TLS_HOST", "0.0.0.0"),  # noqa: S104 — east-west overlay bind
        tls_port=int(env.get("INBOUND_TLS_PORT", "9443")),
        plaintext_host=plaintext_host,
        plaintext_port=plaintext_port,
        cert_path=cert,
        key_path=key,
        ca_path=ca,
        allowed_peers=allowed,
        disable_reason=reason,
    )


def build_inbound_ssl_context(cert_path: str, key_path: str, ca_path: str) -> ssl.SSLContext:
    """Return a TLS 1.3 server context that presents the leaf and verifies clients.

    The returned context MUST pick up an atomic cert/key replace on the next
    handshake (or via ``InboundCertReloader.maybe_reload``) without the
    process restarting.
    """
    cert = _require_readable_file(cert_path, "cert")
    key = _require_readable_file(key_path, "key")
    ca = _require_readable_file(ca_path, "CA")
    ca_pem = ca.read_bytes()
    if b"BEGIN CERTIFICATE" not in ca_pem:
        msg = f"inbound TLS CA bundle did not contain a PEM certificate: {ca_path!r}"
        raise RuntimeError(msg)  # noqa: TRY003 — fail-closed startup config, single call site

    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    ctx.verify_mode = ssl.CERT_REQUIRED
    ctx.load_verify_locations(cafile=str(ca))
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))
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
        except (
            ssl.SSLError,
            OSError,
        ):
            return False
        self._cert_mtime = cm
        self._key_mtime = km
        return True


def identities_from_ssl_object(ssl_object: ssl.SSLObject | ssl.SSLSocket | None) -> tuple[str, ...]:
    """CN + DNS SANs of the verified client cert, or empty if none."""
    if ssl_object is None:
        return ()
    cert = ssl_object.getpeercert()
    if not cert:
        return ()
    names: list[str] = []
    for rdn in cert.get("subject", ()):
        for key, value in rdn:
            if key == "commonName" and value:
                names.append(str(value))
    for kind, value in cert.get("subjectAltName", ()):
        if kind == "DNS" and value:
            names.append(str(value))
    return tuple(names)


def primary_peer_cn(identities: tuple[str, ...]) -> str:
    return identities[0] if identities else ""


def wrap_inbound_tls_protocol(
    base: type[asyncio.Protocol],
    allowed_peers: tuple[str, ...],
) -> type[asyncio.Protocol]:
    """Subclass a uvicorn HTTP protocol to inject TLS peer CN into ASGI scope.

    The inbound ``X-Alt-Peer-Identity`` header is stripped and, when a client
    cert is present, replaced with the verified CN. Callers never get to
    name themselves.
    """
    allowed = {p for p in allowed_peers if p}

    class InboundTLSProtocol(base):  # type: ignore[misc,valid-type]
        def connection_made(self, transport: asyncio.BaseTransport) -> None:
            super().connection_made(transport)  # type: ignore[misc]
            ssl_object = transport.get_extra_info("ssl_object")
            identities = identities_from_ssl_object(ssl_object)
            cn = primary_peer_cn(identities)
            if allowed and not any(name in allowed for name in identities):
                transport.close()
                return
            inner = self.app  # type: ignore[attr-defined]

            async def asgi_app(
                scope: dict[str, object],
                receive: Callable[..., object],
                send: Callable[..., object],
            ) -> None:
                if scope.get("type") == "http":
                    extensions = scope.get("extensions")
                    if not isinstance(extensions, dict):
                        extensions = {}
                        scope["extensions"] = extensions
                    tls_ext = extensions.get("tls")
                    if not isinstance(tls_ext, dict):
                        tls_ext = {}
                        extensions["tls"] = tls_ext
                    tls_ext["client_cn"] = cn
                    raw_headers = scope.get("headers", [])
                    headers: list[tuple[bytes, bytes]] = []
                    if isinstance(raw_headers, list):
                        headers = [(k, v) for k, v in raw_headers if k.lower() != _PEER_HEADER]
                    if cn:
                        headers.append((_PEER_HEADER, cn.encode("ascii")))
                    scope["headers"] = headers
                await inner(scope, receive, send)

            self.app = asgi_app  # type: ignore[attr-defined]

    InboundTLSProtocol.__name__ = f"InboundTLS{getattr(base, '__name__', 'Protocol')}"
    InboundTLSProtocol.__qualname__ = InboundTLSProtocol.__name__
    return InboundTLSProtocol


class InboundTLSUvicornConfig(uvicorn.Config):
    """uvicorn.Config that serves a caller-owned ``SSLContext``.

    File-based ``ssl_certfile`` is a startup-only load. We keep the context
    alive so ``load_cert_chain`` is visible on the next handshake.
    """

    def __init__(
        self,
        app: object,
        *,
        inbound_ssl_context: ssl.SSLContext,
        inbound_allowed_peers: tuple[str, ...] = (),
        host: str = "127.0.0.1",
        port: int = 9443,
        **kwargs: object,
    ) -> None:
        kwargs.setdefault("http", "h11")
        kwargs.setdefault("lifespan", "off")
        kwargs.setdefault("log_config", None)
        kwargs.setdefault("ws", "none")
        super().__init__(app, host=host, port=port, **kwargs)  # type: ignore[misc]
        self.inbound_ssl_context = inbound_ssl_context
        self.inbound_allowed_peers = inbound_allowed_peers

    def load(self) -> None:
        super().load()
        self.ssl = self.inbound_ssl_context
        self.http_protocol_class = wrap_inbound_tls_protocol(
            self.http_protocol_class,
            allowed_peers=self.inbound_allowed_peers,
        )


def build_inbound_uvicorn_config(
    app: object,
    ctx: ssl.SSLContext,
    *,
    host: str,
    port: int,
    allowed_peers: tuple[str, ...] = (),
) -> InboundTLSUvicornConfig:
    return InboundTLSUvicornConfig(
        app,
        inbound_ssl_context=ctx,
        inbound_allowed_peers=allowed_peers,
        host=host,
        port=port,
    )


async def serve_inbound_tls(  # noqa: PLR0913 — host/port/allowlist are the listener contract
    app: object,
    ctx: ssl.SSLContext,
    *,
    host: str,
    port: int,
    allowed_peers: tuple[str, ...] = (),
    app_state: dict[str, object] | None = None,
) -> asyncio.Server:
    """Bind ``host:port`` with the long-lived server ``SSLContext``."""
    config = build_inbound_uvicorn_config(app, ctx, host=host, port=port, allowed_peers=allowed_peers)
    config.load()
    loop = asyncio.get_running_loop()
    state = app_state if app_state is not None else {}
    server_state = ServerState()
    protocol_class: type[H11Protocol] = config.http_protocol_class  # type: ignore[assignment]

    def factory() -> asyncio.Protocol:
        return protocol_class(config=config, server_state=server_state, app_state=state)

    return await loop.create_server(factory, host=host, port=port, ssl=ctx)


async def watch_inbound_cert_rotation(
    reloader: InboundCertReloader,
    interval_seconds: float = 30.0,
) -> None:
    """Background poll so rotated leaves are picked up without a restart."""
    while True:
        try:
            await asyncio.sleep(interval_seconds)
            reloader.maybe_reload()
        except asyncio.CancelledError:
            raise
        except Exception:  # noqa: BLE001 — watcher must survive transient filesystem hiccups
            _logger.warning("inbound_tls_cert_rotation_iteration_failed", exc_info=True)
            continue


async def start_inbound_tls_listener(
    app: object,
    bind: InboundTLSBind,
) -> InboundTLSRuntime | None:
    """Start the mTLS listener, or log disabled and return None."""
    if not bind.enabled:
        _logger.info(
            "inbound_tls_disabled",
            reason=bind.disable_reason,
            plaintext_host=bind.plaintext_host,
            plaintext_port=bind.plaintext_port,
            tls_host=bind.tls_host,
            tls_port=bind.tls_port,
        )
        return None

    ctx = build_inbound_ssl_context(bind.cert_path, bind.key_path, bind.ca_path)
    reloader = InboundCertReloader(ctx, bind.cert_path, bind.key_path)
    server = await serve_inbound_tls(
        app,
        ctx,
        host=bind.tls_host,
        port=bind.tls_port,
        allowed_peers=bind.allowed_peers,
    )
    watch_task = asyncio.create_task(
        watch_inbound_cert_rotation(reloader),
        name="inbound-tls-cert-rotation-watch",
    )
    _logger.info(
        "inbound_tls_enabled",
        host=bind.tls_host,
        port=bind.tls_port,
        cert_path=bind.cert_path,
        plaintext_host=bind.plaintext_host,
        plaintext_port=bind.plaintext_port,
        allowed_peers=list(bind.allowed_peers),
    )
    return InboundTLSRuntime(server=server, reloader=reloader, watch_task=watch_task, ctx=ctx)
