"""Tests for the recap-evaluator mTLS outbound helper."""

from __future__ import annotations

import asyncio
import os
import socket
import ssl
import tempfile
import threading
from pathlib import Path

import httpx
import pytest
import structlog

from recap_evaluator.infra import mtls_client
from recap_evaluator.infra.mtls_client import (
    SslContextReloader,
    build_ssl_context,
    mtls_enforced,
    outbound_mtls,
    watch_cert_rotation,
)


def test_mtls_enforce_unset_fails_fast(monkeypatch):
    monkeypatch.delenv("MTLS_ENFORCE", raising=False)
    with pytest.raises(RuntimeError, match="MTLS_ENFORCE"):
        mtls_enforced()


@pytest.mark.parametrize("raw", ["", "yes", "1", "enabled"])
def test_mtls_enforce_garbage_fails_fast(monkeypatch, raw):
    monkeypatch.setenv("MTLS_ENFORCE", raw)
    with pytest.raises(RuntimeError, match="MTLS_ENFORCE"):
        mtls_enforced()


def test_mtls_enforced_true_when_env_set(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    assert mtls_enforced()


def test_mtls_enforced_false_only_when_explicitly_disabled(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "false")
    assert mtls_enforced() is False


def test_build_ssl_context_none_when_explicitly_disabled(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "false")
    assert build_ssl_context() is None


def test_build_ssl_context_fails_fast_when_enforce_unset(monkeypatch):
    monkeypatch.delenv("MTLS_ENFORCE", raising=False)
    with pytest.raises(RuntimeError, match="MTLS_ENFORCE"):
        build_ssl_context()


def test_build_ssl_context_fails_closed_when_paths_missing(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    for v in ("MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE"):
        monkeypatch.delenv(v, raising=False)

    with pytest.raises(RuntimeError, match="MTLS_CERT_FILE"):
        build_ssl_context()


def test_build_ssl_context_fails_closed_when_cert_unreadable(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    monkeypatch.setenv("MTLS_CERT_FILE", "/nonexistent/cert.pem")
    with tempfile.NamedTemporaryFile() as ca:
        monkeypatch.setenv("MTLS_KEY_FILE", ca.name)
        monkeypatch.setenv("MTLS_CA_FILE", ca.name)

        with pytest.raises((FileNotFoundError, ssl.SSLError, OSError)):
            build_ssl_context()


def _write_test_identity(dir_path: Path, cn: str) -> tuple[Path, Path]:
    """Generate a throwaway self-signed cert + key PEM pair under dir.

    Uses ``cryptography`` if available, else falls back to shelling out to
    openssl. The resulting pair is valid for TLS usage in tests.
    """
    cert_path = dir_path / f"{cn}-cert.pem"
    key_path = dir_path / f"{cn}-key.pem"

    try:
        import datetime

        from cryptography import x509
        from cryptography.hazmat.primitives import hashes, serialization
        from cryptography.hazmat.primitives.asymmetric import ec
        from cryptography.x509.oid import NameOID
    except ImportError:  # pragma: no cover — CI always has cryptography
        import subprocess

        subprocess.run(
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
                "1",
                "-nodes",
                "-subj",
                f"/CN={cn}",
            ],
            check=True,
            capture_output=True,
        )
        return cert_path, key_path

    key = ec.generate_private_key(ec.SECP256R1())
    subject = issuer = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)])
    cert = (
        x509.CertificateBuilder()
        .subject_name(subject)
        .issuer_name(issuer)
        .public_key(key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(datetime.datetime.now(datetime.UTC))
        .not_valid_after(datetime.datetime.now(datetime.UTC) + datetime.timedelta(days=1))
        .sign(key, hashes.SHA256())
    )
    cert_path.write_bytes(cert.public_bytes(serialization.Encoding.PEM))
    key_path.write_bytes(
        key.private_bytes(
            encoding=serialization.Encoding.PEM,
            format=serialization.PrivateFormat.PKCS8,
            encryption_algorithm=serialization.NoEncryption(),
        )
    )
    return cert_path, key_path


def test_ssl_context_reloader_maybe_reload_true_on_mtime_advance(tmp_path):
    cert, key = _write_test_identity(tmp_path, "initial")

    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))

    reloader = SslContextReloader(ctx, str(cert), str(key))
    assert reloader.maybe_reload() is False, "no-op when mtime unchanged"

    # Overwrite with fresh material and bump mtime into the future.
    new_cert, new_key = _write_test_identity(tmp_path, "rotated")
    cert.write_bytes(new_cert.read_bytes())
    key.write_bytes(new_key.read_bytes())
    future = reloader._cert_mtime + 2.0  # noqa: SLF001 — test-only access
    os.utime(cert, (future, future))
    os.utime(key, (future, future))

    assert reloader.maybe_reload() is True, "should reload when mtime advances"
    # Second call with no further changes must be a no-op.
    assert reloader.maybe_reload() is False


