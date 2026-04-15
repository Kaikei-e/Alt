"""Tests for the news-creator mTLS outbound helper."""

from __future__ import annotations

import os
import ssl
import tempfile

import pytest

from news_creator.infra.mtls_client import build_ssl_context, mtls_enforced


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
