"""Summarize handler - REST endpoint for article summarization."""

import asyncio
import json
import logging
from collections.abc import AsyncGenerator, AsyncIterator
from contextlib import aclosing, suppress
from dataclasses import dataclass

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from news_creator.domain.errors import PreemptedException, QueueFullError
from news_creator.domain.models import SummarizeRequest, SummarizeResponse
from news_creator.usecase.summarize_usecase import SummarizeUsecase
from news_creator.utils.context_logger import (
    clear_context,
    set_ai_pipeline,
    set_article_id,
    set_processing_stage,
)

logger = logging.getLogger(__name__)

_MIN_CONTENT_LENGTH = 100
_HEARTBEAT_INTERVAL_SECONDS = 10
_HEARTBEAT_IDLE_THRESHOLD_SECONDS = 5.0
_INITIAL_CHUNK_LOG_LIMIT = 3
_CHUNK_PREVIEW_LENGTH = 50


def _validate_summarize_content(request: SummarizeRequest) -> None:
    if not request.content or len(request.content.strip()) < _MIN_CONTENT_LENGTH:
        error_msg = (
            f"Content is too short for summarization. "
            f"Content length: {len(request.content) if request.content else 0}, "
            f"Minimum required: {_MIN_CONTENT_LENGTH} characters."
        )
        logger.warning(
            "Rejecting summarize request: content too short",
            extra={
                "article_id": request.article_id,
                "content_length": len(request.content) if request.content else 0,
                "min_required": _MIN_CONTENT_LENGTH,
            },
        )
        raise HTTPException(status_code=400, detail=error_msg)


@dataclass(frozen=True)
class _SSEStreamContext:
    http_request: Request
    stopped: asyncio.Event
    last_data_time_ref: list[float]
    article_id: str


async def _heartbeat_loop(
    ctx: _SSEStreamContext,
    heartbeat_queue: asyncio.Queue[tuple[str, str] | None],
) -> None:
    """Send heartbeat comments periodically, but only when no data is flowing."""
    try:
        while not ctx.stopped.is_set():
            # Interruptible sleep: wake as soon as the stream
            # is done instead of ticking out the whole interval.
            try:
                await asyncio.wait_for(
                    ctx.stopped.wait(),
                    timeout=_HEARTBEAT_INTERVAL_SECONDS,
                )
                break
            except TimeoutError:
                pass
            if await ctx.http_request.is_disconnected():
                ctx.stopped.set()
                break

            current_time = asyncio.get_running_loop().time()
            time_since_data = current_time - ctx.last_data_time_ref[0]

            # Send heartbeat only if no data for at least 5 seconds
            # This prevents heartbeats from interfering with data streaming
            if time_since_data >= _HEARTBEAT_IDLE_THRESHOLD_SECONDS:
                logger.debug(
                    "Sending SSE heartbeat",
                    extra={
                        "article_id": ctx.article_id,
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
    """Forward data from original stream with priority."""
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
                            "article_id": ctx.article_id,
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
                "article_id": ctx.article_id,
            },
        )
        # Surface the failure to the client instead of
        # letting the stream end silently and look
        # like a normal (if short) completed summary.
        await data_queue.put(("error", "summary generation failed"))
    finally:
        ctx.stopped.set()
        await data_queue.put(None)  # Sentinel to stop


def _format_sse_item(item: tuple[str, str], article_id: str) -> str | None:
    item_type, content = item
    if item_type == "data":
        logger.debug(
            "Yielding data chunk to client",
            extra={
                "article_id": article_id,
                "chunk_size": len(content) if content else 0,
            },
        )
        return f"data: {json.dumps(content)}\n\n"
    if item_type == "error":
        logger.debug(
            "Yielding SSE error event",
            extra={
                "article_id": article_id,
                "error": content,
            },
        )
        return f"event: error\ndata: {json.dumps({'error': content})}\n\n"
    return None


async def _drain_heartbeat(
    heartbeat_queue: asyncio.Queue[tuple[str, str] | None],
    article_id: str,
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
            extra={"article_id": article_id},
        )
        return heartbeat_content
    return None


