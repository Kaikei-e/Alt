"""Tests for chat proxy handler — routes Ollama /api/chat through semaphore."""

import gc
import json
from collections.abc import AsyncIterator
from http import HTTPStatus
from typing import Any
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.domain.errors import QueueFullError
from news_creator.handler.chat_handler import (
    ChatMessage,
    ChatRequest,
    create_chat_router,
)


def make_app(gateway: MagicMock, config: MagicMock | None = None) -> FastAPI:
    if config is None:
        config = MagicMock()
        config.model_routing_enabled = False
    app = FastAPI()
    app.include_router(create_chat_router(gateway, config))
    return app


class TestChatEndpointStreaming:
    """Streaming /api/chat must acquire semaphore with HIGH priority."""

    def test_streaming_chat_returns_ndjson(self) -> None:
        gateway = AsyncMock()
        stream_chunks = [
            {"message": {"role": "assistant", "content": "Hello"}, "done": False},
            {
                "message": {"role": "assistant", "content": ""},
                "done": True,
                "done_reason": "stop",
            },
        ]

        async def fake_chunks() -> AsyncIterator[dict[str, Any]]:
            for item in stream_chunks:
                yield item

        gateway.chat_stream.return_value = fake_chunks()

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == HTTPStatus.OK
        lines = [line for line in resp.text.strip().split("\n") if line.strip()]
        assert len(lines) == len(stream_chunks)
        first = json.loads(lines[0])
        assert first["message"]["content"] == "Hello"
        assert first["done"] is False
        last = json.loads(lines[1])
        assert last["done"] is True

    def test_streaming_chat_passes_full_payload(self) -> None:
        gateway = AsyncMock()

        async def empty() -> AsyncIterator[dict[str, Any]]:
            yield {"message": {"role": "assistant", "content": ""}, "done": True}

        gateway.chat_stream.return_value = empty()

        client = TestClient(make_app(gateway))
        requested_options = {"temperature": 0.7, "num_predict": 2048}
        payload = {
            "model": "gemma4-e4b-12k",
            "messages": [{"role": "user", "content": "test"}],
            "stream": True,
            "keep_alive": -1,
            "options": requested_options,
        }
        client.post("/api/chat", json=payload)

        gateway.chat_stream.assert_awaited_once()
        call_payload = gateway.chat_stream.call_args[1]["payload"]
        assert call_payload["model"] == "gemma4-e4b-12k"
        assert call_payload["messages"] == [{"role": "user", "content": "test"}]
        assert (
            call_payload["options"]["temperature"] == requested_options["temperature"]
        )

    def test_streaming_chat_passes_think_false(self) -> None:
        """Streaming chat must forward think parameter to gateway."""
        gateway = AsyncMock()

        async def empty() -> AsyncIterator[dict[str, Any]]:
            yield {"message": {"role": "assistant", "content": ""}, "done": True}

        gateway.chat_stream.return_value = empty()

        client = TestClient(make_app(gateway))
        client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
                "think": False,
            },
        )

        call_payload = gateway.chat_stream.call_args[1]["payload"]
        assert "think" in call_payload, "think must be forwarded to gateway"
        assert call_payload["think"] is False

    def test_streaming_chat_queue_full_returns_429(self) -> None:
        gateway = AsyncMock()
        gateway.chat_stream.side_effect = QueueFullError("queue full")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == HTTPStatus.TOO_MANY_REQUESTS
        assert "Retry-After" in resp.headers

    def test_streaming_chat_runtime_error_returns_502(self) -> None:
        gateway = AsyncMock()
        gateway.chat_stream.side_effect = RuntimeError("Ollama unreachable")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == HTTPStatus.BAD_GATEWAY

    def test_streaming_chat_unexpected_error_returns_500(self) -> None:
        gateway = AsyncMock()
        gateway.chat_stream.side_effect = Exception("unexpected")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == HTTPStatus.INTERNAL_SERVER_ERROR

    @pytest.mark.asyncio
    async def test_streaming_chat_aclose_closes_inner_stream_without_gc(self) -> None:
        """Closing ndjson_generator early must immediately close gateway.chat_stream generator without GC."""
        gateway = AsyncMock()
        inner_closed = False

        async def fake_chunks() -> AsyncIterator[dict[str, Any]]:
            nonlocal inner_closed
            try:
                yield {
                    "message": {"role": "assistant", "content": "Hello"},
                    "done": False,
                }
                yield {
                    "message": {"role": "assistant", "content": " world"},
                    "done": True,
                }
            finally:
                inner_closed = True

        inner_gen = fake_chunks()
        gateway.chat_stream.return_value = inner_gen

        config = MagicMock()
        config.model_routing_enabled = False
        router = create_chat_router(gateway, config)
        chat_route = next(
            r for r in router.routes if getattr(r, "path", None) == "/api/chat"
        )

        gc_was_enabled = gc.isenabled()
        gc.disable()
        try:
            resp = await chat_route.endpoint(
                ChatRequest(
                    model="gemma4-e4b-12k",
                    messages=[ChatMessage(role="user", content="hi")],
                    stream=True,
                )
            )
            first_line = await anext(resp.body_iterator)
            assert json.loads(first_line)["message"]["content"] == "Hello"
            assert not inner_closed
            await resp.body_iterator.aclose()
            assert inner_closed, (
                "Inner gateway.chat_stream generator must be closed immediately when ndjson_generator is closed"
            )
        finally:
            if gc_was_enabled:
                gc.enable()
            await inner_gen.aclose()


