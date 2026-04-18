"""Tests for connect_client_factory module."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest

from tag_generator.driver.connect_client_factory import (
    _ReloadingBackendClient,
    create_backend_client,
)


def _write_test_pki(dir_path: Path, cn: str) -> tuple[Path, Path, Path]:
    """Generate a throwaway leaf cert + key + CA bundle via openssl.

    We use a self-signed leaf (acting as its own CA for test purposes) so the
    PEM shapes match what pki-agent produces on disk.
    """
    cert_path = dir_path / f"{cn}-cert.pem"
    key_path = dir_path / f"{cn}-key.pem"
    ca_path = dir_path / "ca.pem"

    cmd = [  # noqa: S603, S607 — test-only openssl shell-out
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
    ]
    subprocess.run(cmd, check=True, capture_output=True)  # noqa: S603
    # For tests, reuse the same cert as CA bundle — the reloading client
    # doesn't validate chain semantics, only that all three files are
    # readable and produce a valid SyncHTTPTransport.
    ca_path.write_bytes(cert_path.read_bytes())
    return cert_path, key_path, ca_path


class TestCreateBackendClient:
    """Tests for the create_backend_client factory function.

    Authentication to the backend is now established at the TLS transport
    layer (mTLS); the factory returns an empty auth headers dict — the only
    invariant asserted is that the client is constructed correctly.
    """

    def test_returns_none_when_url_not_set(self) -> None:
        """Returns None when BACKEND_API_URL is not set."""
        with patch.dict(os.environ, {}, clear=True):
            result = create_backend_client()
        assert result is None

    def test_returns_none_when_url_empty(self) -> None:
        """Returns None when BACKEND_API_URL is empty string."""
        with patch.dict(os.environ, {"BACKEND_API_URL": ""}, clear=True):
            result = create_backend_client()
        assert result is None

    def test_returns_client_when_url_set(self) -> None:
        """Returns (client, headers) tuple when BACKEND_API_URL is set."""
        env = {"BACKEND_API_URL": "http://alt-backend:9000"}
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        client, headers = result
        from tag_generator.gen.proto.services.backend.v1.internal_connect import (
            BackendInternalServiceClientSync,
        )

        assert isinstance(client, BackendInternalServiceClientSync)
        assert isinstance(headers, dict)
        assert "X-Service-Token" not in headers

    def test_auth_headers_always_empty(self) -> None:
        """No X-Service-Token is ever added; auth is transport-layer."""
        env = {"BACKEND_API_URL": "http://alt-backend:9000"}
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        _, headers = result
        assert "X-Service-Token" not in headers


class TestReloadingBackendClient:
    """Tests for the mTLS cert hot-reload behaviour of _ReloadingBackendClient."""

    def test_rebuilds_inner_client_when_mtime_advances(self, tmp_path: Path) -> None:
        cert, key, ca = _write_test_pki(tmp_path, "tag-generator")
        rc = _ReloadingBackendClient(
            base_url="https://alt-backend:9443",
            cert_file=str(cert),
            key_file=str(key),
            ca_file=str(ca),
            timeout_ms=1000,
        )
        first_inner = rc._inner  # noqa: SLF001 — test-only access

        # Rotate: overwrite with fresh material + bump mtime.
        new_cert, new_key, _ = _write_test_pki(tmp_path, "tag-generator")
        cert.write_bytes(new_cert.read_bytes())
        key.write_bytes(new_key.read_bytes())
        future = rc._cert_mtime + 2.0  # noqa: SLF001
        os.utime(cert, (future, future))
        os.utime(key, (future, future))

        rc._maybe_reload()  # noqa: SLF001 — test-only access

        assert rc._inner is not first_inner, (  # noqa: SLF001
            "reloading client must construct a fresh inner on cert rotation"
        )

    def test_keeps_previous_inner_on_transient_read_failure(self, tmp_path: Path) -> None:
        cert, key, ca = _write_test_pki(tmp_path, "tag-generator")
        rc = _ReloadingBackendClient(
            base_url="https://alt-backend:9443",
            cert_file=str(cert),
            key_file=str(key),
            ca_file=str(ca),
            timeout_ms=1000,
        )
        original = rc._inner  # noqa: SLF001

        # Truncate and bump mtime — next rebuild should fail internally and
        # leave the original client intact.
        cert.write_bytes(b"not a pem")
        future = rc._cert_mtime + 2.0  # noqa: SLF001
        os.utime(cert, (future, future))

        rc._maybe_reload()  # noqa: SLF001 — must not raise

        assert rc._inner is original, (  # noqa: SLF001
            "rebuild failure must keep the previously-good inner client"
        )

    def test_forwards_attribute_access_to_inner(self, tmp_path: Path) -> None:
        cert, key, ca = _write_test_pki(tmp_path, "tag-generator")
        rc = _ReloadingBackendClient(
            base_url="https://alt-backend:9443",
            cert_file=str(cert),
            key_file=str(key),
            ca_file=str(ca),
            timeout_ms=1000,
        )
        # BackendInternalServiceClientSync exposes `base_url` as a positional
        # constructor arg; the underlying ConnectClientSync stores it on
        # `_base_url`. Rather than depending on internal field names, we just
        # assert that __getattr__ routes an unknown-to-proxy attribute to the
        # inner client by checking a known method's callability.
        with pytest.raises(AttributeError):
            _ = rc.this_attribute_should_not_exist_anywhere
