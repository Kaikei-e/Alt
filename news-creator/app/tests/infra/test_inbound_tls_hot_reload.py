"""Wave 4 RED: inbound TLS hot-reload without process restart.

Handshake after an atomic cert replace must present the new NotAfter.
Pattern B currently terminates TLS in pki-agent; this test fails until
the in-process server context exists. Do not catch NotImplementedError —
that is the right RED.
"""

from __future__ import annotations

import contextlib
import os
import socket
import ssl
import subprocess
import threading
import time
from pathlib import Path

import pytest

from news_creator.infra.inbound_tls import (
    InboundCertReloader,
    build_inbound_ssl_context,
    inbound_listener_timeouts,
    load_inbound_ssl_context_from_env,
)


def _write_leaf(dir_path: Path, cn: str, days: int) -> tuple[Path, Path, Path]:
    """Self-signed leaf used as both server cert and trust anchor."""
    cert_path = dir_path / f"{cn}-{days}d-cert.pem"
    key_path = dir_path / f"{cn}-{days}d-key.pem"
    subprocess.run(  # noqa: S603, S607 — test-only openssl
        [
            "openssl",
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
        ["openssl", "x509", "-noout", "-enddate"],
        input=pem,
        check=True,
        capture_output=True,
        text=True,
    )
    # "notAfter=Aug 19 10:00:00 2026 GMT"
    return out.stdout.strip().split("=", 1)[1]


def _handshake_enddate(port: int) -> str:
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_2
    ctx.maximum_version = ssl.TLSVersion.TLSv1_3
    ctx.check_hostname = False
    ctx.verify_mode = ssl.CERT_NONE
    assert ctx.minimum_version >= ssl.TLSVersion.TLSv1_2
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
            except (ssl.SSLError, OSError, TimeoutError):
                pass
            finally:
                conn.close()

    def close(self) -> None:
        self._stop.set()
        self._thread.join(timeout=2)
        self._sock.close()


def test_inbound_handshake_shows_new_not_after_after_atomic_replace(
    tmp_path: Path,
) -> None:
    """Wave 4 acceptance: same process, new NotAfter after atomic rename."""
    cert, key, ca = _write_leaf(tmp_path, "news-creator", days=1)
    dest_cert = tmp_path / "svc-cert.pem"
    dest_key = tmp_path / "svc-key.pem"
    dest_ca = tmp_path / "ca-bundle.pem"
    dest_cert.write_bytes(cert.read_bytes())
    dest_key.write_bytes(key.read_bytes())
    dest_ca.write_bytes(ca.read_bytes())

    ctx = build_inbound_ssl_context(str(dest_cert), str(dest_key), str(dest_ca))
    reloader = InboundCertReloader(ctx, str(dest_cert), str(dest_key))
    server = _TLSServer(ctx)
    server.start()
    try:
        first = _handshake_enddate(server.port)

        new_cert, new_key, _new_ca = _write_leaf(tmp_path, "news-creator", days=30)
        _atomic_replace(dest_cert, new_cert)
        _atomic_replace(dest_key, new_key)
        future = time.time() + 2.0
        os.utime(dest_cert, (future, future))
        os.utime(dest_key, (future, future))

        assert reloader.maybe_reload() is True, (
            "reloader must pick up the atomic replace"
        )
        second = _handshake_enddate(server.port)
        assert second != first, (
            "inbound handshake must present the rotated NotAfter without "
            "restarting the process"
        )
        assert server.thread_ident is not None
        assert server._thread.is_alive(), "listener thread must survive cert rotation"  # noqa: SLF001
    finally:
        server.close()


def test_inbound_listener_rejects_plaintext(tmp_path: Path) -> None:
    """No plaintext fallback on the inbound port — TLS handshake is mandatory."""
    cert, key, ca = _write_leaf(tmp_path, "news-creator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    server = _TLSServer(ctx)
    server.start()
    try:
        with socket.create_connection(("127.0.0.1", server.port), timeout=5) as sock:
            sock.sendall(b"GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n")
            sock.settimeout(2)
            try:
                data = sock.recv(64)
            except (TimeoutError, ConnectionResetError, BrokenPipeError):
                data = b""
        assert not data.startswith(b"HTTP/"), (
            "plaintext HTTP on the inbound TLS port is forbidden; "
            "Wave 4 must not add an http fallback next to mTLS"
        )
    finally:
        server.close()


def test_inbound_reloader_swallows_transient_parse_error(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "news-creator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    reloader = InboundCertReloader(ctx, str(cert), str(key))
    cert.write_bytes(b"not a pem")
    future = time.time() + 2.0
    os.utime(cert, (future, future))
    assert reloader.maybe_reload() is False


def test_inbound_rejects_tls12(tmp_path: Path) -> None:
    cert, key, ca = _write_leaf(tmp_path, "news-creator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    server = _TLSServer(ctx)
    server.start()
    try:
        client_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        client_ctx.minimum_version = ssl.TLSVersion.TLSv1_2
        client_ctx.maximum_version = ssl.TLSVersion.TLSv1_2
        client_ctx.check_hostname = False
        client_ctx.verify_mode = ssl.CERT_NONE
        assert client_ctx.minimum_version >= ssl.TLSVersion.TLSv1_2
        with (
            socket.create_connection(("127.0.0.1", server.port), timeout=5) as sock,
            pytest.raises(ssl.SSLError),
        ):
            client_ctx.wrap_socket(sock, server_hostname="localhost")
    finally:
        server.close()


def test_inbound_listener_has_no_response_header_timeout() -> None:
    timeouts = inbound_listener_timeouts()
    assert timeouts.response_header_timeout is None, (
        "in-process inbound TLS must not reintroduce pki-agent "
        "PROXY_RESPONSE_HEADER_TIMEOUT; long LLM calls would 504"
    )


def test_slow_first_byte_is_not_cut_by_header_timeout(tmp_path: Path) -> None:
    """A delayed first response byte must still complete.

    pki-agent's default ResponseHeaderTimeout is 15s; news-creator's sidecar
    raises that to 960s. The in-process listener has no such cutoff, so a
    request that waits 2s before writing headers must return 200.
    """
    cert, key, ca = _write_leaf(tmp_path, "news-creator", days=1)
    ctx = build_inbound_ssl_context(str(cert), str(key), str(ca))
    sock = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
    sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    sock.bind(("127.0.0.1", 0))
    sock.listen(1)
    sock.settimeout(5)
    port = sock.getsockname()[1]
    delay = 2.0
    stop = threading.Event()

    def loop() -> None:
        try:
            conn, _addr = sock.accept()
        except TimeoutError:
            return
        try:
            with ctx.wrap_socket(conn, server_side=True) as ssock:
                ssock.settimeout(5)
                with contextlib.suppress(ssl.SSLError, TimeoutError, OSError):
                    ssock.read(1)
                time.sleep(delay)
                ssock.sendall(b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nok")
        except (ssl.SSLError, OSError, TimeoutError):
            pass
        finally:
            conn.close()
            stop.set()

    thread = threading.Thread(target=loop, daemon=True)
    thread.start()
    try:
        client_ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
        client_ctx.minimum_version = ssl.TLSVersion.TLSv1_2
        client_ctx.maximum_version = ssl.TLSVersion.TLSv1_3
        client_ctx.check_hostname = False
        client_ctx.verify_mode = ssl.CERT_NONE
        assert client_ctx.minimum_version >= ssl.TLSVersion.TLSv1_2
        client_ctx.load_cert_chain(str(cert), str(key))
        started = time.monotonic()
        with (
            socket.create_connection(("127.0.0.1", port), timeout=10) as raw,
            client_ctx.wrap_socket(raw, server_hostname="localhost") as ssock,
        ):
            ssock.sendall(b"GET /health HTTP/1.1\r\nHost: localhost\r\n\r\n")
            ssock.settimeout(10)
            data = ssock.recv(64)
        elapsed = time.monotonic() - started
        assert elapsed >= delay * 0.8, "server must have waited before headers"
        assert data.startswith(b"HTTP/1.1 200"), data
        assert inbound_listener_timeouts().response_header_timeout is None
    finally:
        stop.wait(timeout=2)
        thread.join(timeout=2)
        sock.close()


def test_load_inbound_ssl_context_from_env_disabled(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.delenv("INBOUND_MTLS", raising=False)
    assert load_inbound_ssl_context_from_env() is None


def test_load_inbound_ssl_context_from_env_fails_closed_missing_paths(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.delenv("MTLS_CERT_FILE", raising=False)
    monkeypatch.delenv("MTLS_KEY_FILE", raising=False)
    monkeypatch.delenv("MTLS_CA_FILE", raising=False)
    with pytest.raises(RuntimeError, match="MTLS_CERT_FILE"):
        load_inbound_ssl_context_from_env()


def test_load_inbound_ssl_context_from_env_fails_closed_empty_allowlist(
    monkeypatch: pytest.MonkeyPatch, tmp_path: Path
) -> None:
    cert, key, ca = _write_leaf(tmp_path, "news-creator", days=1)
    monkeypatch.setenv("INBOUND_MTLS", "true")
    monkeypatch.setenv("MTLS_CERT_FILE", str(cert))
    monkeypatch.setenv("MTLS_KEY_FILE", str(key))
    monkeypatch.setenv("MTLS_CA_FILE", str(ca))
    monkeypatch.setenv("MTLS_ALLOWED_PEERS", "  ,  ")
    with pytest.raises(RuntimeError, match="MTLS_ALLOWED_PEERS"):
        load_inbound_ssl_context_from_env()
