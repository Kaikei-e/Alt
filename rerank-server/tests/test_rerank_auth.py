"""Attack-path and security tests for rerank-server authentication (B03/B-SUPP01).

Tests:
- Fail-fast on startup when INFERENCE_SERVICE_TOKEN_FILE is missing, empty, or too short.
- Rejection of unauthenticated, spoofed, or invalid bearer tokens with 401.
- Constant-time comparison (hmac.compare_digest) to prevent timing attacks.
- Acceptance of valid bearer tokens on /v1/rerank.
- Unauthenticated access preserved for /health and / endpoints.
- Lifespan model initialization preserved when auth succeeds.
"""

from __future__ import annotations

import hmac
import secrets
from collections.abc import Iterator
from http import HTTPStatus
from pathlib import Path
from unittest.mock import MagicMock, patch

import anyio
import pytest
from fastapi.testclient import TestClient

import rerank_server
from rerank_server import (
    app,
    lifespan,
    load_inference_auth_config,
)

VALID_SECRET = secrets.token_urlsafe(32)


@pytest.fixture
def valid_token_file(tmp_path: Path) -> Path:
    token_file = tmp_path / "inference_token.txt"
    token_file.write_text(VALID_SECRET)
    return token_file


@pytest.fixture
def fake_model() -> MagicMock:
    model = MagicMock()
    model.predict.side_effect = lambda pairs, **_: [0.9] * len(pairs)
    return model


