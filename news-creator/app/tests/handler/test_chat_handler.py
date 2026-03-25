"""Tests for chat proxy handler — routes Ollama /api/chat through semaphore."""

import json
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from news_creator.handler.chat_handler import create_chat_router


def make_app(gateway: MagicMock) -> FastAPI:
    app = FastAPI()
    app.include_router(create_chat_router(gateway))
    return app


class TestChatEndpointStreaming:
    """Streaming /api/chat must acquire semaphore with HIGH priority."""

    def test_streaming_chat_returns_ndjson(self):
        gateway = AsyncMock()

        async def fake_chunks():
            yield {"message": {"role": "assistant", "content": "Hello"}, "done": False}
            yield {"message": {"role": "assistant", "content": ""}, "done": True, "done_reason": "stop"}

        gateway.chat_stream.return_value = fake_chunks()

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == 200
        lines = [l for l in resp.text.strip().split("\n") if l.strip()]
        assert len(lines) == 2
        first = json.loads(lines[0])
        assert first["message"]["content"] == "Hello"
        assert first["done"] is False
        last = json.loads(lines[1])
        assert last["done"] is True

    def test_streaming_chat_passes_full_payload(self):
        gateway = AsyncMock()

        async def empty():
            yield {"message": {"role": "assistant", "content": ""}, "done": True}

        gateway.chat_stream.return_value = empty()

        client = TestClient(make_app(gateway))
        payload = {
            "model": "gemma3-4b-8k",
            "messages": [{"role": "user", "content": "test"}],
            "stream": True,
            "keep_alive": -1,
            "options": {"temperature": 0.7, "num_predict": 2048},
        }
        client.post("/api/chat", json=payload)

        gateway.chat_stream.assert_awaited_once()
        call_payload = gateway.chat_stream.call_args[1]["payload"]
        assert call_payload["model"] == "gemma3-4b-8k"
        assert call_payload["messages"] == [{"role": "user", "content": "test"}]
        assert call_payload["options"]["temperature"] == 0.7

    def test_streaming_chat_queue_full_returns_429(self):
        from news_creator.gateway.hybrid_priority_semaphore import QueueFullError

        gateway = AsyncMock()
        gateway.chat_stream.side_effect = QueueFullError("queue full")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == 429
        assert "Retry-After" in resp.headers

    def test_streaming_chat_runtime_error_returns_502(self):
        gateway = AsyncMock()
        gateway.chat_stream.side_effect = RuntimeError("Ollama unreachable")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == 502

    def test_streaming_chat_unexpected_error_returns_500(self):
        gateway = AsyncMock()
        gateway.chat_stream.side_effect = Exception("unexpected")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": True,
            },
        )

        assert resp.status_code == 500


class TestChatEndpointValidation:
    """Request validation."""

    def test_missing_messages_returns_422(self):
        gateway = AsyncMock()
        client = TestClient(make_app(gateway))
        resp = client.post("/api/chat", json={"model": "test"})
        assert resp.status_code == 422

    def test_empty_messages_returns_422(self):
        gateway = AsyncMock()
        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={"model": "test", "messages": []},
        )
        assert resp.status_code == 422

    def test_non_streaming_calls_chat_generate(self):
        """Non-streaming chat routes to gateway.chat_generate()."""
        gateway = AsyncMock()
        gateway.chat_generate.return_value = {
            "model": "gemma3-4b-8k",
            "message": {"role": "assistant", "content": "Hello!"},
            "done": True,
        }

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
            },
        )

        assert resp.status_code == 200
        data = resp.json()
        assert data["message"]["content"] == "Hello!"
        gateway.chat_generate.assert_awaited_once()
        call_payload = gateway.chat_generate.call_args[1]["payload"]
        assert call_payload["model"] == "gemma3-4b-8k"

    def test_non_streaming_passes_options(self):
        """Non-streaming chat passes options to gateway."""
        gateway = AsyncMock()
        gateway.chat_generate.return_value = {
            "model": "gemma3-4b-8k",
            "message": {"role": "assistant", "content": "ok"},
            "done": True,
        }

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
                "options": {"num_predict": 4096},
            },
        )

        assert resp.status_code == 200
        call_payload = gateway.chat_generate.call_args[1]["payload"]
        assert call_payload["options"]["num_predict"] == 4096

    def test_non_streaming_queue_full_returns_429(self):
        """Non-streaming chat returns 429 when queue is full."""
        from news_creator.gateway.hybrid_priority_semaphore import QueueFullError

        gateway = AsyncMock()
        gateway.chat_generate.side_effect = QueueFullError("queue full")

        client = TestClient(make_app(gateway))
        resp = client.post(
            "/api/chat",
            json={
                "model": "gemma3-4b-8k",
                "messages": [{"role": "user", "content": "hi"}],
                "stream": False,
            },
        )
        assert resp.status_code == 429
