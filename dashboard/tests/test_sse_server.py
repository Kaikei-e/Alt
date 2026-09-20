"""Tests for sse_server.py's auth-token gate, health exemption, and startup resolution."""

import http.client
import json
import threading
from urllib.parse import urlparse

import pytest

import sse_server


class TestIsAuthorized:
    def test_auth_disabled_when_no_token_configured(self) -> None:
        """With empty token, every request is authorized (disabled mode)."""
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


class TestLoadAuthTokenStartup:
    def test_startup_failure_without_config(self, monkeypatch: pytest.MonkeyPatch) -> None:
        """When neither SSE_AUTH_TOKEN_FILE nor SSE_AUTH=disabled is set, startup exits non-zero."""
        monkeypatch.delenv("SSE_AUTH_TOKEN_FILE", raising=False)
        monkeypatch.delenv("SSE_AUTH_TOKEN", raising=False)
        monkeypatch.delenv("SSE_AUTH", raising=False)

        with pytest.raises(SystemExit) as exc_info:
            sse_server.load_auth_token(fail_fast=True)
        assert exc_info.value.code != 0

    def test_startup_failure_when_token_file_missing(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path
    ) -> None:
        missing_file = str(tmp_path / "nonexistent_token.txt")
        monkeypatch.setenv("SSE_AUTH_TOKEN_FILE", missing_file)
        monkeypatch.delenv("SSE_AUTH", raising=False)

        with pytest.raises(SystemExit) as exc_info:
            sse_server.load_auth_token(fail_fast=True)
        assert exc_info.value.code != 0

    def test_startup_failure_when_token_file_empty(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path
    ) -> None:
        empty_file = tmp_path / "empty_token.txt"
        empty_file.write_text("   \n", encoding="utf-8")
        monkeypatch.setenv("SSE_AUTH_TOKEN_FILE", str(empty_file))
        monkeypatch.delenv("SSE_AUTH", raising=False)

        with pytest.raises(SystemExit) as exc_info:
            sse_server.load_auth_token(fail_fast=True)
        assert exc_info.value.code != 0

    def test_disabled_mode(
        self, monkeypatch: pytest.MonkeyPatch, caplog: pytest.LogCaptureFixture
    ) -> None:
        """Explicit SSE_AUTH=disabled allows open operation and logs sse_auth_disabled."""
        monkeypatch.setenv("SSE_AUTH", "disabled")
        monkeypatch.delenv("SSE_AUTH_TOKEN_FILE", raising=False)
        monkeypatch.delenv("SSE_AUTH_TOKEN", raising=False)

        with caplog.at_level("WARNING"):
            token = sse_server.load_auth_token(fail_fast=True)
        assert token == ""
        assert "sse_auth_disabled" in caplog.text

    def test_token_file_mode(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path, caplog: pytest.LogCaptureFixture
    ) -> None:
        token_file = tmp_path / "valid_token.txt"
        token_file.write_text("valid-secret-token\n", encoding="utf-8")
        monkeypatch.setenv("SSE_AUTH_TOKEN_FILE", str(token_file))
        monkeypatch.delenv("SSE_AUTH", raising=False)

        with caplog.at_level("INFO"):
            token = sse_server.load_auth_token(fail_fast=True)
        assert token == "valid-secret-token"
        assert "sse_auth_enabled" in caplog.text

    def test_unresolved_returns_none_when_fail_fast_false(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.delenv("SSE_AUTH_TOKEN_FILE", raising=False)
        monkeypatch.delenv("SSE_AUTH_TOKEN", raising=False)
        monkeypatch.delenv("SSE_AUTH", raising=False)

        assert sse_server.load_auth_token(fail_fast=False) is None

    def test_missing_file_returns_none_when_fail_fast_false(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path
    ) -> None:
        missing_file = str(tmp_path / "nonexistent_token.txt")
        monkeypatch.setenv("SSE_AUTH_TOKEN_FILE", missing_file)
        monkeypatch.delenv("SSE_AUTH", raising=False)

        assert sse_server.load_auth_token(fail_fast=False) is None

    def test_empty_file_returns_none_when_fail_fast_false(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path
    ) -> None:
        empty_file = tmp_path / "empty_token.txt"
        empty_file.write_text("   \n", encoding="utf-8")
        monkeypatch.setenv("SSE_AUTH_TOKEN_FILE", str(empty_file))
        monkeypatch.delenv("SSE_AUTH", raising=False)

        assert sse_server.load_auth_token(fail_fast=False) is None


class TestSseServerHttpEndpoints:
    """End-to-end HTTP tests exercising /health exemption and /stream auth on loopback socket."""

    @pytest.fixture
    def server(self, monkeypatch: pytest.MonkeyPatch):
        class _TestServer(sse_server.ThreadingHTTPServer):
            daemon_threads = True
            block_on_close = False

        def _run_server(token: str):
            monkeypatch.setattr(sse_server, "AUTH_TOKEN", token)
            httpd = _TestServer(("127.0.0.1", 0), sse_server.SSEHandler)
            addr = httpd.server_address
            host = str(addr[0])
            port = int(addr[1])
            thread = threading.Thread(target=httpd.serve_forever, daemon=True)
            thread.start()
            try:
                yield host, port
            finally:
                httpd.shutdown()
                httpd.server_close()
                thread.join(timeout=2)

        return _run_server

    def test_health_exemption_when_auth_enabled(self, server) -> None:
        """/health must return 200 without token even when SSE auth is enabled."""
        for host, port in server("configured-secret"):
            conn = http.client.HTTPConnection(host, port, timeout=5)
            try:
                conn.request("GET", "/health")
                resp = conn.getresponse()
                assert resp.status == 200
                data = json.loads(resp.read().decode("utf-8"))
                assert data.get("status") == "ok"
                assert data.get("service") == "sse-server"
            finally:
                conn.close()

    def test_stream_401_without_token(self, server) -> None:
        """/stream must return 401 unauthorized when required token is missing."""
        for host, port in server("configured-secret"):
            conn = http.client.HTTPConnection(host, port, timeout=5)
            try:
                conn.request("GET", "/stream")
                resp = conn.getresponse()
                assert resp.status == 401
                data = json.loads(resp.read().decode("utf-8"))
                assert data.get("status") == "error"
                assert data.get("error") == "unauthorized"
            finally:
                conn.close()

    def test_stream_200_with_query_token(self, server) -> None:
        """/stream returns 200 and text/event-stream with matching query parameter token."""
        for host, port in server("configured-secret"):
            conn = http.client.HTTPConnection(host, port, timeout=5)
            try:
                conn.request("GET", "/stream?token=configured-secret")
                resp = conn.getresponse()
                assert resp.status == 200
                assert "text/event-stream" in resp.getheader("Content-Type", "")
                first_line = resp.fp.readline()
                assert b"data:" in first_line
            finally:
                conn.close()

    def test_stream_200_with_bearer_token(self, server) -> None:
        """/stream returns 200 and text/event-stream with matching Bearer header token."""
        for host, port in server("configured-secret"):
            conn = http.client.HTTPConnection(host, port, timeout=5)
            try:
                conn.request(
                    "GET",
                    "/stream",
                    headers={"Authorization": "Bearer configured-secret"},
                )
                resp = conn.getresponse()
                assert resp.status == 200
                assert "text/event-stream" in resp.getheader("Content-Type", "")
                first_line = resp.fp.readline()
                assert b"data:" in first_line
            finally:
                conn.close()

    def test_stream_200_in_disabled_mode(self, server) -> None:
        """In disabled mode (empty token), /stream is accessible without any credentials."""
        for host, port in server(""):
            conn = http.client.HTTPConnection(host, port, timeout=5)
            try:
                conn.request("GET", "/stream")
                resp = conn.getresponse()
                assert resp.status == 200
                assert "text/event-stream" in resp.getheader("Content-Type", "")
                first_line = resp.fp.readline()
                assert b"data:" in first_line
            finally:
                conn.close()

    def test_query_token_redacted_from_logs(
        self, server, caplog: pytest.LogCaptureFixture
    ) -> None:
        """Query parameter tokens must not leak into logs."""
        sensitive_token = "super-secret-token-xyz"
        for host, port in server(sensitive_token):
            with caplog.at_level("INFO"):
                conn = http.client.HTTPConnection(host, port, timeout=5)
                try:
                    conn.request("GET", f"/stream?token={sensitive_token}")
                    resp = conn.getresponse()
                    assert resp.status == 200
                    resp.fp.readline()
                finally:
                    conn.close()

            assert sensitive_token not in caplog.text
            assert "/stream" in caplog.text

    def test_unauthorized_query_token_redacted_from_logs(
        self, server, caplog: pytest.LogCaptureFixture
    ) -> None:
        """Rejected query parameter tokens must also not leak into logs."""
        configured_token = "correct-secret-123"
        rejected_token = "attacker-attempted-token-999"
        for host, port in server(configured_token):
            with caplog.at_level("INFO"):
                conn = http.client.HTTPConnection(host, port, timeout=5)
                try:
                    conn.request("GET", f"/stream?token={rejected_token}")
                    resp = conn.getresponse()
                    assert resp.status == 401
                    resp.read()
                finally:
                    conn.close()

            assert rejected_token not in caplog.text
            assert configured_token not in caplog.text


class TestSystemMonitorTabTokenWiring:
    def test_resolves_token_from_file(
        self, monkeypatch: pytest.MonkeyPatch, tmp_path
    ) -> None:
        token_file = tmp_path / "tab_token.txt"
        token_file.write_text("tab-secret-value\n", encoding="utf-8")
        monkeypatch.setenv("SSE_AUTH_TOKEN_FILE", str(token_file))
        monkeypatch.delenv("SSE_AUTH", raising=False)

        from tabs.system_monitor_tab import _resolve_sse_token

        assert _resolve_sse_token() == "tab-secret-value"

    def test_disabled_mode_returns_empty_token(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("SSE_AUTH", "disabled")
        from tabs.system_monitor_tab import _resolve_sse_token

        assert _resolve_sse_token() == ""

    def test_unresolved_token_returns_none(
        self, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.delenv("SSE_AUTH_TOKEN_FILE", raising=False)
        monkeypatch.delenv("SSE_AUTH_TOKEN", raising=False)
        monkeypatch.delenv("SSE_AUTH", raising=False)

        from tabs.system_monitor_tab import _resolve_sse_token

        assert _resolve_sse_token() is None
