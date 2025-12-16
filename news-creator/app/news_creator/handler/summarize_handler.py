"""Summarize handler - REST endpoint for article summarization."""

import logging
from fastapi import APIRouter, HTTPException

from news_creator.domain.models import SummarizeRequest, SummarizeResponse
from news_creator.usecase.summarize_usecase import SummarizeUsecase

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
    async def summarize_endpoint(request: SummarizeRequest) -> SummarizeResponse:
        """
        Generate a Japanese summary using LLM.

        Args:
            request: Summarize request with article_id and content

        Returns:
            SummarizeResponse with summary and metadata

        Raises:
            HTTPException: 400 for invalid request, 502 for LLM errors, 500 for unexpected errors
        """
        # Zero Trust: Log incoming request details
        incoming_content_length = len(request.content) if request.content else 0
        logger.info(
            "Received summarize request",
            extra={
                "article_id": request.article_id,
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
            summary, metadata = await summarize_usecase.generate_summary(
                article_id=request.article_id,
                content=request.content,
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

        except ValueError as exc:
            logger.warning("Invalid summarize request", extra={"error": str(exc)})
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        except RuntimeError as exc:
            error_detail = str(exc)
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

    return router