class TestInferenceTokenFailFast:
    """Startup fail-fast validation when auth is enabled."""

    def test_missing_token_file_raises_runtime_error(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        missing_file = tmp_path / "does_not_exist.token"
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(missing_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="does not exist"):
            load_inference_auth_config()

    def test_auth_disabled_skips_token_file_check(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "disabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(tmp_path / "missing.token"))

        config = load_inference_auth_config()

        assert config.enabled is False
        assert config.token is None

    def test_unset_token_file_uses_default_path(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        default_path = tmp_path / "default_inference_token"
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)
        monkeypatch.setattr(rerank_server, "DEFAULT_INFERENCE_TOKEN_FILE", default_path)

        with pytest.raises(RuntimeError, match="default_inference_token"):
            load_inference_auth_config()

    def test_empty_token_file_raises_runtime_error(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        empty_file = tmp_path / "empty.token"
        empty_file.write_text("   \n  ")
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(empty_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="empty"):
            load_inference_auth_config()

    def test_newline_only_token_file_raises_is_empty(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        token_file = tmp_path / "blank.token"
        token_file.write_text("\r\n")
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(token_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="is empty"):
            load_inference_auth_config()

    def test_short_token_raises_runtime_error(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        short_file = tmp_path / "short.token"
        short_file.write_text("short-tok")
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(short_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="too short"):
            load_inference_auth_config()

    def test_lifespan_fails_fast_on_missing_token(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        missing_file = tmp_path / "nonexistent.token"
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(missing_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        async def _run_lifespan() -> None:
            async with lifespan(app):
                pass

        with pytest.raises(RuntimeError, match="does not exist"):
            anyio.run(_run_lifespan)

    def test_lifespan_succeeds_when_token_valid(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        token_file = tmp_path / "valid.token"
        token_file.write_text(VALID_SECRET)
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(token_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        async def _run_lifespan() -> None:
            async with lifespan(app):
                assert app.state.auth_enabled is True
                assert app.state.auth_token == VALID_SECRET

        anyio.run(_run_lifespan)

    def test_token_with_control_chars_rejected(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Tokens containing control characters (tabs, etc.) are rejected."""
        bad_file = tmp_path / "control.token"
        bad_file.write_text("valid_prefix\ttab_in_middle_rest")
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(bad_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="RFC 6750"):
            load_inference_auth_config()

    def test_token_with_crlf_rejected(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Tokens with embedded CR/LF are rejected (even after strip)."""
        bad_file = tmp_path / "crlf.token"
        bad_file.write_text("partA\r\npartB_long_enough_token")
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(bad_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="RFC 6750"):
            load_inference_auth_config()

    def test_token_with_whitespace_rejected(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Tokens with internal whitespace are rejected by RFC 6750 alphabet."""
        bad_file = tmp_path / "space.token"
        bad_file.write_text("valid_token_with space_inside_long")
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(bad_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="RFC 6750"):
            load_inference_auth_config()

    def test_unicode_decode_error_no_content_leak(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """UnicodeDecodeError must not leak file content in the error message."""
        bad_file = tmp_path / "binary.token"
        bad_file.write_bytes(b"\xff\xfe\x00\x01" * 10)
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(bad_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with pytest.raises(RuntimeError, match="invalid encoding") as exc_info:
            load_inference_auth_config()
        # Must NOT contain binary content or __cause__
        assert exc_info.value.__cause__ is None

    def test_os_error_reading_token_file_raises_runtime_error(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        token_file = tmp_path / "unreadable.token"
        token_file.write_text(VALID_SECRET)
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(token_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        with (
            patch.object(Path, "open", side_effect=OSError("permission denied")),
            pytest.raises(RuntimeError, match="Failed to read INFERENCE_SERVICE_TOKEN_FILE"),
        ):
            load_inference_auth_config()

    def test_valid_rfc6750_token_accepted(
        self, tmp_path: Path, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        """Valid RFC 6750 b64token (alphanumeric + -._~+/=) is accepted."""
        good_file = tmp_path / "good.token"
        good_token = "ABCDEFghijklmnop+/.-_~="  # noqa: S105 - test fixture value, not a credential
        good_file.write_text(good_token)
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(good_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)

        config = load_inference_auth_config()
        assert config.enabled is True
        assert config.token == good_token


class TestInferenceAuthEndpoints:
    """Verification of endpoint security with active token enforcement."""

    @pytest.fixture(autouse=True)
    def setup_auth_env(
        self, valid_token_file: Path, monkeypatch: pytest.MonkeyPatch
    ) -> Iterator[None]:
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "enabled")
        monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(valid_token_file))
        monkeypatch.delenv("INFERENCE_SERVICE_TOKEN", raising=False)
        app.state.auth_config = None
        yield
        app.state.auth_config = None

    def test_rerank_rejects_missing_authorization_header(self, fake_model: MagicMock) -> None:
        app.state.model = fake_model
        client = TestClient(app)

        resp = client.post(
            "/v1/rerank",
            json={"query": "test query", "candidates": ["candidate A", "candidate B"]},
        )
        assert resp.status_code == HTTPStatus.UNAUTHORIZED
        assert "Bearer" in resp.headers.get("WWW-Authenticate", "")

    def test_rerank_rejects_invalid_token(self, fake_model: MagicMock) -> None:
        app.state.model = fake_model
        client = TestClient(app)

        wrong_token = secrets.token_urlsafe(32)
        resp = client.post(
            "/v1/rerank",
            headers={"Authorization": f"Bearer {wrong_token}"},
            json={"query": "test query", "candidates": ["candidate A"]},
        )
        assert resp.status_code == HTTPStatus.UNAUTHORIZED
        assert "Bearer" in resp.headers.get("WWW-Authenticate", "")

    def test_rerank_rejects_malformed_authorization_header(self, fake_model: MagicMock) -> None:
        app.state.model = fake_model
        client = TestClient(app)

        resp = client.post(
            "/v1/rerank",
            headers={"Authorization": "Basic dXNlcjpwYXNz"},
            json={"query": "test query", "candidates": ["candidate A"]},
        )
        assert resp.status_code == HTTPStatus.UNAUTHORIZED

    def test_rerank_accepts_valid_token(self, fake_model: MagicMock) -> None:
        app.state.model = fake_model
        client = TestClient(app)
        candidates = ["candidate A", "candidate B"]

        resp = client.post(
            "/v1/rerank",
            headers={"Authorization": f"Bearer {VALID_SECRET}"},
            json={"query": "test query", "candidates": candidates},
        )
        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert len(data["results"]) == len(candidates)

    def test_rerank_allows_unauthenticated_when_auth_disabled(
        self, fake_model: MagicMock, monkeypatch: pytest.MonkeyPatch
    ) -> None:
        monkeypatch.setenv("INFERENCE_SERVICE_AUTH", "disabled")
        app.state.auth_config = None
        app.state.model = fake_model
        client = TestClient(app)

        resp = client.post(
            "/v1/rerank",
            json={"query": "test query", "candidates": ["candidate A"]},
        )
        assert resp.status_code == HTTPStatus.OK

    def test_health_endpoint_accessible_without_token(self, fake_model: MagicMock) -> None:
        app.state.model = fake_model
        client = TestClient(app)

        resp = client.get("/health")
        assert resp.status_code == HTTPStatus.OK
        assert resp.json()["status"] == "ok"

    def test_root_endpoint_accessible_without_token(self) -> None:
        client = TestClient(app)

        resp = client.get("/")
        assert resp.status_code == HTTPStatus.OK
        assert resp.json()["service"] == "rerank-server"

    def test_constant_time_comparison_is_used(self, fake_model: MagicMock) -> None:
        """Assert hmac.compare_digest is called to prevent timing attacks."""
        app.state.model = fake_model
        client = TestClient(app)

        with patch("hmac.compare_digest", wraps=hmac.compare_digest) as mock_compare:
            resp = client.post(
                "/v1/rerank",
                headers={"Authorization": f"Bearer {VALID_SECRET}"},
                json={"query": "test", "candidates": ["cand"]},
            )
            assert resp.status_code == HTTPStatus.OK
            assert mock_compare.called
