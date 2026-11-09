"""Tests for generate handler - HTTP 429 queue full behavior and Gemma prompt boundary."""

import logging
from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, Mock

from news_creator.domain.errors import QueueFullError
from news_creator.domain.models import LLMGenerateResponse
from news_creator.gateway.payload_builder import prepare_raw_prompt
from news_creator.handler.generate_handler import create_generate_router
from news_creator.port.llm_provider_port import LLMProviderPort


def _make_client(mock_provider):
    app = FastAPI()
    app.include_router(create_generate_router(mock_provider))
    return TestClient(app)


def _make_mock_provider(return_value=None, side_effect=None):
    mock = Mock(spec=LLMProviderPort)
    if side_effect:
        mock.generate = AsyncMock(side_effect=side_effect)
    else:
        mock.generate = AsyncMock(
            return_value=return_value
            or LLMGenerateResponse(
                response="hello",
                model="test-model",
                done=True,
                done_reason="stop",
            )
        )
    return mock


def test_generate_returns_429_when_queue_full():
    """Queue saturation is expected backpressure; must surface as 429, not 500."""
    mock_provider = _make_mock_provider(
        side_effect=QueueFullError("Queue depth 10 >= max 10")
    )
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 429
    data = response.json()
    assert "queue full" in data["error"]
    assert response.headers.get("Retry-After") == "30"


def test_generate_returns_200_on_success():
    mock_provider = _make_mock_provider()
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 200
    assert response.json()["response"] == "hello"


def test_generate_prompt_over_cap_keeps_its_validation_detail():
    """The prompt-length 400 is static validation feedback, not internal detail.

    The E2E contract asserts /prompt too long/i on this response; collapsing it
    into the generic "Invalid request" broke the one client-actionable hint
    while protecting nothing (the message carries only the client's own prompt
    length and the cap).
    """
    mock_provider = _make_mock_provider()
    client = _make_client(mock_provider)

    response = client.post(
        "/api/generate", json={"prompt": "a" * 240_001, "stream": False}
    )

    assert response.status_code == 400
    assert "prompt too long" in response.json()["detail"].lower()
    mock_provider.generate.assert_not_awaited()


def test_generate_value_error_does_not_leak_exception_text():
    """Regression: ValueError must not leak to the client body."""
    secret = (
        "invalid generate payload at /app/news_creator/gateway/ollama_gateway.py:88"
    )
    mock_provider = _make_mock_provider(side_effect=ValueError(secret))
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 400
    assert response.json()["detail"] == "Invalid request"
    assert secret not in response.text


def test_generate_runtime_error_does_not_leak_exception_text():
    """Regression: RuntimeError must not leak to the client body."""
    secret = "Ollama connection refused at http://ollama.internal:11434"
    mock_provider = _make_mock_provider(side_effect=RuntimeError(secret))
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 502
    assert response.json()["detail"] == "Upstream service error"
    assert secret not in response.text


def test_generate_timeout_does_not_leak_exception_text():
    """Regression: timeout errors must not leak host details to the client."""
    import asyncio

    secret = "timed out connecting to ollama.internal:11434"
    mock_provider = _make_mock_provider(side_effect=asyncio.TimeoutError(secret))
    client = _make_client(mock_provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == 502
    assert response.json()["detail"] == "Upstream service error"
    assert secret not in response.text


def test_generate_wraps_untemplated_prompt_and_logs_wrapped_true(caplog):
    mock_provider = _make_mock_provider()
    client = _make_client(mock_provider)

    with caplog.at_level(logging.INFO):
        response = client.post("/api/generate", json={"prompt": "hello world"})

    assert response.status_code == 200
    mock_provider.generate.assert_awaited_once()
    sent_prompt = mock_provider.generate.call_args.kwargs["prompt"]
    assert sent_prompt == "<|turn>user\nhello world<turn|>\n<|turn>model\n"
    assert "generate_prompt_wrapped=true" in caplog.text


def test_generate_passes_already_templated_prompt_unchanged_and_logs_wrapped_false(
    caplog,
):
    mock_provider = _make_mock_provider()
    client = _make_client(mock_provider)
    templated_prompt = "<|turn>user\nhello world<turn|>\n<|turn>model\n"

    with caplog.at_level(logging.INFO):
        response = client.post("/api/generate", json={"prompt": templated_prompt})

    assert response.status_code == 200
    mock_provider.generate.assert_awaited_once()
    sent_prompt = mock_provider.generate.call_args.kwargs["prompt"]
    assert sent_prompt == templated_prompt
    assert "generate_prompt_wrapped=false" in caplog.text


def test_generate_rejects_malformed_templated_prompt_missing_model_turn_with_400(
    caplog,
):
    async def _guarding_generate(**kwargs):
        prepare_raw_prompt(kwargs["prompt"])
        return LLMGenerateResponse(
            response="ok",
            model="test-model",
            done=True,
            done_reason="stop",
        )

    mock_provider = _make_mock_provider(side_effect=_guarding_generate)
    client = _make_client(mock_provider)
    malformed_prompt = "<|turn>user\nhello world<turn|>"

    with caplog.at_level(logging.INFO):
        response = client.post("/api/generate", json={"prompt": malformed_prompt})

    assert response.status_code == 400
    assert response.json()["detail"] == "Invalid request"
    mock_provider.generate.assert_awaited_once()
    sent_prompt = mock_provider.generate.call_args.kwargs["prompt"]
    assert sent_prompt == malformed_prompt
    assert "generate_prompt_wrapped=false" in caplog.text
