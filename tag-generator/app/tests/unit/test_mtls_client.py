"""Tests for the mTLS outbound helper."""

from __future__ import annotations

import tempfile

import pytest

from tag_generator.infra.mtls_client import build_ssl_context, mtls_enforced


def test_mtls_enforced_false_by_default(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MTLS_ENFORCE", raising=False)
    assert not mtls_enforced()


def test_mtls_enforced_true_when_env_set(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    assert mtls_enforced()


def test_build_ssl_context_none_when_not_enforced(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.delenv("MTLS_ENFORCE", raising=False)
    assert build_ssl_context() is None


def test_build_ssl_context_fails_closed_when_paths_missing(monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    monkeypatch.delenv("MTLS_CERT_FILE", raising=False)
    monkeypatch.delenv("MTLS_KEY_FILE", raising=False)
    monkeypatch.delenv("MTLS_CA_FILE", raising=False)
    with pytest.raises(RuntimeError, match="MTLS_CERT_FILE"):
        build_ssl_context()


def test_build_ssl_context_fails_closed_when_cert_unreadable(monkeypatch: pytest.MonkeyPatch) -> None:
    """Non-existent cert path should fail, not fall back silently."""
    monkeypatch.setenv("MTLS_ENFORCE", "true")
    monkeypatch.setenv("MTLS_CERT_FILE", "/nonexistent/cert.pem")
    # Provide valid CA file path (a real tempfile) so the error comes from cert_chain.
    with tempfile.NamedTemporaryFile() as ca:
        monkeypatch.setenv("MTLS_KEY_FILE", ca.name)
        monkeypatch.setenv("MTLS_CA_FILE", ca.name)
        with pytest.raises(OSError):
            build_ssl_context()
