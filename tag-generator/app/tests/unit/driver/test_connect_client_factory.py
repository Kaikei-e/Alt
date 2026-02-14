"""Tests for connect_client_factory module."""

from __future__ import annotations

import os
from unittest.mock import patch

from tag_generator.driver.connect_client_factory import create_backend_client


class TestCreateBackendClient:
    """Tests for the create_backend_client factory function."""

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

    def test_reads_token_from_file(self, tmp_path) -> None:
        """Reads service token from SERVICE_TOKEN_FILE."""
        token_file = tmp_path / "token"
        token_file.write_text("file-secret-token\n")

        env = {
            "BACKEND_API_URL": "http://alt-backend:9000",
            "SERVICE_TOKEN_FILE": str(token_file),
        }
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        _, headers = result
        assert headers["X-Service-Token"] == "file-secret-token"

    def test_falls_back_to_env_token(self) -> None:
        """Falls back to SERVICE_TOKEN env var when file not available."""
        env = {
            "BACKEND_API_URL": "http://alt-backend:9000",
            "SERVICE_TOKEN": "env-secret-token",
        }
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        _, headers = result
        assert headers["X-Service-Token"] == "env-secret-token"

    def test_file_token_takes_precedence(self, tmp_path) -> None:
        """SERVICE_TOKEN_FILE takes precedence over SERVICE_TOKEN."""
        token_file = tmp_path / "token"
        token_file.write_text("file-token\n")

        env = {
            "BACKEND_API_URL": "http://alt-backend:9000",
            "SERVICE_TOKEN_FILE": str(token_file),
            "SERVICE_TOKEN": "env-token",
        }
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        _, headers = result
        assert headers["X-Service-Token"] == "file-token"

    def test_no_token_header_when_no_token(self) -> None:
        """No X-Service-Token header when no token is configured."""
        env = {"BACKEND_API_URL": "http://alt-backend:9000"}
        with patch.dict(os.environ, env, clear=True):
            result = create_backend_client()

        assert result is not None
        _, headers = result
        assert "X-Service-Token" not in headers