async def _stream_with_heartbeat(
    original_stream: AsyncGenerator[str],
    http_request: Request,
    article_id: str,
) -> AsyncIterator[str]:
    """Wrap the stream generator with heartbeat for SSE.

    This implementation prioritizes data chunks over heartbeats to ensure
    real-time streaming. Heartbeats are sent only when no data is available
    to prevent blocking data chunk processing.
    """
    data_queue: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
    heartbeat_queue: asyncio.Queue[tuple[str, str] | None] = asyncio.Queue()
    stopped = asyncio.Event()
    ctx = _SSEStreamContext(
        http_request=http_request,
        stopped=stopped,
        last_data_time_ref=[asyncio.get_running_loop().time()],
        article_id=article_id,
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
                    # Generator is done, so end the body now.
                    # The heartbeat task is torn down in the
                    # finally block; waiting for its sentinel
                    # here would hold the connection open for
                    # up to a full heartbeat interval, and the
                    # consumer only persists the summary once
                    # it has read EOF.
                    break
                formatted = _format_sse_item(item, article_id)
                if formatted is not None:
                    yield formatted
                if await http_request.is_disconnected():
                    stopped.set()
                    break
            except TimeoutError:
                hb = await _drain_heartbeat(heartbeat_queue, article_id)
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


def _create_streaming_response(
    summarize_usecase: SummarizeUsecase,
    request: SummarizeRequest,
    http_request: Request,
    incoming_content_length: int,
) -> StreamingResponse:
    logger.info(
        "Received streaming summarize request",
        extra={
            "article_id": request.article_id,
            "content_length": incoming_content_length,
        },
    )
    try:
        original_stream = summarize_usecase.generate_summary_stream(
            article_id=request.article_id,
            content=request.content,
            priority=request.priority,
        )
        logger.info(
            "Stream generator created, creating StreamingResponse with heartbeat",
            extra={"article_id": request.article_id},
        )
        response = StreamingResponse(
            _stream_with_heartbeat(original_stream, http_request, request.article_id),
            media_type="text/event-stream; charset=utf-8",
        )
        response.headers["Cache-Control"] = "no-cache"
        response.headers["Connection"] = "keep-alive"
        response.headers["X-Accel-Buffering"] = "no"  # Disable nginx buffering

        logger.info(
            "StreamingResponse created successfully with SSE format",
            extra={
                "article_id": request.article_id,
                "media_type": "text/event-stream",
            },
        )
    except Exception as stream_exc:
        logger.exception(
            "Failed to create streaming response",
            extra={
                "article_id": request.article_id,
                "error": str(stream_exc),
                "error_type": type(stream_exc).__name__,
            },
        )
        raise
    else:
        return response


async def _handle_summarize_request(
    summarize_usecase: SummarizeUsecase,
    request: SummarizeRequest,
    http_request: Request,
) -> SummarizeResponse | StreamingResponse | JSONResponse:
    # Set ADR 98 business context for logging
    set_article_id(request.article_id)
    set_ai_pipeline("summarization")
    set_processing_stage("handler")

    # Zero Trust: Log incoming request details
    incoming_content_length = len(request.content) if request.content else 0
    logger.info(
        "Received summarize request",
        extra={"incoming_content_length": incoming_content_length},
    )

    _validate_summarize_content(request)

    try:
        if request.stream:
            return _create_streaming_response(
                summarize_usecase, request, http_request, incoming_content_length
            )

        summary, metadata = await summarize_usecase.generate_summary(
            article_id=request.article_id,
            content=request.content,
            priority=request.priority,
        )

        return SummarizeResponse(
            success=True,
            article_id=request.article_id,
            summary=summary,
            model=metadata.model,
            prompt_tokens=metadata.prompt_tokens,
            completion_tokens=metadata.completion_tokens,
            total_duration_ms=metadata.total_duration_ms,
        )

    except QueueFullError as exc:
        logger.warning(
            "Queue full, returning 429",
            extra={"article_id": request.article_id, "error": str(exc)},
        )
        return JSONResponse(
            status_code=429,
            content={"error": "queue full"},
            headers={"Retry-After": "30"},
        )

    except ValueError as exc:
        logger.warning("Invalid summarize request", extra={"error": str(exc)})
        raise HTTPException(status_code=400, detail="Invalid request") from exc

    except PreemptedException as exc:
        logger.warning(
            "Summary generation preempted by higher-priority request",
            extra={"article_id": request.article_id, "error": str(exc)},
        )
        raise HTTPException(status_code=502, detail="Upstream service error") from exc

    except RuntimeError as exc:
        error_detail = str(exc)
        if "empty/whitespace summary" in error_detail:
            logger.warning(
                "Article content caused model degeneration (empty output)",
                extra={
                    "article_id": request.article_id,
                    "error": error_detail,
                },
            )
            raise HTTPException(
                status_code=422, detail="Content not processable"
            ) from exc
        logger.exception(
            "Failed to generate summary",
            extra={
                "error": error_detail,
                "article_id": request.article_id,
                "error_type": type(exc).__name__,
                "content_length": len(request.content) if request.content else 0,
            },
        )
        raise HTTPException(status_code=502, detail="Upstream service error") from exc

    except Exception as exc:
        logger.exception(
            "Unexpected error while generating summary",
            extra={"article_id": request.article_id},
        )
        raise HTTPException(status_code=500, detail="Internal server error") from exc

    finally:
        # Clear business context after request processing
        clear_context()


def create_summarize_router(summarize_usecase: SummarizeUsecase) -> APIRouter:
    """
    Create summarize router with dependency injection.

    Args:
        summarize_usecase: Summarize usecase instance

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.post("/api/v1/summarize", response_model=SummarizeResponse)
    async def summarize_endpoint(
        request: SummarizeRequest, http_request: Request
    ) -> SummarizeResponse | StreamingResponse | JSONResponse:
        """
        Generate a Japanese summary using LLM.

        Args:
            request: Summarize request with article_id and content

        Returns:
            SummarizeResponse with summary and metadata

        Raises:
            HTTPException: 400 for invalid request, 502 for LLM errors, 500 for unexpected errors
        """
        return await _handle_summarize_request(summarize_usecase, request, http_request)

    return router
