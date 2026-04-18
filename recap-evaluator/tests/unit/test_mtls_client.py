"""Tests for the recap-evaluator mTLS outbound helper."""

from __future__ import annotations

import asyncio
import os
import ssl
import tempfile
from pathlib import Path

import pytest

from recap_evaluator.infra.mtls_client import (
    SslContextReloader,
    build_ssl_context,
    mtls_enforced,
    watch_cert_rotation,
)


def test_mtls_enforced_false_by_default():
    os.environ.pop("MTLS_ENFORCE", None)
    assert not mtls_enforced()


def test_mtls_enforced_true_when_env_set():
    os.environ["MTLS_ENFORCE"] = "true"
    try:
        assert mtls_enforced()
    finally:
        os.environ.pop("MTLS_ENFORCE", None)


def test_build_ssl_context_none_when_not_enforced():
    os.environ.pop("MTLS_ENFORCE", None)
    assert build_ssl_context() is None


def test_build_ssl_context_fails_closed_when_paths_missing():
    os.environ["MTLS_ENFORCE"] = "true"
    for v in ("MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE"):
        os.environ.pop(v, None)
    try:
        with pytest.raises(RuntimeError, match="MTLS_CERT_FILE"):
            build_ssl_context()
    finally:
        os.environ.pop("MTLS_ENFORCE", None)


def test_build_ssl_context_fails_closed_when_cert_unreadable():
    os.environ["MTLS_ENFORCE"] = "true"
    os.environ["MTLS_CERT_FILE"] = "/nonexistent/cert.pem"
    with tempfile.NamedTemporaryFile() as ca:
        os.environ["MTLS_KEY_FILE"] = ca.name
        os.environ["MTLS_CA_FILE"] = ca.name
        try:
            with pytest.raises((FileNotFoundError, ssl.SSLError, OSError)):
                build_ssl_context()
        finally:
            for v in ("MTLS_ENFORCE", "MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE"):
                os.environ.pop(v, None)


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
        .not_valid_before(datetime.datetime.now(datetime.timezone.utc))
        .not_valid_after(
            datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=1)
        )
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
