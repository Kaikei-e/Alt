"""Tests for VllmGateway — OpenAI-compatible API for vLLM inference."""

from __future__ import annotations

import json
from collections.abc import Callable, Coroutine
from typing import Any

import httpx
import pytest

from acolyte.config.settings import Settings
from acolyte.gateway.vllm_gw import VllmGateway
from acolyte.port.llm_provider import LLMMode


def _make_settings(**overrides: Any) -> Settings:  # noqa: ANN401 — heterogeneous Settings field overrides
    defaults: dict[str, Any] = {
        "news_creator_url": "http://test-vllm:8000/v1",
        "default_model": "qwen3.5-27b",
        "default_num_predict": 2000,
        "llm_provider": "vllm",
        "vllm_api_key": "test-key",
    }
    defaults.update(overrides)
    return Settings(**defaults)


def _mock_transport(handler: Callable[[httpx.Request], Coroutine[None, None, httpx.Response]]) -> httpx.AsyncClient:
    """Create an httpx.AsyncClient with a mock transport."""
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


def _openai_response(
    content: str = '{"result": "ok"}', model: str = "qwen3.5-27b", prompt_tokens: int = 50, completion_tokens: int = 100
) -> dict:
    """Build a minimal OpenAI-compatible chat completion response."""
    return {
        "id": "chatcmpl-test",
        "object": "chat.completion",
        "model": model,
        "choices": [
            {
                "index": 0,
                "message": {"role": "assistant", "content": content},
                "finish_reason": "stop",
            }
        ],
        "usage": {
            "prompt_tokens": prompt_tokens,
            "completion_tokens": completion_tokens,
            "total_tokens": prompt_tokens + completion_tokens,
        },
    }


# --- Endpoint routing ---


@pytest.mark.asyncio
async def test_uses_chat_completions_endpoint() -> None:
    """All requests must use /v1/chat/completions."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test prompt", temperature=0)

    assert len(captured) == 1
    assert "/v1/chat/completions" in str(captured[0].url)


# --- Structured output ---


@pytest.mark.asyncio
async def test_structured_mode_sends_response_format() -> None:
    """STRUCTURED mode wraps format dict into response_format.json_schema."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response('{"topics": ["AI"]}'))

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    schema = {"type": "object", "properties": {"topics": {"type": "array"}}}
    await gw.generate("list topics", output_schema=schema, mode=LLMMode.STRUCTURED)

    body = json.loads(captured[0].content)
    assert body["response_format"]["type"] == "json_schema"
    assert body["response_format"]["json_schema"]["schema"] == schema
    assert body["response_format"]["json_schema"]["name"] == "output"


@pytest.mark.asyncio
async def test_structured_mode_temperature_zero() -> None:
    """STRUCTURED mode defaults to temperature=0."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", output_schema={"type": "object"}, mode=LLMMode.STRUCTURED)

    body = json.loads(captured[0].content)
    assert body["temperature"] == 0


@pytest.mark.asyncio
async def test_structured_mode_disables_thinking() -> None:
    """STRUCTURED mode sends enable_thinking=false via chat_template_kwargs."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", output_schema={"type": "object"}, mode=LLMMode.STRUCTURED)

    body = json.loads(captured[0].content)
    assert body["chat_template_kwargs"]["enable_thinking"] is False


# --- Longform output ---


@pytest.mark.asyncio
async def test_longform_mode_enables_thinking() -> None:
    """LONGFORM mode with think=True sends enable_thinking=true."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response("Generated text."))

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("write something", mode=LLMMode.LONGFORM, think=True)

    body = json.loads(captured[0].content)
    assert body["chat_template_kwargs"]["enable_thinking"] is True
    assert "response_format" not in body


@pytest.mark.asyncio
async def test_longform_mode_temperature_default() -> None:
    """LONGFORM mode defaults to temperature=0.7."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response("Text."))

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", mode=LLMMode.LONGFORM)

    body = json.loads(captured[0].content)
    assert body["temperature"] == 0.7


# --- Response parsing ---


@pytest.mark.asyncio
async def test_response_extracts_content_and_usage() -> None:
    """LLMResponse must populate text, model, prompt_tokens, completion_tokens."""

    async def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, json=_openai_response("hello world", "qwen3.5-27b", 120, 450))

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    result = await gw.generate("test")

    assert result.text == "hello world"
    assert result.model == "qwen3.5-27b"
    assert result.prompt_tokens == 120
    assert result.completion_tokens == 450


# --- num_predict → max_tokens mapping ---


