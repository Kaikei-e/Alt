"""Composition-root wiring: enrollment owns the leaf before any outbound
client exists, and a failed startup never strands the renewal thread."""

from __future__ import annotations

import secrets
from pathlib import Path

import pytest
import structlog

from recap_evaluator import main
from recap_evaluator.config import Settings
from recap_evaluator.infra import bearer_auth, mtls_client


class _FakeHandle:
    def __init__(self, calls: list[str]) -> None:
        self._calls = calls

    def stop(self) -> None:
        self._calls.append("pki_stop")


@pytest.fixture
def lifespan_env(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> list[str]:
    token_file = tmp_path / "evaluator_api_token"
    token_file.write_text(secrets.token_urlsafe(32))
    monkeypatch.setenv("EVALUATOR_API_TOKEN_FILE", str(token_file))
    monkeypatch.setenv("MTLS_ENFORCE", "false")
    monkeypatch.setattr(main, "configure_logging", lambda **_: None)
    calls: list[str] = []

    def fake_start(service_name: str) -> _FakeHandle:
        calls.append(f"pki:{service_name}")
        return _FakeHandle(calls)

    async def fake_create_pool(**_: object) -> None:
        calls.append("db")
        raise RuntimeError("recap-db unavailable")

    monkeypatch.setattr(mtls_client, "start_pki_enrollment", fake_start)
    monkeypatch.setattr(main.asyncpg, "create_pool", fake_create_pool)
    return calls


def _settings(ollama_url: str) -> Settings:
    return Settings(
        recap_db_dsn="postgres://test:test@localhost:5432/test",
        ollama_url=ollama_url,
        recap_worker_url="http://localhost:8081",
        enable_scheduler=False,
    )


async def test_lifespan_enrolls_first_and_stops_enrollment_on_startup_failure(
    lifespan_env: list[str],
) -> None:
    app = main.create_app(settings=_settings("http://localhost:11434"))

    with pytest.raises(RuntimeError, match="recap-db unavailable"):
        async with app.router.lifespan_context(app):
            pass

    assert lifespan_env == ["pki:recap-evaluator", "db", "pki_stop"]


async def test_lifespan_refuses_https_news_creator_without_client_cert(
    lifespan_env: list[str],
) -> None:
    app = main.create_app(settings=_settings("https://news-creator:9443"))

    with pytest.raises(RuntimeError, match="OLLAMA_URL"):
        async with app.router.lifespan_context(app):
            pass

    assert lifespan_env == []


def _auth_events(logs: list[dict]) -> list[tuple[str, str]]:
    return [
        (entry["event"], entry["log_level"])
        for entry in logs
        if entry["event"] in {"evaluator_auth_enabled", "evaluator_auth_disabled"}
    ]


async def test_lifespan_logs_evaluator_auth_enabled_once(lifespan_env: list[str]) -> None:
    app = main.create_app(settings=_settings("http://localhost:11434"))

    with structlog.testing.capture_logs() as logs, pytest.raises(RuntimeError):
        async with app.router.lifespan_context(app):
            pass

    assert _auth_events(logs) == [("evaluator_auth_enabled", "info")]
    assert app.state.api_token


async def test_lifespan_warns_when_evaluator_auth_disabled(
    lifespan_env: list[str], monkeypatch: pytest.MonkeyPatch
) -> None:
    monkeypatch.setattr(bearer_auth, "EVALUATOR_AUTH_DISABLED", True)
    app = main.create_app(settings=_settings("http://localhost:11434"))

    with structlog.testing.capture_logs() as logs, pytest.raises(RuntimeError):
        async with app.router.lifespan_context(app):
            pass

    assert _auth_events(logs) == [("evaluator_auth_disabled", "warning")]
    assert app.state.api_token == ""
