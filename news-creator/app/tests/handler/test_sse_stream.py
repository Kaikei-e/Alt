"""Unit tests for the canonical SSE / NDJSON streaming helper (sse_stream.py)."""

import asyncio
import json
from collections.abc import AsyncGenerator, AsyncIterator
from typing import Any, Self
from unittest.mock import AsyncMock

import pytest
from fastapi import Request

from news_creator.handler.sse_stream import (
    SSE_HEADERS,
    SSEHeartbeatConfig,
    create_ndjson_response,
    create_sse_response,
    stream_ndjson,
    stream_sse_with_heartbeat,
)


def _make_request(*, disconnect_after: int | None = None) -> Request:
    """Create a minimal mock Request whose is_disconnected() returns False (or True after N calls)."""
    calls = 0

    async def _is_disconnected() -> bool:
        nonlocal calls
        calls += 1
        return disconnect_after is not None and calls >= disconnect_after

    req = AsyncMock(spec=Request)
    req.is_disconnected = _is_disconnected
    return req


class _TrackingStream(AsyncGenerator[str]):
    """AsyncGenerator that tracks aclose() and can optionally fail during iteration or close."""

    def __init__(
        self,
        chunks: list[str],
        *,
        fail_after: int | None = None,
        fail_on_close: bool = False,
        chunk_delay: float = 0.0,
    ) -> None:
        self._chunks = chunks
        self._index = 0
        self._fail_after = fail_after
        self._fail_on_close = fail_on_close
        self._chunk_delay = chunk_delay
        self.aclose_called = False

    def __aiter__(self) -> Self:
        return self

    async def __anext__(self) -> str:
        if self._fail_after is not None and self._index >= self._fail_after:
            msg = "simulated stream failure"
            raise RuntimeError(msg)
        if self._index >= len(self._chunks):
            raise StopAsyncIteration
        if self._chunk_delay > 0:
            await asyncio.sleep(self._chunk_delay)
        chunk = self._chunks[self._index]
        self._index += 1
        return chunk

    async def aclose(self) -> None:
        self.aclose_called = True
        if self._fail_on_close:
            msg = "simulated aclose failure"
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
async def test_stream_sse_emits_data_and_closes_stream() -> None:
    stream = _TrackingStream(["alpha", "beta"])
    req = _make_request()

    events = [
        event
        async for event in stream_sse_with_heartbeat(
            stream, req, stream_id="art-1", error_message="summary generation failed"
        )
    ]

    assert events == ['data: "alpha"\n\n', 'data: "beta"\n\n']
    assert stream.aclose_called is True


@pytest.mark.asyncio
async def test_stream_sse_emits_error_and_terminates_on_aclose_failure() -> None:
    """C-07 invariant: an exception in aclose() must still emit error and terminal sentinel."""
    stream = _TrackingStream(["first"], fail_on_close=True)
    req = _make_request()

    async def _collect() -> list[str]:
        return [
            event
            async for event in stream_sse_with_heartbeat(
                stream,
                req,
                stream_id="art-aclose-err",
                error_message="summary generation failed",
            )
        ]

    events = await asyncio.wait_for(_collect(), timeout=1.0)

    assert stream.aclose_called is True
    assert events[0] == 'data: "first"\n\n'
    assert events[1] == (
        f"event: error\ndata: {json.dumps({'error': 'summary generation failed'})}\n\n"
    )


@pytest.mark.asyncio
async def test_stream_sse_emits_error_and_closes_on_midstream_exception() -> None:
    stream = _TrackingStream(["chunk-1", "chunk-2"], fail_after=1)
    req = _make_request()

    events = await asyncio.wait_for(
        _collect_events(
            stream_sse_with_heartbeat(
                stream,
                req,
                stream_id="art-mid-err",
                error_message="stream broken",
            )
        ),
        timeout=1.0,
    )

    assert stream.aclose_called is True
    assert events[0] == 'data: "chunk-1"\n\n'
    assert "event: error" in events[1]
    assert "stream broken" in events[1]


@pytest.mark.asyncio
async def test_stream_sse_stops_and_closes_on_client_disconnect() -> None:
    stream = _TrackingStream(["one", "two", "three"])
    req = _make_request(disconnect_after=1)

    events = await asyncio.wait_for(
        _collect_events(
            stream_sse_with_heartbeat(stream, req, stream_id="art-disconnect")
        ),
        timeout=1.0,
    )

    assert events == []
    assert stream.aclose_called is True


@pytest.mark.asyncio
async def test_stream_sse_emits_heartbeat_when_idle() -> None:
    stream = _TrackingStream(["late"], chunk_delay=0.25)
    req = _make_request()

    events = await asyncio.wait_for(
        _collect_events(
            stream_sse_with_heartbeat(
                stream,
                req,
                stream_id="art-hb",
                heartbeat_config=SSEHeartbeatConfig(
                    interval_seconds=0.05,
                    idle_threshold_seconds=0.02,
                ),
            )
        ),
        timeout=2.0,
    )

    assert ":\n\n" in events
    assert 'data: "late"\n\n' in events
    assert stream.aclose_called is True


def test_create_sse_response_sets_headers_and_media_type() -> None:
    stream = _TrackingStream([])
    req = _make_request()

    response = create_sse_response(stream, req, stream_id="art-headers")

    assert response.media_type == "text/event-stream; charset=utf-8"
    for key, value in SSE_HEADERS.items():
        assert response.headers[key] == value


@pytest.mark.asyncio
async def test_stream_ndjson_serializes_and_closes_inner_immediately() -> None:
    inner_closed = False

    async def _chunks() -> AsyncGenerator[dict[str, Any]]:
        nonlocal inner_closed
        try:
            yield {"message": {"content": "hello"}, "done": False}
            yield {"message": {"content": "world"}, "done": True}
        finally:
            inner_closed = True

    outer = stream_ndjson(_chunks())
    first = await anext(outer)
    assert json.loads(first) == {"message": {"content": "hello"}, "done": False}
    assert inner_closed is False

    await outer.aclose()
    assert inner_closed is True


def test_create_ndjson_response_sets_headers_and_media_type() -> None:
    async def _empty() -> AsyncGenerator[dict[str, Any]]:
        if False:  # pragma: no cover
            yield {}

    response = create_ndjson_response(_empty())
    assert response.media_type == "application/x-ndjson"
    for key, value in SSE_HEADERS.items():
        assert response.headers[key] == value


async def _collect_events(iterator: AsyncIterator[str]) -> list[str]:
    return [item async for item in iterator]