def test_ssl_context_reloader_swallows_transient_error(tmp_path):
    cert, key = _write_test_identity(tmp_path, "fallback")

    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))

    reloader = SslContextReloader(ctx, str(cert), str(key))

    # Truncate the cert (simulate mid-rotation window) and bump mtime.
    cert.write_bytes(b"not a pem")
    future = reloader._cert_mtime + 2.0  # noqa: SLF001 — test-only access
    os.utime(cert, (future, future))

    # maybe_reload must NOT raise; it returns False so the caller keeps
    # using the previously-installed cert.
    assert reloader.maybe_reload() is False


def test_watch_cert_rotation_cancels_cleanly(tmp_path):
    cert, key = _write_test_identity(tmp_path, "watch")

    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))
    reloader = SslContextReloader(ctx, str(cert), str(key))

    async def runner():
        task = asyncio.create_task(watch_cert_rotation(reloader, interval_seconds=60.0))
        # Give the task one event-loop tick to start sleeping.
        await asyncio.sleep(0)
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task

    asyncio.run(runner())


def _internal_pki(tmp_path: Path) -> dict[str, Path]:
    """Mint a throwaway CA, a news-creator server leaf, and a recap-evaluator
    client leaf, shaped like the step-ca leaves of the east-west mesh."""
    import datetime
    import ipaddress

    from cryptography import x509
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import ec
    from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID

    now = datetime.datetime.now(datetime.UTC)
    ca_key = ec.generate_private_key(ec.SECP256R1())
    ca_name = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, "alt-test-ca")])
    ca_ski = x509.SubjectKeyIdentifier.from_public_key(ca_key.public_key())
    ca_cert = (
        x509.CertificateBuilder()
        .subject_name(ca_name)
        .issuer_name(ca_name)
        .public_key(ca_key.public_key())
        .serial_number(x509.random_serial_number())
        .not_valid_before(now - datetime.timedelta(minutes=1))
        .not_valid_after(now + datetime.timedelta(hours=1))
        .add_extension(x509.BasicConstraints(ca=True, path_length=0), critical=True)
        .add_extension(
            x509.KeyUsage(
                digital_signature=False,
                content_commitment=False,
                key_encipherment=False,
                data_encipherment=False,
                key_agreement=False,
                key_cert_sign=True,
                crl_sign=True,
                encipher_only=False,
                decipher_only=False,
            ),
            critical=True,
        )
        .add_extension(ca_ski, critical=False)
        .sign(ca_key, hashes.SHA256())
    )

    def leaf(cn: str, sans: list, eku) -> tuple[bytes, bytes]:
        key = ec.generate_private_key(ec.SECP256R1())
        cert = (
            x509.CertificateBuilder()
            .subject_name(x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, cn)]))
            .issuer_name(ca_name)
            .public_key(key.public_key())
            .serial_number(x509.random_serial_number())
            .not_valid_before(now - datetime.timedelta(minutes=1))
            .not_valid_after(now + datetime.timedelta(hours=1))
            .add_extension(x509.SubjectAlternativeName(sans), critical=False)
            .add_extension(x509.BasicConstraints(ca=False, path_length=None), critical=True)
            .add_extension(x509.ExtendedKeyUsage([eku]), critical=False)
            .add_extension(
                x509.AuthorityKeyIdentifier.from_issuer_subject_key_identifier(ca_ski),
                critical=False,
            )
            .sign(ca_key, hashes.SHA256())
        )
        return (
            cert.public_bytes(serialization.Encoding.PEM),
            key.private_bytes(
                serialization.Encoding.PEM,
                serialization.PrivateFormat.PKCS8,
                serialization.NoEncryption(),
            ),
        )

    server_cert, server_key = leaf(
        "news-creator",
        [x509.DNSName("localhost"), x509.IPAddress(ipaddress.ip_address("127.0.0.1"))],
        ExtendedKeyUsageOID.SERVER_AUTH,
    )
    client_cert, client_key = leaf(
        "recap-evaluator",
        [x509.DNSName("recap-evaluator")],
        ExtendedKeyUsageOID.CLIENT_AUTH,
    )
    paths = {
        "ca": tmp_path / "ca-bundle.pem",
        "server_cert": tmp_path / "server-cert.pem",
        "server_key": tmp_path / "server-key.pem",
        "client_cert": tmp_path / "svc-cert.pem",
        "client_key": tmp_path / "svc-key.pem",
    }
    paths["ca"].write_bytes(ca_cert.public_bytes(serialization.Encoding.PEM))
    paths["server_cert"].write_bytes(server_cert)
    paths["server_key"].write_bytes(server_key)
    paths["client_cert"].write_bytes(client_cert)
    paths["client_key"].write_bytes(client_key)
    return paths


