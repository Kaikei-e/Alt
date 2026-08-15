"""Tests for summarize handler - HTTP 429 queue full behavior."""

import time

from fastapi import FastAPI
from fastapi.testclient import TestClient
from unittest.mock import AsyncMock, Mock

from news_creator.domain.models import SummaryMetadata
from news_creator.gateway.hybrid_priority_semaphore import QueueFullError


def _make_client(mock_usecase):
    """Create a fresh test client with a fresh router (avoid module-level router reuse)."""
    # Reload module to get a fresh router each time
    import importlib
    import news_creator.handler.summarize_handler as mod

    importlib.reload(mod)

    app = FastAPI()
    router = mod.create_summarize_router(mock_usecase)
    app.include_router(router)
    return TestClient(app)


def _make_mock_usecase(return_value=None, side_effect=None):
    """Create a mock SummarizeUsecase."""
    from news_creator.usecase.summarize_usecase import SummarizeUsecase

    mock = Mock(spec=SummarizeUsecase)
    if side_effect:
        mock.generate_summary = AsyncMock(side_effect=side_effect)
    else:
        mock.generate_summary = AsyncMock(
            return_value=return_value
            or (
                "テスト要約",
                SummaryMetadata(
                    model="test-model",
                    prompt_tokens=100,
                    completion_tokens=50,
                    total_duration_ms=1000.0,
                ),
            )
        )
    return mock


def test_summarize_returns_429_when_queue_full():
    """Test that summarize endpoint returns HTTP 429 when queue is full."""
    mock_usecase = _make_mock_usecase(
        side_effect=QueueFullError("Queue depth 20 >= max 20")
    )
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 429
    data = response.json()
    assert "queue full" in data["error"]
    assert response.headers.get("Retry-After") == "30"


def test_summarize_returns_200_on_success():
    """Test that summarize endpoint returns 200 on success."""
    mock_usecase = _make_mock_usecase()
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 200
    data = response.json()
    assert data["success"] is True
    assert data["summary"] == "テスト要約"


def test_empty_summary_returns_422():
    """RuntimeError with 'empty/whitespace summary' should return HTTP 422."""
    secret = (
        "LLM returned empty/whitespace summary 2 times consecutively for article secret-id. "
        "Model may be in a bad state."
    )
    mock_usecase = _make_mock_usecase(side_effect=RuntimeError(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 422
    assert response.json()["detail"] == "Content not processable"
    assert secret not in response.text


def test_other_runtime_error_returns_502():
    """RuntimeError without 'empty/whitespace' should still return HTTP 502."""
    secret = "LLM connection timeout to ollama.internal:11434"
    mock_usecase = _make_mock_usecase(side_effect=RuntimeError(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 502
    assert response.json()["detail"] == "Upstream service error"
    assert secret not in response.text


def test_summarize_value_error_does_not_leak_exception_text():
    """ValueError from usecase returns 400 without leaking exception text."""
    secret = (
        "invalid summarize request at /app/news_creator/usecase/summarize_usecase.py:12"
    )
    mock_usecase = _make_mock_usecase(side_effect=ValueError(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == 400
    assert response.json()["detail"] == "Invalid request"
    assert secret not in response.text


def test_summarize_stream_error_does_not_leak_exception_text():
    """SSE error events must not include exception / traceback text."""
    secret = "Ollama crash at /usr/lib/ollama/runner.py:412"

    async def failing_stream(*args, **kwargs):
        raise RuntimeError(secret)
        yield  # pragma: no cover — marks this as an async generator

    mock_usecase = _make_mock_usecase()
    mock_usecase.generate_summary_stream = failing_stream
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": True},
    )

    assert response.status_code == 200
    assert secret not in response.text
    assert "summary generation failed" in response.text


def test_summarize_stream_reaches_eof_right_after_last_chunk():
    """The SSE body must reach EOF as soon as the generator is done.

    Downstream consumers persist the summary only after reading EOF, so any
    delay between the final data chunk and the end of the body is data loss
    waiting to happen.
    """
    chunks = ["Alpha", "Beta", "Gamma"]

    async def fast_stream(*args, **kwargs):
        for chunk in chunks:
            yield chunk

    mock_usecase = _make_mock_usecase()
    mock_usecase.generate_summary_stream = fast_stream
    client = _make_client(mock_usecase)

    body = ""
    started_at = time.monotonic()
    with client.stream(
        "POST",
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": True},
    ) as response:
        assert response.status_code == 200
        for part in response.iter_text():
            body += part
    elapsed = time.monotonic() - started_at

    # The fake generator yields instantly, so anything close to the heartbeat
    # interval means the body was held open after the last chunk.
    assert elapsed < 1.0, f"stream stayed open for {elapsed:.2f}s"
    # SSE wire format is contractual - keep it byte-identical.
    assert body == "".join(f'data: "{chunk}"\n\n' for chunk in chunks)
