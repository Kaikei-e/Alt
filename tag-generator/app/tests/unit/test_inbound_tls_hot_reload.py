"""Wave 4 RED: inbound TLS hot-reload without process restart.

Handshake after an atomic cert replace must present the new NotAfter.
Pattern B currently terminates TLS in pki-agent; this test fails until
the in-process server context exists. Do not catch NotImplementedError —
that is the right RED.
"""

from __future__ import annotations

import asyncio
import os
import socket
import ssl
import subprocess
import threading
import time
from pathlib import Path

import pytest
from starlette.applications import Starlette
from starlette.responses import PlainTextResponse
from starlette.routing import Route

from tag_generator.infra.inbound_tls import (
    InboundCertReloader,
    build_inbound_ssl_context,
    build_inbound_uvicorn_config,
    resolve_inbound_tls_bind,
    start_inbound_tls_listener,
)


def _write_leaf(dir_path: Path, cn: str, days: int) -> tuple[Path, Path, Path]:
    """Self-signed leaf used as both server cert and trust anchor."""
    cert_path = dir_path / f"{cn}-{days}d-cert.pem"
    key_path = dir_path / f"{cn}-{days}d-key.pem"
    subprocess.run(  # noqa: S603, S607 — test-only openssl
        [
            "openssl",  # noqa: S607 — test-only openssl
            "req",
            "-x509",
            "-newkey",
            "rsa:2048",
            "-keyout",
            str(key_path),
            "-out",
            str(cert_path),
            "-days",
            str(days),
            "-nodes",
            "-subj",
            f"/CN={cn}",
            "-addext",
            "subjectAltName=DNS:localhost,IP:127.0.0.1",
        ],
        check=True,
        capture_output=True,
    )
    ca_path = dir_path / f"{cn}-{days}d-ca.pem"
    ca_path.write_bytes(cert_path.read_bytes())
    return cert_path, key_path, ca_path


def _atomic_replace(dest: Path, src: Path) -> None:
    tmp = dest.with_suffix(dest.suffix + ".tmp")
    tmp.write_bytes(src.read_bytes())
    os.replace(tmp, dest)


def _not_after_from_der(der: bytes) -> str:
    pem = ssl.DER_cert_to_PEM_cert(der)
    out = subprocess.run(  # noqa: S603, S607
        ["openssl", "x509", "-noout", "-enddate"],  # noqa: S607
        input=pem,
        check=True,
        capture_output=True,
        text=True,
    )
    # "notAfter=Aug 19 10:00:00 2026 GMT"
    return out.stdout.strip().split("=", 1)[1]


def _handshake_enddate(port: int, client_cert: Path, client_key: Path) -> str:
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    ctx.load_cert_chain(certfile=str(client_cert), keyfile=str(client_key))
    with socket.create_connection(("127.0.0.1", port), timeout=5) as sock:
        with ctx.wrap_socket(sock, server_hostname="localhost") as ssock:
            der = ssock.getpeercert(binary_form=True)
            assert der is not None
            return _not_after_from_der(der)


class _TLSServer:
    def __init__(self, ctx: ssl.SSLContext) -> None:
        self._ctx = ctx
        self._sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        self._sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        self._sock.bind(("127.0.0.1", 0))
        self._sock.listen(8)
        self._sock.settimeout(0.2)
        self.port = self._sock.getsockname()[1]
        self._stop = threading.Event()
        self._thread = threading.Thread(target=self._loop, daemon=True)
        self.thread_ident: int | None = None

    def start(self) -> None:
        self._thread.start()

    def _loop(self) -> None:
        self.thread_ident = threading.get_ident()
        while not self._stop.is_set():
            try:
                conn, _addr = self._sock.accept()
            except TimeoutError:
                continue
            try:
                with self._ctx.wrap_socket(conn, server_side=True) as ssock:
                    ssock.read(1)
            except (
                ssl.SSLError,
                OSError,
                TimeoutError,
            ):
                pass
            finally:
                conn.close()

    def close(self) -> None:
        self._stop.set()
        self._thread.join(timeout=2)
        self._sock.close()


