"""Tests for OllamaGateway — /api/chat for structured, /api/generate for freetext."""

from __future__ import annotations

import json

import httpx
import pytest

from acolyte.config.settings import Settings
from acolyte.gateway.ollama_gw import OllamaGateway


def _make_settings(**overrides) -> Settings:
    defaults = {
        "news_creator_url": "http://test-ollama:11434",
        "default_model": "gemma4:26b",
        "default_num_predict": 2000,
    }
    defaults.update(overrides)
    return Settings(**defaults)


def _mock_transport(handler):
    """Create an httpx.AsyncClient with a mock transport."""
    return httpx.AsyncClient(transport=httpx.MockTransport(handler))


# --- /api/chat vs /api/generate routing ---


@pytest.mark.asyncio
async def test_uses_chat_endpoint_for_structured_output():
    """When format is provided, gateway must use /api/chat."""
    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": '{"reasoning": "test", "sections": []}'},
                "model": "gemma4:26b",
                "eval_count": 100,
                "prompt_eval_count": 50,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    schema = {"type": "object", "properties": {"reasoning": {"type": "string"}}}
    result = await gw.generate("test prompt", format=schema, temperature=0)

    assert len(captured_requests) == 1
    assert "/api/chat" in str(captured_requests[0].url)
    body = json.loads(captured_requests[0].content)
    assert "messages" in body
    assert body["messages"][0]["role"] == "user"
    assert body["messages"][0]["content"] == "test prompt"
    assert body["format"] == schema
    assert result.text == '{"reasoning": "test", "sections": []}'


@pytest.mark.asyncio
async def test_uses_generate_endpoint_for_freetext():
    """When format is NOT provided, gateway must use /api/generate."""
    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "response": "Generated freetext content.",
                "model": "gemma4:26b",
                "eval_count": 200,
                "prompt_eval_count": 100,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    result = await gw.generate("write something", temperature=0.7)

    assert len(captured_requests) == 1
    assert "/api/generate" in str(captured_requests[0].url)
    body = json.loads(captured_requests[0].content)
    assert "prompt" in body
    assert "messages" not in body
    assert result.text == "Generated freetext content."


@pytest.mark.asyncio
async def test_chat_response_extracts_eval_count():
    """completion_tokens must be populated from eval_count in /api/chat response."""

    async def handler(request: httpx.Request) -> httpx.Response:
        return httpx.Response(
            200,
            json={
                "message": {"content": '{"result": "ok"}'},
                "model": "gemma4:26b",
                "eval_count": 512,
                "prompt_eval_count": 100,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    result = await gw.generate("test", format={"type": "object"}, temperature=0)

    assert result.completion_tokens == 512
    assert result.prompt_tokens == 100


@pytest.mark.asyncio
async def test_chat_does_not_send_think_parameter():
    """Structured output calls must NOT send think parameter (Gemma4 #15260)."""
    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "{}"},
                "model": "gemma4:26b",
                "eval_count": 10,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", format={"type": "object"}, temperature=0)

    body = json.loads(captured_requests[0].content)
    assert "think" not in body
    assert "think" not in body.get("options", {})


# --- LLMMode-based routing tests ---


@pytest.mark.asyncio
async def test_structured_mode_uses_chat_endpoint():
    """mode=STRUCTURED routes to /api/chat with temperature=0."""
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": '{"result": "ok"}'},
                "model": "gemma4:26b",
                "eval_count": 50,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", format={"type": "object"}, mode=LLMMode.STRUCTURED)

    assert len(captured_requests) == 1
    assert "/api/chat" in str(captured_requests[0].url)
    body = json.loads(captured_requests[0].content)
    assert body["options"]["temperature"] == 0


@pytest.mark.asyncio
async def test_longform_mode_uses_chat_endpoint():
    """mode=LONGFORM routes to /api/chat with think=false (top-level) to suppress thinking exhaustion.

    Ollama /api/generate ignores think=false for Qwen3.5 (#14793).
    /api/chat with top-level think=false works correctly for all models.
    """
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "Generated text."},
                "model": "gemma4:26b",
                "eval_count": 200,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    result = await gw.generate("test", mode=LLMMode.LONGFORM)

    assert len(captured_requests) == 1
    assert "/api/chat" in str(captured_requests[0].url)
    body = json.loads(captured_requests[0].content)
    assert body["options"]["temperature"] == 0.7
    # think=false must be top-level, NOT inside options
    assert body["think"] is False
    assert "think" not in body.get("options", {})
    # No format parameter for freetext
    assert "format" not in body
    # Messages format
    assert body["messages"][0]["role"] == "user"
    assert body["messages"][0]["content"] == "test"
    assert result.text == "Generated text."


