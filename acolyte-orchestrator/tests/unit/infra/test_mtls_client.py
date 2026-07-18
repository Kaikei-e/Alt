"""Tests for the acolyte-orchestrator mTLS outbound helper."""

from __future__ import annotations

import asyncio
import os
import ssl
import subprocess
import tempfile
from pathlib import Path
from unittest.mock import MagicMock, patch

import pytest

from acolyte.infra.mtls_client import (
    SslContextReloader,
    build_ssl_context,
    mtls_enforced,
    watch_cert_rotation,
)


def _write_test_identity(dir_path: Path, cn: str) -> tuple[Path, Path]:
    """Generate a throwaway self-signed cert + key PEM pair via openssl."""
    cert_path = dir_path / f"{cn}-cert.pem"
    key_path = dir_path / f"{cn}-key.pem"
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


def test_mtls_enforced_false_by_default() -> None:
    os.environ.pop("MTLS_ENFORCE", None)
    assert not mtls_enforced()


def test_mtls_enforced_true_when_env_set() -> None:
    os.environ["MTLS_ENFORCE"] = "true"
    try:
        assert mtls_enforced()
    finally:
        os.environ.pop("MTLS_ENFORCE", None)


def test_build_ssl_context_none_when_not_enforced() -> None:
    os.environ.pop("MTLS_ENFORCE", None)
    assert build_ssl_context() is None


def test_build_ssl_context_fails_closed_when_paths_missing() -> None:
    os.environ["MTLS_ENFORCE"] = "true"
    for v in ("MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE"):
        os.environ.pop(v, None)
    try:
        with pytest.raises(RuntimeError, match="MTLS_CERT_FILE"):
            build_ssl_context()
    finally:
        os.environ.pop("MTLS_ENFORCE", None)


def test_build_ssl_context_fails_closed_when_cert_unreadable() -> None:
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


def test_ssl_context_reloader_reloads_on_mtime_advance(tmp_path: Path) -> None:
    cert, key = _write_test_identity(tmp_path, "initial")
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))

    reloader = SslContextReloader(ctx, str(cert), str(key))
    assert reloader.maybe_reload() is False

    new_cert, new_key = _write_test_identity(tmp_path, "rotated")
    cert.write_bytes(new_cert.read_bytes())
    key.write_bytes(new_key.read_bytes())
    future = reloader._cert_mtime + 2.0
    os.utime(cert, (future, future))
    os.utime(key, (future, future))

    assert reloader.maybe_reload() is True
    assert reloader.maybe_reload() is False


def test_ssl_context_reloader_swallows_transient_error(tmp_path: Path) -> None:
    cert, key = _write_test_identity(tmp_path, "fallback")
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))

    reloader = SslContextReloader(ctx, str(cert), str(key))
    cert.write_bytes(b"not a pem")
    future = reloader._cert_mtime + 2.0
    os.utime(cert, (future, future))

    assert reloader.maybe_reload() is False


def test_watch_cert_rotation_cancels_cleanly(tmp_path: Path) -> None:
    cert, key = _write_test_identity(tmp_path, "watch")
    ctx = ssl.create_default_context(ssl.Purpose.SERVER_AUTH)
    ctx.load_cert_chain(certfile=str(cert), keyfile=str(key))
    reloader = SslContextReloader(ctx, str(cert), str(key))

    async def runner() -> None:
        task = asyncio.create_task(watch_cert_rotation(reloader, interval_seconds=60.0))
        await asyncio.sleep(0)
        task.cancel()
        with pytest.raises(asyncio.CancelledError):
            await task

    asyncio.run(runner())


def test_watch_cert_rotation_logs_warning_on_failure(tmp_path: Path) -> None:
    """Transient watcher failures must be visible at WARNING (not debug-only)."""
    reloader = MagicMock()
    reloader.maybe_reload.side_effect = OSError("boom")

    with patch("acolyte.infra.mtls_client._logger") as mock_logger:

        async def runner() -> None:
            task = asyncio.create_task(watch_cert_rotation(reloader, interval_seconds=0.01))
            await asyncio.sleep(0.05)
            task.cancel()
            with pytest.raises(asyncio.CancelledError):
                await task

        asyncio.run(runner())

    mock_logger.warning.assert_called()
    assert any("cert_rotation_iteration_failed" in str(c) for c in mock_logger.warning.call_args_list)
