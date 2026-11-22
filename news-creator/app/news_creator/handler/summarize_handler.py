"""Summarize handler - REST endpoint for article summarization."""

import logging

from fastapi import APIRouter, HTTPException, Request
from fastapi.responses import JSONResponse, StreamingResponse

from news_creator.domain.errors import PreemptedException, QueueFullError
from news_creator.domain.models import SummarizeRequest, SummarizeResponse
from news_creator.handler.sse_stream import create_sse_response
from news_creator.usecase.summarize_usecase import SummarizeUsecase
from news_creator.utils.context_logger import (
    clear_context,
    set_ai_pipeline,
    set_article_id,
    set_processing_stage,
)

logger = logging.getLogger(__name__)

_MIN_CONTENT_LENGTH = 100


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
        response = create_sse_response(
            original_stream,
            http_request,
            stream_id=request.article_id,
            error_message="summary generation failed",
        )

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
