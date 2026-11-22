"""Canonical SSE and NDJSON streaming helpers with guaranteed aclosing cleanup.

Every HTTP streaming endpoint in news-creator must route its async generator
through this module so:
1. Upstream async generators are always wrapped in ``contextlib.aclosing``.
2. Close-time errors inside ``aclose()`` cannot bypass error reporting or the
   terminal SSE sentinel (preventing hung responses).
3. Streaming headers (no-cache, keep-alive, X-Accel-Buffering) stay consistent.
"""

import asyncio
import json
import logging
from collections.abc import AsyncGenerator, AsyncIterator
from contextlib import aclosing, suppress
from dataclasses import dataclass
from typing import Any

from fastapi import Request
from fastapi.responses import StreamingResponse

logger = logging.getLogger(__name__)

_DEFAULT_HEARTBEAT_INTERVAL_SECONDS = 10.0
_DEFAULT_HEARTBEAT_IDLE_THRESHOLD_SECONDS = 5.0
_INITIAL_CHUNK_LOG_LIMIT = 3
_CHUNK_PREVIEW_LENGTH = 50

SSE_HEADERS: dict[str, str] = {
    "Cache-Control": "no-cache",
    "Connection": "keep-alive",
    "X-Accel-Buffering": "no",
}


@dataclass(frozen=True)
class SSEHeartbeatConfig:
    """Timing configuration for idle SSE heartbeat comments."""

    interval_seconds: float = _DEFAULT_HEARTBEAT_INTERVAL_SECONDS
    idle_threshold_seconds: float = _DEFAULT_HEARTBEAT_IDLE_THRESHOLD_SECONDS


@dataclass(frozen=True)
class _SSEStreamContext:
    http_request: Request
    stopped: asyncio.Event
    last_data_time_ref: list[float]
    stream_id: str
    error_message: str
    heartbeat: SSEHeartbeatConfig


async def _heartbeat_loop(
    ctx: _SSEStreamContext,
    heartbeat_queue: asyncio.Queue[tuple[str, str] | None],
) -> None:
    """Send heartbeat comments periodically, but only when no data is flowing."""
    try:
        while not ctx.stopped.is_set():
            try:
                await asyncio.wait_for(
                    ctx.stopped.wait(),
                    timeout=ctx.heartbeat.interval_seconds,
                )
                break
            except TimeoutError:
                pass
            if await ctx.http_request.is_disconnected():
                ctx.stopped.set()
                break

            current_time = asyncio.get_running_loop().time()
            time_since_data = current_time - ctx.last_data_time_ref[0]

            if time_since_data >= ctx.heartbeat.idle_threshold_seconds:
                logger.debug(
                    "Sending SSE heartbeat",
                    extra={
                        "article_id": ctx.stream_id,
                        "time_since_data": f"{time_since_data:.2f}s",
                    },
                )
                await heartbeat_queue.put(("heartbeat", ":\n\n"))
    except asyncio.CancelledError:
        pass
    finally:
        ctx.stopped.set()
        await heartbeat_queue.put(None)  # Sentinel to stop


async def _forward_stream_chunks(
    original_stream: AsyncGenerator[str],
    ctx: _SSEStreamContext,
    data_queue: asyncio.Queue[tuple[str, str] | None],
) -> None:
    """Forward data from original stream with guaranteed aclosing and terminal sentinel."""
    chunk_count = 0
    try:
        async with aclosing(original_stream) as stream:
            async for chunk in stream:
                if await ctx.http_request.is_disconnected():
                    ctx.stopped.set()
                    break
                chunk_count += 1
                if chunk_count <= _INITIAL_CHUNK_LOG_LIMIT:
                    logger.debug(
                        "Streaming chunk to client",
                        extra={
                            "article_id": ctx.stream_id,
                            "chunk_count": chunk_count,
                            "chunk_size": len(chunk) if chunk else 0,
                            "chunk_preview": chunk[:_CHUNK_PREVIEW_LENGTH]
                            if chunk and len(chunk) > _CHUNK_PREVIEW_LENGTH
                            else chunk,
                        },
                    )
                ctx.last_data_time_ref[0] = asyncio.get_running_loop().time()
                await data_queue.put(("data", chunk))
    except Exception as e:
        logger.exception(
            "Error in stream task",
            extra={
                "error": str(e),
                "article_id": ctx.stream_id,
            },
        )
        # Surface the failure to the client instead of letting the stream end
        # silently and look like a normal (if short) completed stream.
        await data_queue.put(("error", ctx.error_message))
    finally:
        ctx.stopped.set()
        await data_queue.put(None)  # Sentinel to stop


