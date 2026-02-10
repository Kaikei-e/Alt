"""Summarize handler - REST endpoint for article summarization."""

import asyncio
import json
import logging
from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from news_creator.domain.models import SummarizeRequest, SummarizeResponse
from news_creator.gateway.hybrid_priority_semaphore import QueueFullError
from news_creator.usecase.summarize_usecase import SummarizeUsecase
from news_creator.utils.context_logger import (
    set_article_id,
    set_ai_pipeline,
    set_processing_stage,
    clear_context,
)

logger = logging.getLogger(__name__)

router = APIRouter()


def create_summarize_router(summarize_usecase: SummarizeUsecase) -> APIRouter:
    """
    Create summarize router with dependency injection.

    Args:
        summarize_usecase: Summarize usecase instance

    Returns:
        Configured APIRouter
    """
    @router.post("/api/v1/summarize", response_model=SummarizeResponse)
    async def summarize_endpoint(request: SummarizeRequest, http_request: Request) -> SummarizeResponse:
        """
        Generate a Japanese summary using LLM.

        Args:
            request: Summarize request with article_id and content

        Returns:
            SummarizeResponse with summary and metadata

        Raises:
            HTTPException: 400 for invalid request, 502 for LLM errors, 500 for unexpected errors
        """
        # Set ADR 98 business context for logging
        set_article_id(request.article_id)
        set_ai_pipeline("summarization")
        set_processing_stage("handler")

        # Zero Trust: Log incoming request details
        incoming_content_length = len(request.content) if request.content else 0
        logger.info(
            "Received summarize request",
            extra={
                "incoming_content_length": incoming_content_length,
            }
        )

        # Early check: reject requests with content shorter than 100 characters
        # This prevents unnecessary LLM calls for short content
        min_content_length = 100
        if not request.content or len(request.content.strip()) < min_content_length:
            error_msg = (
                f"Content is too short for summarization. "
                f"Content length: {len(request.content) if request.content else 0}, "
                f"Minimum required: {min_content_length} characters."
            )
            logger.warning(
                "Rejecting summarize request: content too short",
                extra={
                    "article_id": request.article_id,
                    "content_length": len(request.content) if request.content else 0,
                    "min_required": min_content_length,
                }
            )
            raise HTTPException(status_code=400, detail=error_msg)



        try:
            if request.stream:
                # Streaming response logic with SSE format and heartbeat
                logger.info(
                    "Received streaming summarize request",
                    extra={
                        "article_id": request.article_id,
                        "content_length": incoming_content_length,
                    }
                )

                try:
                    # Get the original stream generator
                    original_stream = summarize_usecase.generate_summary_stream(
                        article_id=request.article_id,
                        content=request.content,
                        priority=request.priority,
                    )
                    logger.info(
                        "Stream generator created, creating StreamingResponse with heartbeat",
                        extra={
                            "article_id": request.article_id,
                        }
                    )

                    # Create a wrapper that adds heartbeat support
                    async def stream_with_heartbeat():
                        """Wrap the stream generator with heartbeat for SSE.

                        This implementation prioritizes data chunks over heartbeats to ensure
                        real-time streaming. Heartbeats are sent only when no data is available
                        to prevent blocking data chunk processing.
                        """
                        heartbeat_interval = 10  # seconds - reduced from 15 to 10 for better responsiveness
                        data_queue = asyncio.Queue()
                        heartbeat_queue = asyncio.Queue()
                        stopped = asyncio.Event()
                        last_data_time = asyncio.get_event_loop().time()

                        async def heartbeat_task():
                            """Send heartbeat comments periodically, but only when no data is flowing."""
                            try:
                                while not stopped.is_set():
                                    await asyncio.sleep(heartbeat_interval)
                                    if await http_request.is_disconnected():
                                        stopped.set()
                                        break

                                    # Only send heartbeat if no data was received recently (within last 5 seconds)
                                    current_time = asyncio.get_event_loop().time()
                                    time_since_data = current_time - last_data_time

                                    # Send heartbeat only if no data for at least 5 seconds
                                    # This prevents heartbeats from interfering with data streaming
                                    if time_since_data >= 5.0:
                                        logger.debug(
                                            "Sending SSE heartbeat",
                                            extra={
                                                "article_id": request.article_id,
                                                "time_since_data": f"{time_since_data:.2f}s"
                                            }
                                        )
                                        await heartbeat_queue.put(("heartbeat", ":\n\n"))
                            except asyncio.CancelledError:
                                pass
                            finally:
                                stopped.set()
                                await heartbeat_queue.put(None)  # Sentinel to stop

                        async def stream_task():
                            """Forward data from original stream with priority."""
                            nonlocal last_data_time
                            chunk_count = 0
                            try:
                                async for chunk in original_stream:
                                    if await http_request.is_disconnected():
                                        stopped.set()
                                        break
                                    chunk_count += 1
                                    # Log first few chunks for debugging incremental rendering
                                    if chunk_count <= 3:
                                        logger.debug(
                                            "Streaming chunk to client",
                                            extra={
                                                "article_id": request.article_id,
                                                "chunk_count": chunk_count,
                                                "chunk_size": len(chunk) if chunk else 0,
                                                "chunk_preview": chunk[:50] if chunk and len(chunk) > 50 else chunk
                                            }
                                        )
                                    # Update last data time to prevent unnecessary heartbeats
                                    last_data_time = asyncio.get_event_loop().time()
                                    await data_queue.put(("data", chunk))
                            except Exception as e:
                                logger.error(
                                    "Error in stream task",
                                    extra={"error": str(e), "article_id": request.article_id},
                                    exc_info=True
                                )
                            finally:
                                stopped.set()
                                await data_queue.put(None)  # Sentinel to stop

                        # Start both tasks
                        heartbeat = asyncio.create_task(heartbeat_task())
                        stream = asyncio.create_task(stream_task())

                        try:
                            # Process items from queues with priority for data chunks
                            data_sentinel_received = False
                            heartbeat_sentinel_received = False

                            while True:
                                # Prioritize data queue - check it first with timeout
                                try:
                                    # Use timeout to allow checking heartbeat queue periodically
                                    item = await asyncio.wait_for(data_queue.get(), timeout=0.1)
                                    if item is None:
                                        data_sentinel_received = True
                                        if data_sentinel_received and heartbeat_sentinel_received:
                                            break
                                        continue

                                    item_type, content = item
                                    if item_type == "data":
                                        # Log data chunk yield for debugging incremental rendering
                                        logger.debug(
                                            "Yielding data chunk to client",
                                            extra={
                                                "article_id": request.article_id,
                                                "chunk_size": len(content) if content else 0
                                            }
                                        )
                                        # Standard SSE format: data: <json_encoded_data>\n\n
                                        yield f"data: {json.dumps(content)}\n\n"

                                    if await http_request.is_disconnected():
                                        stopped.set()
                                        break
                                except asyncio.TimeoutError:
                                    # No data available, check heartbeat queue
                                    try:
                                        heartbeat_item = heartbeat_queue.get_nowait()
                                        if heartbeat_item is None:
                                            heartbeat_sentinel_received = True
                                            if data_sentinel_received and heartbeat_sentinel_received:
                                                break
                                            continue

                                        heartbeat_type, heartbeat_content = heartbeat_item
                                        if heartbeat_type == "heartbeat":
                                            logger.debug(
                                                "Yielding SSE heartbeat",
                                                extra={"article_id": request.article_id}
                                            )
                                            yield heartbeat_content

                                        if await http_request.is_disconnected():
                                            stopped.set()
                                            break
                                    except asyncio.QueueEmpty:
                                        # No heartbeat available either, continue waiting for data
                                        continue
                        finally:
                            # Cleanup
                            stopped.set()
                            heartbeat.cancel()
                            stream.cancel()
                            try:
                                await asyncio.gather(heartbeat, stream, return_exceptions=True)
                            except Exception:
                                pass

                    response = StreamingResponse(
                        stream_with_heartbeat(),
                        media_type="text/event-stream; charset=utf-8"
                    )
                    # Set headers for SSE streaming
                    response.headers["Cache-Control"] = "no-cache"
                    response.headers["Connection"] = "keep-alive"
                    response.headers["X-Accel-Buffering"] = "no"  # Disable nginx buffering

                    logger.info(
                        "StreamingResponse created successfully with SSE format",
                        extra={
                            "article_id": request.article_id,
                            "media_type": "text/event-stream",
                        }
                    )
                    return response
                except Exception as stream_exc:
                    logger.error(
                        "Failed to create streaming response",
                        extra={
                            "article_id": request.article_id,
                            "error": str(stream_exc),
                            "error_type": type(stream_exc).__name__,
                        },
                        exc_info=True,
                    )
                    raise

            summary, metadata = await summarize_usecase.generate_summary(
                article_id=request.article_id,
                content=request.content,
                priority=request.priority,
            )

            return SummarizeResponse(
                success=True,
                article_id=request.article_id,
                summary=summary,
                model=metadata.get("model", "unknown"),
                prompt_tokens=metadata.get("prompt_tokens"),
                completion_tokens=metadata.get("completion_tokens"),
                total_duration_ms=metadata.get("total_duration_ms"),
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
            raise HTTPException(status_code=400, detail=str(exc)) from exc

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
                    status_code=422,
                    detail=f"Content not processable: {error_detail}"
                ) from exc
            logger.error(
                "Failed to generate summary",
                extra={
                    "error": error_detail,
                    "article_id": request.article_id,
                    "error_type": type(exc).__name__,
                    "content_length": len(request.content) if request.content else 0,
                },
                exc_info=True,  # Include full traceback for debugging
            )
            raise HTTPException(status_code=502, detail=error_detail) from exc

        except Exception as exc:
            logger.exception(
                "Unexpected error while generating summary",
                extra={"article_id": request.article_id},
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

        finally:
            # Clear business context after request processing
            clear_context()

    return router
