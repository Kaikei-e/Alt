"""Branch characterization tests for summarize handler stream cleanup."""

import asyncio
import importlib
from collections.abc import AsyncGenerator
from http import HTTPStatus
from typing import Self
from unittest.mock import Mock

import httpx
import pytest
from fastapi import FastAPI

import news_creator.handler.summarize_handler as mod
from news_creator.usecase.summarize_usecase import SummarizeUsecase

_VALID_CONTENT_PADDING = "A" * 120
_STREAM_WAIT_TIMEOUT_SECONDS = 1.0


class _FailingCloseStream(AsyncGenerator[str]):
    """Async stream whose aclose() raises after yielding chunks."""

    def __init__(self, chunks: list[str]) -> None:
        self._chunks = chunks
        self._index = 0
        self.aclose_called = False

    def __aiter__(self) -> Self:
        return self

    async def __anext__(self) -> str:
        if self._index >= len(self._chunks):
            raise StopAsyncIteration
        chunk = self._chunks[self._index]
        self._index += 1
        return chunk

    async def aclose(self) -> None:
        self.aclose_called = True
        msg = "simulated stream aclose failure"
        raise RuntimeError(msg)

    async def asend(self, value: None) -> str:  # pragma: no cover - protocol stub
        raise StopAsyncIteration

    async def athrow(
        self,
        typ: object,
        val: object = None,
        tb: object = None,
    ) -> str:  # pragma: no cover - protocol stub
        raise StopAsyncIteration


@pytest.mark.asyncio
async def test_summarize_stream_terminates_when_aclose_raises() -> None:
    """When stream aclose() raises, sentinel must still stop the SSE response."""
    importlib.reload(mod)
    failing_stream = _FailingCloseStream(["hello"])
    mock_usecase = Mock(spec=SummarizeUsecase)
    mock_usecase.generate_summary_stream = Mock(return_value=failing_stream)

    app = FastAPI()
    app.include_router(mod.create_summarize_router(mock_usecase))

    async def _collect_sse() -> tuple[int, str]:
        transport = httpx.ASGITransport(app=app)
        async with (
            httpx.AsyncClient(
                transport=transport, base_url="http://testserver"
            ) as client,
            client.stream(
                "POST",
                "/api/v1/summarize",
                json={
                    "article_id": "art-aclose-error",
                    "content": _VALID_CONTENT_PADDING,
                    "stream": True,
                },
            ) as response,
        ):
            body = (await response.aread()).decode("utf-8")
            return response.status_code, body

    status_code, body = await asyncio.wait_for(
        _collect_sse(), timeout=_STREAM_WAIT_TIMEOUT_SECONDS
    )

    assert status_code == HTTPStatus.OK
    assert failing_stream.aclose_called is True
    assert 'data: "hello"' in body
    assert "event: error" in body
