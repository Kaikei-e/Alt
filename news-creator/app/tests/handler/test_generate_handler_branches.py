"""Branch coverage tests for generate_handler option overrides and response fields."""

from __future__ import annotations

from collections.abc import AsyncIterator
from http import HTTPStatus
from unittest.mock import AsyncMock, Mock

from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.models import LLMGenerateResponse
from news_creator.handler.generate_handler import create_generate_router
from news_creator.port.llm_provider_port import LLMProviderPort

EXPECTED_NUM_PREDICT = 128
EXPECTED_PROMPT_EVAL_COUNT = 42
EXPECTED_EVAL_COUNT = 17
EXPECTED_TOTAL_DURATION_NS = 123_456


async def _empty_stream() -> AsyncIterator[LLMGenerateResponse]:
    if False:
        yield LLMGenerateResponse(response="", model="unused", done=True)


def _make_generate_client(provider: LLMProviderPort) -> TestClient:
    app = FastAPI()
    app.include_router(create_generate_router(provider))
    return TestClient(app)


def test_generate_handler_strips_num_ctx_and_forwards_num_predict_and_metrics() -> None:
    provider = Mock(spec=LLMProviderPort)
    provider.generate = AsyncMock(
        return_value=LLMGenerateResponse(
            response="ok",
            model="test-model",
            done=True,
            done_reason="stop",
            prompt_eval_count=EXPECTED_PROMPT_EVAL_COUNT,
            eval_count=EXPECTED_EVAL_COUNT,
            total_duration=EXPECTED_TOTAL_DURATION_NS,
        )
    )
    client = _make_generate_client(provider)

    response = client.post(
        "/api/generate",
        json={
            "prompt": "hello",
            "options": {
                "num_ctx": 8192,
                "num_predict": str(EXPECTED_NUM_PREDICT),
                "temperature": 0.2,
            },
        },
    )

    assert response.status_code == HTTPStatus.OK
    body = response.json()
    assert body["prompt_eval_count"] == EXPECTED_PROMPT_EVAL_COUNT
    assert body["eval_count"] == EXPECTED_EVAL_COUNT
    assert body["total_duration"] == EXPECTED_TOTAL_DURATION_NS
    call_kwargs = provider.generate.call_args.kwargs
    assert call_kwargs["num_predict"] == EXPECTED_NUM_PREDICT
    assert call_kwargs["options"] == {"temperature": 0.2}


def test_generate_handler_invalid_num_predict_falls_back_to_none() -> None:
    provider = Mock(spec=LLMProviderPort)
    provider.generate = AsyncMock(
        return_value=LLMGenerateResponse(
            response="ok",
            model="test-model",
            done=True,
            done_reason="stop",
        )
    )
    client = _make_generate_client(provider)

    response = client.post(
        "/api/generate",
        json={"prompt": "hello", "options": {"num_predict": "not-an-int"}},
    )

    assert response.status_code == HTTPStatus.OK
    assert provider.generate.call_args.kwargs["num_predict"] is None


def test_generate_handler_unexpected_stream_return_returns_500() -> None:
    provider = Mock(spec=LLMProviderPort)
    provider.generate = AsyncMock(return_value=_empty_stream())
    client = _make_generate_client(provider)

    response = client.post("/api/generate", json={"prompt": "hello"})

    assert response.status_code == HTTPStatus.INTERNAL_SERVER_ERROR
    assert response.json()["detail"] == "Internal server error"