def _format_sse_item(item: tuple[str, str], stream_id: str) -> str | None:
    item_type, content = item
    if item_type == "data":
        logger.debug(
            "Yielding data chunk to client",
            extra={
                "article_id": stream_id,
                "chunk_size": len(content) if content else 0,
            },
        )
        return f"data: {json.dumps(content)}\n\n"
    if item_type == "error":
        logger.debug(
            "Yielding SSE error event",
            extra={
                "article_id": stream_id,
                "error": content,
            },
        )
        return f"event: error\ndata: {json.dumps({'error': content})}\n\n"
    return None


async def _drain_heartbeat(
    heartbeat_queue: asyncio.Queue[tuple[str, str] | None],
    stream_id: str,
) -> str | None:
    try:
        heartbeat_item = heartbeat_queue.get_nowait()
    except asyncio.QueueEmpty:
        return None
    if heartbeat_item is None:
        return None
    heartbeat_type, heartbeat_content = heartbeat_item
    if heartbeat_type == "heartbeat":
        logger.debug(
            "Yielding SSE heartbeat",
            extra={"article_id": stream_id},
        )
        return heartbeat_content
    return None


async def stream_sse_with_heartbeat(
    original_stream: AsyncGenerator[str],
    http_request: Request,
    *,
    stream_id: str,
    error_message: str = "stream generation failed",
    heartbeat_config: SSEHeartbeatConfig | None = None,
) -> AsyncIterator[str]:
    """Wrap an async text generator with heartbeat comments and guaranteed cleanup.

    Prioritizes data chunks over heartbeats to ensure real-time streaming.
    Heartbeats are sent only when no data is flowing for at least
    ``heartbeat_config.idle_threshold_seconds``.
    """
    hb_config = heartbeat_config or SSEHeartbeatConfig()
    data_queue: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
    heartbeat_queue: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
    stopped = asyncio.Event()
    ctx = _SSEStreamContext(
        http_request=http_request,
        stopped=stopped,
        last_data_time_ref=[asyncio.get_running_loop().time()],
        stream_id=stream_id,
        error_message=error_message,
        heartbeat=hb_config,
    )

    heartbeat = asyncio.create_task(_heartbeat_loop(ctx, heartbeat_queue))
    stream = asyncio.create_task(
        _forward_stream_chunks(original_stream, ctx, data_queue)
    )

    try:
        while True:
            try:
                item = await asyncio.wait_for(data_queue.get(), timeout=0.1)
                if item is None:
                    break
                formatted = _format_sse_item(item, stream_id)
                if formatted is not None:
                    yield formatted
                if await http_request.is_disconnected():
                    stopped.set()
                    break
            except TimeoutError:
                hb = await _drain_heartbeat(heartbeat_queue, stream_id)
                if hb is not None:
                    yield hb
                    if await http_request.is_disconnected():
                        stopped.set()
                        break
    finally:
        stopped.set()
        heartbeat.cancel()
        stream.cancel()
        with suppress(Exception):
            # Partial-failure allowed: cancelled tasks during SSE cleanup.
            await asyncio.gather(heartbeat, stream, return_exceptions=True)


def create_sse_response(
    original_stream: AsyncGenerator[str],
    http_request: Request,
    *,
    stream_id: str,
    error_message: str = "stream generation failed",
    heartbeat_config: SSEHeartbeatConfig | None = None,
) -> StreamingResponse:
    """Build a standard SSE StreamingResponse backed by ``stream_sse_with_heartbeat``."""
    return StreamingResponse(
        stream_sse_with_heartbeat(
            original_stream,
            http_request,
            stream_id=stream_id,
            error_message=error_message,
            heartbeat_config=heartbeat_config,
        ),
        media_type="text/event-stream; charset=utf-8",
        headers=dict(SSE_HEADERS),
    )


async def stream_ndjson(
    stream_iter: AsyncGenerator[dict[str, Any]],
) -> AsyncIterator[str]:
    """Serialize an async dict generator to NDJSON lines with guaranteed ``aclosing``."""
    async with aclosing(stream_iter) as stream:
        async for chunk in stream:
            yield json.dumps(chunk, ensure_ascii=False) + "\n"


def create_ndjson_response(
    stream_iter: AsyncGenerator[dict[str, Any]],
) -> StreamingResponse:
    """Build a standard NDJSON StreamingResponse backed by ``stream_ndjson``."""
    return StreamingResponse(
        stream_ndjson(stream_iter),
        media_type="application/x-ndjson",
        headers=dict(SSE_HEADERS),
    )