def test_inbound_handshake_shows_new_not_after_after_atomic_replace(tmp_path: Path) -> None:
    """Wave 4 acceptance: same process, new NotAfter after atomic rename."""
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    dest_cert = tmp_path / "svc-cert.pem"
    dest_key = tmp_path / "svc-key.pem"
    dest_ca = tmp_path / "ca-bundle.pem"
    dest_cert.write_bytes(cert.read_bytes())
    dest_key.write_bytes(key.read_bytes())
    dest_ca.write_bytes(ca.read_bytes())
    # Stable client identity: not rotated, still trusted by dest_ca.
    client_cert = tmp_path / "client-cert.pem"
    client_key = tmp_path / "client-key.pem"
    client_cert.write_bytes(cert.read_bytes())
    client_key.write_bytes(key.read_bytes())

    ctx = build_inbound_ssl_context(str(dest_cert), str(dest_key), str(dest_ca))
    reloader = InboundCertReloader(ctx, str(dest_cert), str(dest_key))
    server = _TLSServer(ctx)
    server.start()
    try:
        first = _handshake_enddate(server.port, client_cert, client_key)

        new_cert, new_key, _new_ca = _write_leaf(tmp_path, "tag-generator", days=30)
        _atomic_replace(dest_cert, new_cert)
        _atomic_replace(dest_key, new_key)
        future = time.time() + 2.0
        os.utime(dest_cert, (future, future))
        os.utime(dest_key, (future, future))

        assert reloader.maybe_reload() is True, "reloader must pick up the atomic replace"
        second = _handshake_enddate(server.port, client_cert, client_key)
        assert second != first, "inbound handshake must present the rotated NotAfter without restarting the process"
        assert server.thread_ident is not None
        assert server._thread.is_alive(), "listener thread must survive cert rotation"  # noqa: SLF001
    finally:
        server.close()


def test_inbound_listener_rejects_plaintext(tmp_path: Path) -> None:
    """No plaintext fallback on the inbound port — TLS handshake is mandatory."""
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    server = _TLSServer(ctx)
    server.start()
    try:
        with socket.create_connection(("127.0.0.1", server.port), timeout=5) as sock:
            sock.sendall(b"GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n")
            sock.settimeout(2)
            try:
                data = sock.recv(64)
            except (
                TimeoutError,
                ConnectionResetError,
                BrokenPipeError,
                OSError,
            ):
                data = b""
        assert not data.startswith(b"HTTP/"), (
            "plaintext HTTP on the inbound TLS port is forbidden; Wave 4 must not add an http fallback next to mTLS"
        )
    finally:
        server.close()


