"""Tests for connect_client_factory module."""

from __future__ import annotations

import os
from unittest.mock import patch

from tag_generator.driver.connect_client_factory import create_backend_client


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
