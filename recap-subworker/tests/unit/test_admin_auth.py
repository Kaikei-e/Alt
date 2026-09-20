"""Unit tests for the /admin/* and /v1/runs bearer-token guard."""

from __future__ import annotations

from pathlib import Path

import pytest
from fastapi import Depends, FastAPI, HTTPException
from fastapi.testclient import TestClient

from recap_subworker.app.infra.admin_auth import (
    AdminAuthConfig,
    get_admin_auth_config,
    load_admin_auth_config,
    require_admin_token,
)


def _protected_app(config: AdminAuthConfig) -> FastAPI:
    app = FastAPI()

    @app.get("/protected", dependencies=[Depends(require_admin_token)])
    async def protected() -> dict[str, bool]:
        return {"ok": True}

    app.dependency_overrides[get_admin_auth_config] = lambda: config
    return app


def test_missing_authorization_header_is_unauthorized():
    app = _protected_app(AdminAuthConfig(token="expected-token-0123456789"))
    with TestClient(app) as client:
        response = client.get("/protected")
    assert response.status_code == 401
    assert response.headers["www-authenticate"] == "Bearer"


def test_wrong_token_is_unauthorized():
    app = _protected_app(AdminAuthConfig(token="expected-token-0123456789"))
    with TestClient(app) as client:
        response = client.get(
            "/protected", headers={"Authorization": "Bearer wrong-token-0123456789"}
        )
    assert response.status_code == 401
    assert response.headers["www-authenticate"] == "Bearer"


def test_missing_and_wrong_token_responses_are_indistinguishable():
    """Body/detail must be stable across failure modes so a caller can't
    use the response to tell "no token sent" apart from "wrong token
    sent"."""
    app = _protected_app(AdminAuthConfig(token="expected-token-0123456789"))
    with TestClient(app) as client:
        missing = client.get("/protected")
        wrong = client.get("/protected", headers={"Authorization": "Bearer wrong-token-0123456789"})
    assert missing.status_code == wrong.status_code == 401
    assert missing.json() == wrong.json()


@pytest.mark.asyncio
async def test_non_ascii_authorization_header_is_unauthorized_not_500():
    """Starlette decodes the raw Authorization header as latin-1, so a
    non-ASCII byte sequence reaches `require_admin_token` as a non-ASCII
    `str`. `hmac.compare_digest` raises `TypeError` on non-ASCII `str`
    arguments, so the comparison must happen on encoded bytes instead."""
    config = AdminAuthConfig(token="expected-token-0123456789")
    with pytest.raises(HTTPException) as exc_info:
        await require_admin_token(
            authorization="Bearer éééwrong-token",
            auth_config=config,
        )
    assert exc_info.value.status_code == 401


def test_correct_token_passes_through():
    app = _protected_app(AdminAuthConfig(token="expected-token-0123456789"))
    with TestClient(app) as client:
        response = client.get(
            "/protected",
            headers={"Authorization": "Bearer expected-token-0123456789"},
        )
    assert response.status_code == 200
    assert response.json() == {"ok": True}


def test_disabled_auth_passes_through_without_header():
    app = _protected_app(AdminAuthConfig(token=None))
    with TestClient(app) as client:
        response = client.get("/protected")
    assert response.status_code == 200


class TestLoadAdminAuthConfig:
    def test_disabled_mode_returns_none_token(self, monkeypatch):
        monkeypatch.setenv("ADMIN_AUTH", "disabled")
        monkeypatch.delenv("ADMIN_TOKEN_FILE", raising=False)
        config = load_admin_auth_config()
        assert config.token is None

    def test_unset_auth_and_missing_token_file_raises(self, monkeypatch):
        monkeypatch.delenv("ADMIN_AUTH", raising=False)
        monkeypatch.delenv("ADMIN_TOKEN_FILE", raising=False)
        with pytest.raises(RuntimeError, match="ADMIN_TOKEN_FILE"):
            load_admin_auth_config()

    def test_valid_token_file_resolves_token(self, tmp_path: Path, monkeypatch):
        token_path = tmp_path / "admin_token"
        token_path.write_text("test-recap-subworker-token-42\n")
        monkeypatch.delenv("ADMIN_AUTH", raising=False)
        monkeypatch.setenv("ADMIN_TOKEN_FILE", str(token_path))
        config = load_admin_auth_config()
        assert config.token == "test-recap-subworker-token-42"

    def test_short_token_is_rejected(self, tmp_path: Path, monkeypatch):
        token_path = tmp_path / "short_token"
        token_path.write_text("too-short")
        monkeypatch.delenv("ADMIN_AUTH", raising=False)
        monkeypatch.setenv("ADMIN_TOKEN_FILE", str(token_path))
        with pytest.raises(RuntimeError, match="at least 24 characters"):
            load_admin_auth_config()

    def test_missing_token_file_is_rejected(self, tmp_path: Path, monkeypatch):
        monkeypatch.delenv("ADMIN_AUTH", raising=False)
        monkeypatch.setenv("ADMIN_TOKEN_FILE", str(tmp_path / "does-not-exist"))
        with pytest.raises(RuntimeError, match="failed to read token file"):
            load_admin_auth_config()