def test_inbound_ssl_context_fails_closed_on_missing_cert(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    with pytest.raises(RuntimeError, match="cert"):
        build_inbound_ssl_context(str(tmp_path / "missing-cert.pem"), str(key), str(ca))


def test_inbound_ssl_context_fails_closed_on_missing_key(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    with pytest.raises(RuntimeError, match="key"):
        build_inbound_ssl_context(str(cert), str(tmp_path / "missing-key.pem"), str(ca))


def test_inbound_ssl_context_fails_closed_on_empty_ca(tmp_path: Path) -> None:
    cert, key, _ca = _write_leaf(tmp_path, "tag-generator", days=1)
    empty_ca = tmp_path / "empty-ca.pem"
    empty_ca.write_text("not a certificate\n")
    with pytest.raises(RuntimeError, match="CA bundle"):
        build_inbound_ssl_context(str(cert), str(key), str(empty_ca))


def test_inbound_ssl_context_is_tls13_and_requires_client_cert(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    assert ctx.minimum_version == ssl.TLSVersion.TLSv1_3
    assert ctx.verify_mode == ssl.CERT_REQUIRED


def test_inbound_listener_rejects_tls12(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    server = _TLSServer(ctx)
    server.start()
    try:
        client = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        client.maximum_version = ssl.TLSVersion.TLSv1_2
        client.check_hostname = False
        client.verify_mode = ssl.CERT_NONE
        client.load_cert_chain(str(cert), str(key))
        with socket.create_connection(("127.0.0.1", server.port), timeout=5) as sock:
            with pytest.raises(ssl.SSLError):
                client.wrap_socket(sock, server_hostname="localhost")
    finally:
        server.close()


def test_inbound_listener_rejects_untrusted_client_cert(tmp_path: Path) -> None:
    server_cert, server_key, server_ca = _write_leaf(tmp_path, "tag-generator", days=1)
    evil_cert, evil_key, _evil_ca = _write_leaf(tmp_path, "evil-client", days=1)
    ctx = build_inbound_ssl_context(str(server_cert), str(server_key), str(server_ca))
    server = _TLSServer(ctx)
    server.start()
    try:
        client = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        client.check_hostname = False
        client.verify_mode = ssl.CERT_NONE
        client.load_cert_chain(str(evil_cert), str(evil_key))
        with socket.create_connection(("127.0.0.1", server.port), timeout=5) as sock:
            ssock = client.wrap_socket(sock, server_hostname="localhost")
            with pytest.raises((ssl.SSLError, OSError, ConnectionResetError, BrokenPipeError)):
                ssock.sendall(b"x")
                ssock.read(1)
    finally:
        server.close()


def test_inbound_reloader_noop_when_mtime_unchanged(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    reloader = InboundCertReloader(ctx, str(cert), str(key))
    assert reloader.maybe_reload() is False


def test_inbound_reloader_keeps_previous_chain_on_garbage(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    reloader = InboundCertReloader(ctx, str(cert), str(key))
    cert.write_bytes(b"not a pem")
    os.utime(cert, (time.time() + 2.0, time.time() + 2.0))
    assert reloader.maybe_reload() is False
    client_cert = tmp_path / "client-cert.pem"
    client_key = tmp_path / "client-key.pem"
    client_cert.write_bytes(ca.read_bytes())
    client_key.write_bytes(key.read_bytes())
    server = _TLSServer(ctx)
    server.start()
    try:
        _handshake_enddate(server.port, client_cert, client_key)
    finally:
        server.close()


def test_inbound_uvicorn_config_reuses_sslcontext(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    app = Starlette(routes=[Route("/health", lambda _r: PlainTextResponse("ok"))])
    config = build_inbound_uvicorn_config(app, ctx, host="127.0.0.1", port=0)
    config.load()
    assert config.ssl is ctx
    assert config.ssl_certfile is None


def test_resolve_inbound_tls_bind_disabled_when_unset() -> None:
    bind = resolve_inbound_tls_bind(
        plaintext_host="0.0.0.0",  # noqa: S104
        plaintext_port=9400,
        environ={},
    )
    assert bind.enabled is False
    assert "unset" in bind.disable_reason
    assert bind.plaintext_port == 9400
    assert bind.tls_port == 9443


def test_resolve_inbound_tls_bind_fails_when_enrollment_enabled_and_inbound_unset() -> None:
    with pytest.raises(RuntimeError, match="INBOUND_TLS_ENABLED"):
        resolve_inbound_tls_bind(
            plaintext_host="0.0.0.0",  # noqa: S104
            plaintext_port=9400,
            environ={"PKI_ENROLLMENT": "enabled"},
        )


def test_resolve_inbound_tls_bind_fails_when_enrollment_enabled_and_inbound_false() -> None:
    with pytest.raises(RuntimeError, match="INBOUND_TLS_ENABLED"):
        resolve_inbound_tls_bind(
            plaintext_host="0.0.0.0",  # noqa: S104
            plaintext_port=9400,
            environ={"PKI_ENROLLMENT": "enabled", "INBOUND_TLS_ENABLED": "false"},
        )


def test_resolve_inbound_tls_bind_fails_closed_when_enabled_without_certs() -> None:
    with pytest.raises(RuntimeError, match="cert"):
        resolve_inbound_tls_bind(
            plaintext_host="127.0.0.1",
            plaintext_port=9400,
            environ={"INBOUND_TLS_ENABLED": "true"},
        )


def test_resolve_inbound_tls_bind_rejects_garbage_flag() -> None:
    with pytest.raises(RuntimeError, match="true or false"):
        resolve_inbound_tls_bind(
            plaintext_host="127.0.0.1",
            plaintext_port=9400,
            environ={"INBOUND_TLS_ENABLED": "maybe"},
        )


@pytest.mark.asyncio
async def test_start_inbound_tls_listener_disabled_returns_none() -> None:
    bind = resolve_inbound_tls_bind(plaintext_host="127.0.0.1", plaintext_port=9400, environ={})
    app = Starlette(routes=[Route("/health", lambda _r: PlainTextResponse("ok"))])
    assert await start_inbound_tls_listener(app, bind) is None


@pytest.mark.asyncio
async def test_start_inbound_tls_listener_serves_and_aclose(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "tag-generator", days=1)
    bind = resolve_inbound_tls_bind(
        plaintext_host="127.0.0.1",
        plaintext_port=9400,
        environ={
            "INBOUND_TLS_ENABLED": "true",
            "INBOUND_TLS_HOST": "127.0.0.1",
            "INBOUND_TLS_PORT": "0",
            "INBOUND_TLS_CERT_FILE": str(cert),
            "INBOUND_TLS_KEY_FILE": str(key),
            "INBOUND_TLS_CA_FILE": str(ca),
        },
    )
    app = Starlette(routes=[Route("/health", lambda _r: PlainTextResponse("ok"))])
    runtime = await start_inbound_tls_listener(app, bind)
    assert runtime is not None
    try:
        port = runtime.server.sockets[0].getsockname()[1]
        await asyncio.to_thread(_handshake_enddate, port, cert, key)
    finally:
        await runtime.aclose()