def _set_client_env(monkeypatch, pki: dict[str, Path]) -> None:
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    monkeypatch.setenv("MTLS_CERT_FILE", str(pki["client_cert"]))
    monkeypatch.setenv("MTLS_KEY_FILE", str(pki["client_key"]))
    monkeypatch.setenv("MTLS_CA_FILE", str(pki["ca"]))


def _serve_mtls_once(pki: dict[str, Path]) -> tuple[int, dict[str, str], threading.Thread]:
    """One-shot HTTPS server that, like news-creator :9443, refuses any
    client that does not present a leaf signed by the internal CA."""
    server_ctx = ssl.create_default_context(ssl.Purpose.CLIENT_AUTH, cafile=str(pki["ca"]))
    server_ctx.load_cert_chain(certfile=str(pki["server_cert"]), keyfile=str(pki["server_key"]))
    server_ctx.verify_mode = ssl.CERT_REQUIRED
    listener = socket.create_server(("127.0.0.1", 0))
    port = listener.getsockname()[1]
    seen: dict[str, str] = {}

    def serve() -> None:
        with listener:
            conn, _ = listener.accept()
            try:
                with server_ctx.wrap_socket(conn, server_side=True) as tls:
                    peer = tls.getpeercert() or {}
                    for rdn in peer.get("subject", ()):
                        for key, value in rdn:
                            if key == "commonName":
                                seen["peer_cn"] = value
                    tls.recv(65536)
                    tls.sendall(
                        b"HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\nok"
                    )
            except (ssl.SSLError, OSError) as exc:
                seen["error"] = repr(exc)

    thread = threading.Thread(target=serve, daemon=True)
    thread.start()
    return port, seen, thread


async def test_client_presents_cert_and_trusts_internal_ca_over_httpx(monkeypatch, tmp_path):
    pki = _internal_pki(tmp_path)
    _set_client_env(monkeypatch, pki)
    port, seen, thread = _serve_mtls_once(pki)

    ctx = build_ssl_context()
    assert ctx is not None
    async with httpx.AsyncClient(verify=ctx) as client:
        response = await client.get(f"https://localhost:{port}/api/tags")
    thread.join(timeout=5)

    assert response.status_code == 200
    assert seen.get("peer_cn") == "recap-evaluator", seen


class _FakeHandle:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls

    def stop(self) -> None:
        self._calls.append("pki_stop")


def _fake_enrollment(calls: list[str], *, publish: tuple[Path, Path, Path, Path] | None = None):
    """Stand-in for the in-process enrollment start(). When ``publish`` is
    given, it moves the leaf into place the way a real enroll writes it."""

    def start(service_name: str) -> _FakeHandle:
        calls.append(f"pki:{service_name}")
        if publish is not None:
            pending_cert, pending_key, cert_path, key_path = publish
            pending_cert.rename(cert_path)
            pending_key.rename(key_path)
        return _FakeHandle(calls)

    return start