@pytest.mark.asyncio
async def test_longform_think_setting_controls_think_param():
    """longform_think=True sends think=true for freetext generation."""
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "Thinking text."},
                "model": "gemma4:26b",
                "eval_count": 200,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings(longform_think=True))

    await gw.generate("test", mode=LLMMode.LONGFORM)

    body = json.loads(captured_requests[0].content)
    assert body["think"] is True


@pytest.mark.asyncio
async def test_structured_no_format_uses_chat_freetext():
    """mode=STRUCTURED without format routes to /api/chat with think=false.

    XML DSL nodes use STRUCTURED mode without format parameter.
    Must use /api/chat for Qwen3.5 think=false compatibility.
    """
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "<plan>test</plan>"},
                "model": "gemma4:26b",
                "eval_count": 100,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    result = await gw.generate("test", mode=LLMMode.STRUCTURED)

    assert len(captured_requests) == 1
    assert "/api/chat" in str(captured_requests[0].url)
    body = json.loads(captured_requests[0].content)
    assert body["think"] is False
    assert "format" not in body
    assert result.text == "<plan>test</plan>"


@pytest.mark.asyncio
async def test_mode_defaults_overridden_by_explicit_kwargs():
    """Explicit temperature overrides mode defaults."""
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "{}"},
                "model": "gemma4:26b",
                "eval_count": 10,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", format={"type": "object"}, mode=LLMMode.STRUCTURED, temperature=0.5)

    body = json.loads(captured_requests[0].content)
    assert body["options"]["temperature"] == 0.5


@pytest.mark.asyncio
async def test_mode_none_falls_back_to_format_routing():
    """mode=None with format present uses existing /api/chat routing."""
    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "{}"},
                "model": "gemma4:26b",
                "eval_count": 10,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", format={"type": "object"}, temperature=0)

    assert "/api/chat" in str(captured_requests[0].url)


@pytest.mark.asyncio
async def test_structured_mode_num_predict_from_settings():
    """mode=STRUCTURED uses structured_num_predict from settings when not explicit."""
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "{}"},
                "model": "gemma4:26b",
                "eval_count": 10,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings(structured_num_predict=2048))

    await gw.generate("test", format={"type": "object"}, mode=LLMMode.STRUCTURED)

    body = json.loads(captured_requests[0].content)
    assert body["options"]["num_predict"] == 2048


@pytest.mark.asyncio
async def test_freetext_forwards_top_p_and_top_k_into_options():
    """top_p / top_k kwargs must reach Ollama options. Gemma 4's official
    sampler (temperature=1.0, top_p=0.95, top_k=64) prevents CJK-induced
    empty responses when combined with think=False.
    """
    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "response": "Generated text.",
                "model": "gemma4:26b",
                "eval_count": 100,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", temperature=1.0, top_p=0.95, top_k=64)

    body = json.loads(captured_requests[0].content)
    assert body["options"]["top_p"] == 0.95
    assert body["options"]["top_k"] == 64
    assert body["options"]["temperature"] == 1.0


@pytest.mark.asyncio
async def test_chat_freetext_forwards_top_p_and_top_k_into_options():
    """Same forwarding contract for /api/chat freetext (LONGFORM / STRUCTURED-no-format)."""
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "Text."},
                "model": "gemma4:26b",
                "eval_count": 100,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", mode=LLMMode.LONGFORM, top_p=0.9, top_k=40)

    body = json.loads(captured_requests[0].content)
    assert body["options"]["top_p"] == 0.9
    assert body["options"]["top_k"] == 40


@pytest.mark.asyncio
async def test_top_p_and_top_k_omitted_when_not_provided():
    """Defaults: without caller-supplied top_p/top_k, keys must be absent
    from options so existing Ollama defaults apply (no silent regression).
    """
    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "response": "Text.",
                "model": "gemma4:26b",
                "eval_count": 10,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings())

    await gw.generate("test", temperature=0.0)

    body = json.loads(captured_requests[0].content)
    assert "top_p" not in body["options"]
    assert "top_k" not in body["options"]


@pytest.mark.asyncio
async def test_longform_mode_num_predict_from_settings():
    """mode=LONGFORM uses longform_num_predict from settings when not explicit."""
    from acolyte.port.llm_provider import LLMMode

    captured_requests: list[httpx.Request] = []

    async def handler(request: httpx.Request) -> httpx.Response:
        captured_requests.append(request)
        return httpx.Response(
            200,
            json={
                "message": {"content": "Text."},
                "model": "gemma4:26b",
                "eval_count": 100,
            },
        )

    client = _mock_transport(handler)
    gw = OllamaGateway(client, _make_settings(longform_num_predict=6000))

    await gw.generate("test", mode=LLMMode.LONGFORM)

    body = json.loads(captured_requests[0].content)
    assert body["options"]["num_predict"] == 6000