@pytest.mark.asyncio
async def test_num_predict_maps_to_max_tokens() -> None:
    """num_predict parameter must be sent as max_tokens in OpenAI format."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", num_predict=4096)

    body = json.loads(captured[0].content)
    assert body["max_tokens"] == 4096


@pytest.mark.asyncio
async def test_structured_num_predict_from_settings() -> None:
    """STRUCTURED mode uses structured_num_predict from settings."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings(structured_num_predict=2048))
    await gw.generate("test", output_schema={"type": "object"}, mode=LLMMode.STRUCTURED)

    body = json.loads(captured[0].content)
    assert body["max_tokens"] == 2048


@pytest.mark.asyncio
async def test_longform_num_predict_from_settings() -> None:
    """LONGFORM mode uses longform_num_predict from settings."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings(longform_num_predict=6000))
    await gw.generate("test", mode=LLMMode.LONGFORM)

    body = json.loads(captured[0].content)
    assert body["max_tokens"] == 6000


# --- Explicit kwargs override mode defaults ---


@pytest.mark.asyncio
async def test_explicit_temperature_overrides_mode_default() -> None:
    """Explicit temperature kwarg overrides mode defaults."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", output_schema={"type": "object"}, mode=LLMMode.STRUCTURED, temperature=0.5)

    body = json.loads(captured[0].content)
    assert body["temperature"] == 0.5


# --- Authorization header ---


@pytest.mark.asyncio
async def test_sends_authorization_header() -> None:
    """API key must be sent as Bearer token in Authorization header."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings(vllm_api_key="my-secret"))
    await gw.generate("test")

    assert captured[0].headers["authorization"] == "Bearer my-secret"


@pytest.mark.asyncio
async def test_no_auth_header_when_key_empty() -> None:
    """No Authorization header when vllm_api_key is empty."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings(vllm_api_key=""))
    await gw.generate("test")

    assert "authorization" not in captured[0].headers


# --- Message format ---


@pytest.mark.asyncio
async def test_prompt_sent_as_user_message() -> None:
    """Prompt string is sent as a single user message."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("my prompt text")

    body = json.loads(captured[0].content)
    assert body["messages"] == [{"role": "user", "content": "my prompt text"}]


# --- Model selection ---


@pytest.mark.asyncio
async def test_explicit_model_overrides_default() -> None:
    """Explicit model kwarg overrides settings default."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", model="custom-model")

    body = json.loads(captured[0].content)
    assert body["model"] == "custom-model"


# --- Error propagation ---


@pytest.mark.asyncio
async def test_http_error_raises() -> None:
    """HTTP errors must propagate as httpx.HTTPStatusError."""

    async def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(500, json={"error": "internal server error"})

    gw = VllmGateway(_mock_transport(handler), _make_settings())

    with pytest.raises(httpx.HTTPStatusError):
        await gw.generate("test")


# --- Fallback routing (no mode) ---


@pytest.mark.asyncio
async def test_no_mode_with_format_sends_response_format() -> None:
    """Without mode, format presence triggers response_format."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response())

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    await gw.generate("test", output_schema={"type": "object"}, temperature=0)

    body = json.loads(captured[0].content)
    assert "response_format" in body
    assert body["chat_template_kwargs"]["enable_thinking"] is False


@pytest.mark.asyncio
async def test_no_mode_without_format_is_freetext() -> None:
    """Without mode or format, no response_format is sent."""
    captured: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured.append(request)
        return httpx.Response(200, json=_openai_response("freetext output"))

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    result = await gw.generate("test", temperature=0.7)

    body = json.loads(captured[0].content)
    assert "response_format" not in body
    assert result.text == "freetext output"


@pytest.mark.asyncio
async def test_null_content_falls_back_to_empty_string() -> None:
    """Tool-call / filtered responses with content=null must not TypeError."""

    async def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "id": "chatcmpl-test",
                "model": "qwen3.5-27b",
                "choices": [{"index": 0, "message": {"role": "assistant", "content": None}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 1, "completion_tokens": 0, "total_tokens": 1},
            },
        )

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    result = await gw.generate("test")
    assert result.text == ""


@pytest.mark.asyncio
async def test_empty_choices_falls_back_to_empty_string() -> None:
    """Empty choices array must not IndexError."""

    async def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={"id": "chatcmpl-test", "model": "qwen3.5-27b", "choices": [], "usage": {}},
        )

    gw = VllmGateway(_mock_transport(handler), _make_settings())
    result = await gw.generate("test")
    assert result.text == ""
