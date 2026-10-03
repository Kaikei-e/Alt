"""INFERENCE_SERVICE_TOKEN_FILE resolution for the generation-proxy bearer.

Every LLM request goes through the token-protected generation-proxy, so an
unset token file is a startup error. INFERENCE_AUTH=disabled is the only way
to run without the Authorization header, and it is logged once per process.
"""

import logging

import pytest

from news_creator.config import inference_token
from news_creator.config.llm_config import LLMConfig


@pytest.fixture(autouse=True)
def _clean_inference_env(monkeypatch):
    monkeypatch.setattr(inference_token, "_disabled_logged", False)
    monkeypatch.delenv("INFERENCE_AUTH", raising=False)
    monkeypatch.delenv("INFERENCE_SERVICE_TOKEN_FILE", raising=False)


def _write_token(tmp_path, content: str):
    path = tmp_path / "inference.token"
    path.write_text(content, encoding="utf-8")
    return path


def test_unset_raises_error_naming_both_variables():
    with pytest.raises(ValueError) as exc_info:
        inference_token.resolve_inference_token()

    assert "INFERENCE_SERVICE_TOKEN_FILE" in str(exc_info.value)
    assert "INFERENCE_AUTH" in str(exc_info.value)


def test_blank_path_raises(monkeypatch):
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", "   ")

    with pytest.raises(ValueError, match="INFERENCE_SERVICE_TOKEN_FILE"):
        inference_token.resolve_inference_token()


def test_missing_file_fails_fast(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE", str(tmp_path / "does-not-exist.token")
    )

    with pytest.raises(ValueError, match="not found"):
        inference_token.resolve_inference_token()


def test_directory_path_fails_fast(tmp_path, monkeypatch):
    monkeypatch.setenv("INFERENCE_SERVICE_TOKEN_FILE", str(tmp_path))

    with pytest.raises(ValueError, match="not found"):
        inference_token.resolve_inference_token()


def test_empty_file_fails_fast(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE", str(_write_token(tmp_path, "  \n"))
    )

    with pytest.raises(ValueError, match="empty"):
        inference_token.resolve_inference_token()


def test_invalid_token_format_fails_fast(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE",
        str(_write_token(tmp_path, "token with spaces!@#")),
    )

    with pytest.raises(ValueError, match="Invalid token format"):
        inference_token.resolve_inference_token()


def test_present_file_returns_trimmed_token(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE",
        str(_write_token(tmp_path, "  proxy-token-123=\n")),
    )

    assert inference_token.resolve_inference_token() == "proxy-token-123="


def test_explicit_disabled_returns_none_and_logs_once(monkeypatch, caplog):
    monkeypatch.setenv("INFERENCE_AUTH", "disabled")

    with caplog.at_level(logging.WARNING, logger=inference_token.__name__):
        assert inference_token.resolve_inference_token() is None
        assert inference_token.resolve_inference_token() is None

    disabled_logs = [
        r for r in caplog.records if "inference_auth_disabled" in r.getMessage()
    ]
    assert len(disabled_logs) == 1


def test_explicit_disabled_is_case_insensitive(monkeypatch):
    monkeypatch.setenv("INFERENCE_AUTH", " Disabled ")

    assert inference_token.resolve_inference_token() is None


def test_explicit_disabled_wins_over_token_file(tmp_path, monkeypatch):
    monkeypatch.setenv("INFERENCE_AUTH", "disabled")
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE", str(_write_token(tmp_path, "proxy-token"))
    )

    assert inference_token.resolve_inference_token() is None


def test_llm_config_from_env_fails_fast_when_unconfigured():
    with pytest.raises(ValueError, match="INFERENCE_SERVICE_TOKEN_FILE"):
        LLMConfig.from_env()


def test_llm_config_from_env_carries_resolved_token(tmp_path, monkeypatch):
    monkeypatch.setenv(
        "INFERENCE_SERVICE_TOKEN_FILE", str(_write_token(tmp_path, "proxy-token"))
    )

    assert LLMConfig.from_env().inference_service_token == "proxy-token"