class TestChatEndpointValidation:
    """Request validation."""

    def test_missing_messages_returns_422(self) -> None:
        gateway = AsyncMock()
        client = TestClient(make_app(gateway))
        resp = client.post("/api/chat", json={"model": "test"})
        assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY

    def test_empty_messages_returns_422(self) -> None:
        gateway = AsyncMock()
        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={"model": "test", "messages": []},
        )
        assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY

    def test_unknown_model_rejected_when_routing_enabled(self) -> None:
        """When model routing is enabled, unknown models must be rejected with 400."""
        gateway = AsyncMock()
        config = MagicMock()
        config.model_routing_enabled = True
        config.is_base_model_name.return_value = False
        config.is_bucket_model_name.return_value = False

        client = TestClient(make_app(gateway, config))
        resp = client.post(
            "/api/chat",
            json={
                "model": "unsupported-model-v1",
                "messages": [{"role": "user", "content": "hi"}],
            },
        )
        assert resp.status_code == HTTPStatus.BAD_REQUEST
        assert resp.json()["detail"] == "Unknown model"

    def test_non_streaming_calls_chat_generate(self) -> None:
        """Non-streaming chat routes to gateway.chat_generate()."""
        gateway = AsyncMock()
        gateway.chat_generate.return_value = {
            "model": "gemma4-e4b-12k",
            "message": {"role": "assistant", "content": "Hello!"},
            "done": True,
        }

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
            },
        )

        assert resp.status_code == HTTPStatus.OK
        data = resp.json()
        assert data["message"]["content"] == "Hello!"
        gateway.chat_generate.assert_awaited_once()
        call_payload = gateway.chat_generate.call_args[1]["payload"]
        assert call_payload["model"] == "gemma4-e4b-12k"

    def test_non_streaming_passes_options(self) -> None:
        """Non-streaming chat passes options to gateway."""
        gateway = AsyncMock()
        gateway.chat_generate.return_value = {
            "model": "gemma4-e4b-12k",
            "message": {"role": "assistant", "content": "ok"},
            "done": True,
        }

        client = TestClient(make_app(gateway))
        requested_num_predict = 4096
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
                "options": {"num_predict": requested_num_predict},
            },
        )

        assert resp.status_code == HTTPStatus.OK
        call_payload = gateway.chat_generate.call_args[1]["payload"]
        assert call_payload["options"]["num_predict"] == requested_num_predict

    def test_non_streaming_passes_think_false(self) -> None:
        """Non-streaming chat must forward think parameter to gateway."""
        gateway = AsyncMock()
        gateway.chat_generate.return_value = {
            "model": "gemma4-e4b-12k",
            "message": {"role": "assistant", "content": "ok"},
            "done": True,
        }

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
                "think": False,
            },
        )

        assert resp.status_code == HTTPStatus.OK
        call_payload = gateway.chat_generate.call_args[1]["payload"]
        assert "think" in call_payload, "think must be forwarded to gateway"
        assert call_payload["think"] is False

    def test_non_streaming_queue_full_returns_429(self) -> None:
        """Non-streaming chat returns 429 when queue is full."""
        gateway = AsyncMock()
        gateway.chat_generate.side_effect = QueueFullError("queue full")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
            },
        )
        assert resp.status_code == HTTPStatus.TOO_MANY_REQUESTS

    def test_invalid_keep_alive_string_returns_422(self) -> None:
        """G7-05: Arbitrary keep_alive strings must be rejected with 422."""
        gateway = AsyncMock()
        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "keep_alive": "invalid-duration",
            },
        )
        assert resp.status_code == HTTPStatus.UNPROCESSABLE_ENTITY

    def test_valid_keep_alive_string_accepted(self) -> None:
        """G7-05: Valid Ollama duration strings (5m, 30s, 1h, 0, -1) are accepted."""
        gateway = AsyncMock()
        gateway.chat_generate.return_value = {
            "model": "gemma4-e4b-12k",
            "message": {"role": "assistant", "content": "ok"},
            "done": True,
        }
        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma4-e4b-12k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
                "keep_alive": "5m",
            },
        )
        assert resp.status_code == HTTPStatus.OK
