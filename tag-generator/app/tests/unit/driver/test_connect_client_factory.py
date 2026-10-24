"""Tests for connect_client_factory module."""

from __future__ import annotations

import os
import subprocess
from pathlib import Path
from unittest.mock import patch

import pytest

from tag_generator.driver.connect_article_fetcher import ConnectArticleFetcher
from tag_generator.driver.connect_client_factory import (
    _ReloadingBackendClient,
    create_article_fetcher_and_inserter,
    create_backend_client,
)
from tag_generator.driver.connect_tag_inserter import ConnectTagInserter


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
        from tag_generator.gen.proto.services.datahub.v1.datahub_connect import (
            DataHubServiceClientSync,
        )

        assert isinstance(client, DataHubServiceClientSync)
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

    @pytest.mark.parametrize(
        "env",
        [
            {"MTLS_ENFORCE": "true"},
            {"MTLS_ENFORCE": "true", "BACKEND_API_MTLS_URL": ""},
        ],
    )
    def test_raises_when_mtls_enforce_true_and_mtls_url_unset_or_empty(self, env: dict[str, str]) -> None:
        """Fails closed with RuntimeError when MTLS_ENFORCE=true and BACKEND_API_MTLS_URL is unset or empty."""
        with patch.dict(os.environ, env, clear=True):
            with pytest.raises(RuntimeError, match="BACKEND_API_MTLS_URL is unset"):
                create_backend_client()

    @pytest.mark.parametrize(
        "missing_var",
        ["MTLS_CERT_FILE", "MTLS_KEY_FILE", "MTLS_CA_FILE"],
    )
    def test_raises_when_mtls_enforce_true_and_cert_files_missing(self, missing_var: str) -> None:
        """Fails closed with RuntimeError when any of MTLS_CERT_FILE/KEY_FILE/CA_FILE is unset."""
        env = {
            "MTLS_ENFORCE": "true",
            "BACKEND_API_MTLS_URL": "https://alt-backend:9443",
            "MTLS_CERT_FILE": "/certs/cert.pem",
            "MTLS_KEY_FILE": "/certs/key.pem",
            "MTLS_CA_FILE": "/certs/ca.pem",
        }
        del env[missing_var]
        with patch.dict(os.environ, env, clear=True):
            with pytest.raises(RuntimeError, match="MTLS_CERT_FILE/KEY_FILE/CA_FILE not fully set"):
                create_backend_client()

    def test_returns_reloading_client_when_mtls_enforce_and_all_env_set(self, tmp_path: Path) -> None:
        """Returns (_ReloadingBackendClient, headers) when MTLS_ENFORCE=true and all env vars are configured."""
        cert, key, ca = _write_test_pki(tmp_path, "tag-generator")
        env = {
            "MTLS_ENFORCE": "true",
            "BACKEND_API_MTLS_URL": "https://alt-backend:9443",
            "MTLS_CERT_FILE": str(cert),
            "MTLS_KEY_FILE": str(key),
            "MTLS_CA_FILE": str(ca),
        }
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        client, headers = result
        assert isinstance(client, _ReloadingBackendClient)
        assert headers == {}


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
        # DataHubServiceClientSync exposes `base_url` as a positional constructor
        # arg; the underlying ConnectClientSync stores it on
        # `_base_url`. Rather than depending on internal field names, we just
        # assert that __getattr__ routes an unknown-to-proxy attribute to the
        # inner client by checking a known method's callability.
        with pytest.raises(AttributeError):
            _ = rc.this_attribute_should_not_exist_anywhere

        # Positive path: an attribute that only exists on the wrapped
        # DataHubServiceClientSync (never defined on the proxy itself)
        # must resolve to a bound method of that same inner
        # instance -- proving __getattr__ actually forwards rather than
        # merely failing closed on unknown names.
        bound_close = rc.close  # noqa: SLF001 — real RPC-client method, forwarded via __getattr__
        assert bound_close.__self__ is rc._inner  # noqa: SLF001


class TestCreateArticleFetcherAndInserter:
    """Tests for create_article_fetcher_and_inserter composition-root helper."""

    def test_raises_runtime_error_when_client_is_none(self) -> None:
        """Raises RuntimeError when create_backend_client returns None."""
        with patch.dict(os.environ, {}, clear=True):
            with pytest.raises(RuntimeError, match="Backend API client could not be created"):
                create_article_fetcher_and_inserter()

    def test_returns_fetcher_and_inserter_when_client_created(self) -> None:
        """Returns (ConnectArticleFetcher, ConnectTagInserter) tuple when client is available."""
        env = {"BACKEND_API_URL": "http://alt-backend:9000"}
        with patch.dict(os.environ, env, clear=True):
            fetcher, inserter = create_article_fetcher_and_inserter()

        assert isinstance(fetcher, ConnectArticleFetcher)
        assert isinstance(inserter, ConnectTagInserter)