async def test_outbound_mtls_enrolls_before_loading_client_cert(monkeypatch, tmp_path):
    pki = _internal_pki(tmp_path)
    _set_client_env(monkeypatch, pki)
    pending_cert = pki["client_cert"].rename(tmp_path / "pending-cert.pem")
    pending_key = pki["client_key"].rename(tmp_path / "pending-key.pem")
    calls: list[str] = []
    monkeypatch.setattr(
        mtls_client,
        "start_pki_enrollment",
        _fake_enrollment(
            calls,
            publish=(pending_cert, pending_key, pki["client_cert"], pki["client_key"]),
        ),
    )

    async with outbound_mtls(
        "recap-evaluator", upstreams={"OLLAMA_URL": "https://news-creator:9443"}
    ) as ctx:
        assert isinstance(ctx, ssl.SSLContext)
        ca_names = [
            value
            for ca in ctx.get_ca_certs()
            for rdn in ca["subject"]
            for key, value in rdn
            if key == "commonName"
        ]
        assert "alt-test-ca" in ca_names
        assert calls == ["pki:recap-evaluator"]

    assert calls == ["pki:recap-evaluator", "pki_stop"]


async def test_outbound_mtls_refuses_https_upstream_without_client_cert(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "false")
    calls: list[str] = []
    monkeypatch.setattr(mtls_client, "start_pki_enrollment", _fake_enrollment(calls))

    with pytest.raises(RuntimeError, match="OLLAMA_URL"):
        async with outbound_mtls(
            "recap-evaluator",
            upstreams={
                "OLLAMA_URL": "https://news-creator:9443",
                "RECAP_WORKER_URL": "http://recap-worker:9005",
            },
        ):
            pass

    assert calls == []


async def test_outbound_mtls_disabled_is_loud_and_plaintext_only(monkeypatch):
    monkeypatch.setenv("MTLS_ENFORCE", "false")
    calls: list[str] = []
    monkeypatch.setattr(mtls_client, "start_pki_enrollment", _fake_enrollment(calls))

    with structlog.testing.capture_logs() as logs:
        async with outbound_mtls(
            "recap-evaluator", upstreams={"OLLAMA_URL": "http://localhost:11434"}
        ) as ctx:
            assert ctx is None

    assert any(entry["event"] == "outbound_mtls_disabled" for entry in logs), logs
    assert calls == ["pki:recap-evaluator", "pki_stop"]


async def test_outbound_mtls_enabled_is_logged(monkeypatch, tmp_path):
    pki = _internal_pki(tmp_path)
    _set_client_env(monkeypatch, pki)
    calls: list[str] = []
    monkeypatch.setattr(mtls_client, "start_pki_enrollment", _fake_enrollment(calls))

    with structlog.testing.capture_logs() as logs:
        async with outbound_mtls(
            "recap-evaluator", upstreams={"OLLAMA_URL": "https://news-creator:9443"}
        ):
            pass

    assert any(entry["event"] == "outbound_mtls_enabled" for entry in logs), logs


async def test_outbound_mtls_stops_enrollment_when_client_cert_unloadable(monkeypatch, tmp_path):
    pki = _internal_pki(tmp_path)
    _set_client_env(monkeypatch, pki)
    monkeypatch.setenv("MTLS_CERT_FILE", str(tmp_path / "never-written.pem"))
    calls: list[str] = []
    monkeypatch.setattr(mtls_client, "start_pki_enrollment", _fake_enrollment(calls))

    with pytest.raises(FileNotFoundError):
        async with outbound_mtls(
            "recap-evaluator", upstreams={"OLLAMA_URL": "https://news-creator:9443"}
        ):
            pass

    # The renewal thread is non-daemon: leaving it running would keep the
    # process alive after a failed startup.
    assert calls == ["pki:recap-evaluator", "pki_stop"]


async def test_outbound_mtls_propagates_enrollment_failure(monkeypatch, tmp_path):
    pki = _internal_pki(tmp_path)
    _set_client_env(monkeypatch, pki)

    def failing_start(service_name: str) -> None:
        del service_name
        raise RuntimeError("pki: enroll failed after 5 attempts")

    monkeypatch.setattr(mtls_client, "start_pki_enrollment", failing_start)

    with pytest.raises(RuntimeError, match="enroll failed"):
        async with outbound_mtls(
            "recap-evaluator", upstreams={"OLLAMA_URL": "https://news-creator:9443"}
        ):
            pass
