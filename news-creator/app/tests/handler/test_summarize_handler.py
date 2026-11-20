"""Tests for summarize handler - HTTP 429 queue full behavior."""

import importlib
import time
from collections.abc import AsyncIterator
from http import HTTPStatus
from unittest.mock import AsyncMock, Mock

from fastapi import FastAPI
from fastapi.testclient import TestClient

import news_creator.handler.summarize_handler as mod
from news_creator.domain.errors import PreemptedException, QueueFullError
from news_creator.domain.models import SummaryMetadata
from news_creator.usecase.summarize_usecase import SummarizeUsecase


def _make_client(mock_usecase: Mock) -> TestClient:
    """Create a fresh test client with a fresh router (avoid module-level router reuse)."""
    # Reload module to get a fresh router each time
    importlib.reload(mod)

    app = FastAPI()
    router = mod.create_summarize_router(mock_usecase)
    app.include_router(router)
    return TestClient(app)


def _make_mock_usecase(
    return_value: tuple[str, SummaryMetadata] | None = None,
    side_effect: Exception | None = None,
) -> Mock:
    """Create a mock SummarizeUsecase."""
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


def test_summarize_returns_429_when_queue_full() -> None:
    """Test that summarize endpoint returns HTTP 429 when queue is full."""
    mock_usecase = _make_mock_usecase(
        side_effect=QueueFullError("Queue depth 20 >= max 20")
    )
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.TOO_MANY_REQUESTS
    data = response.json()
    assert "queue full" in data["error"]
    assert response.headers.get("Retry-After") == "30"


def test_summarize_returns_200_on_success() -> None:
    """Test that summarize endpoint returns 200 on success."""
    mock_usecase = _make_mock_usecase()
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.OK
    data = response.json()
    assert data["success"] is True
    assert data["summary"] == "テスト要約"


def test_empty_summary_returns_422() -> None:
    """RuntimeError with 'empty/whitespace summary' should return HTTP 422."""
    secret = (
        "LLM returned empty/whitespace summary 2 times consecutively for article secret-id. "  # noqa: S105 - test fixture string simulating leaked internal detail
        "Model may be in a bad state."
    )
    mock_usecase = _make_mock_usecase(side_effect=RuntimeError(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
    assert response.json()["detail"] == "Content not processable"
    assert secret not in response.text


def test_other_runtime_error_returns_502() -> None:
    """RuntimeError without 'empty/whitespace' should still return HTTP 502."""
    secret = "LLM connection timeout to ollama.internal:11434"  # noqa: S105 - test fixture string simulating leaked internal detail
    mock_usecase = _make_mock_usecase(side_effect=RuntimeError(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.BAD_GATEWAY
    assert response.json()["detail"] == "Upstream service error"
    assert secret not in response.text


def test_summarize_value_error_does_not_leak_exception_text() -> None:
    """ValueError from usecase returns 400 without leaking exception text."""
    secret = (
        "invalid summarize request at /app/news_creator/usecase/summarize_usecase.py:12"  # noqa: S105 - test fixture string simulating leaked internal detail
    )
    mock_usecase = _make_mock_usecase(side_effect=ValueError(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.BAD_REQUEST
    assert response.json()["detail"] == "Invalid request"
    assert secret not in response.text


def test_summarize_stream_error_does_not_leak_exception_text() -> None:
    """SSE error events must not include exception / traceback text."""
    secret = "Ollama crash at /usr/lib/ollama/runner.py:412"  # noqa: S105 - test fixture string simulating leaked internal detail

    async def failing_stream(*_args: object, **_kwargs: object) -> AsyncIterator[str]:
        raise RuntimeError(secret)
        yield ""  # pragma: no cover — marks this as an async generator

    mock_usecase = _make_mock_usecase()
    mock_usecase.generate_summary_stream = failing_stream
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "test-123", "content": "A" * 200, "stream": True},
    )

    assert response.status_code == HTTPStatus.OK
    assert secret not in response.text
    assert "summary generation failed" in response.text


def test_summarize_stream_reaches_eof_right_after_last_chunk() -> None:
    """The SSE body must reach EOF as soon as the generator is done.

    Downstream consumers persist the summary only after reading EOF, so any
    delay between the final data chunk and the end of the body is data loss
    waiting to happen.
    """
    chunks = ["Alpha", "Beta", "Gamma"]

    async def fast_stream(*_args: object, **_kwargs: object) -> AsyncIterator[str]:
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
        assert response.status_code == HTTPStatus.OK
        for part in response.iter_text():
            body += part
    elapsed = time.monotonic() - started_at

    # The fake generator yields instantly, so anything close to the heartbeat
    # interval means the body was held open after the last chunk.
    assert elapsed < 1.0, f"stream stayed open for {elapsed:.2f}s"
    # SSE wire format is contractual - keep it byte-identical.
    assert body == "".join(f'data: "{chunk}"\n\n' for chunk in chunks)


def test_summarize_short_content_fast_path_returns_400() -> None:
    """Content shorter than 100 chars is rejected with 400 before invoking usecase."""
    mock_usecase = _make_mock_usecase()
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "short-1", "content": "Too short", "stream": False},
    )

    assert response.status_code == HTTPStatus.BAD_REQUEST
    assert "Minimum required: 100 characters" in response.json()["detail"]
    mock_usecase.generate_summary.assert_not_called()


def test_summarize_empty_whitespace_runtime_error_returns_422() -> None:
    """RuntimeError with 'empty/whitespace summary' maps to 422 Unprocessable Entity."""
    mock_usecase = _make_mock_usecase()
    mock_usecase.generate_summary.side_effect = RuntimeError(
        "LLM repeatedly returned empty/whitespace summary"
    )
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "empty-1", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.UNPROCESSABLE_ENTITY
    assert response.json()["detail"] == "Content not processable"


def test_summarize_unexpected_exception_returns_500() -> None:
    """Unexpected generic Exception maps to 500 Internal Server Error."""
    mock_usecase = _make_mock_usecase()
    mock_usecase.generate_summary.side_effect = KeyError("unexpected internal key")
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "err-500", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.INTERNAL_SERVER_ERROR
    assert response.json()["detail"] == "Internal server error"


def test_summarize_preempted_returns_502() -> None:
    """PreemptedException must surface as HTTP 502 (matching recap handlers), never 500."""
    secret = "request preempted during generation at slot 2"  # noqa: S105 - test fixture string simulating leaked internal detail
    mock_usecase = _make_mock_usecase(side_effect=PreemptedException(secret))
    client = _make_client(mock_usecase)

    response = client.post(
        "/api/v1/summarize",
        json={"article_id": "preempt-1", "content": "A" * 200, "stream": False},
    )

    assert response.status_code == HTTPStatus.BAD_GATEWAY
    assert response.json()["detail"] == "Upstream service error"
    assert secret not in response.text
