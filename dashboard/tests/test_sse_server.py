"""Tests for sse_server.py's auth-token gate (pure logic, no live socket)."""

from urllib.parse import urlparse

import sse_server


class TestIsAuthorized:
    def test_auth_disabled_when_no_token_configured(self) -> None:
        """With SSE_AUTH_TOKEN unset, every request is authorized (loopback/dev mode)."""
        assert (
            sse_server.is_authorized(
                configured_token="", token_param=None, auth_header=None
            )
            is True
        )

    def test_missing_credentials_rejected_when_token_configured(self) -> None:
        assert (
            sse_server.is_authorized(
                configured_token="secret", token_param=None, auth_header=None
            )
            is False
        )

    def test_wrong_query_token_rejected(self) -> None:
        assert (
            sse_server.is_authorized(
                configured_token="secret", token_param="wrong", auth_header=None
            )
            is False
        )

    def test_correct_query_token_accepted(self) -> None:
        assert (
            sse_server.is_authorized(
                configured_token="secret", token_param="secret", auth_header=None
            )
            is True
        )

    def test_correct_bearer_header_accepted(self) -> None:
        assert (
            sse_server.is_authorized(
                configured_token="secret", token_param=None, auth_header="Bearer secret"
            )
            is True
        )

    def test_wrong_bearer_header_rejected(self) -> None:
        assert (
            sse_server.is_authorized(
                configured_token="secret", token_param=None, auth_header="Bearer nope"
            )
            is False
        )

    def test_malformed_auth_header_rejected(self) -> None:
        assert (
            sse_server.is_authorized(
                configured_token="secret", token_param=None, auth_header="secret"
            )
            is False
        )


class TestExtractTokenParam:
    def test_extracts_token_query_param(self) -> None:
        query = urlparse("/stream?token=abc123").query
        assert sse_server.extract_token_param(query) == "abc123"

    def test_no_token_param_returns_none(self) -> None:
        query = urlparse("/stream").query
        assert sse_server.extract_token_param(query) is None
