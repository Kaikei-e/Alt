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
            logger.error(
                "Failed to generate summary",
                extra={"error": str(exc), "article_id": request.article_id},
            )
            raise HTTPException(status_code=502, detail=str(exc)) from exc

        except Exception as exc:
            logger.exception(
                "Unexpected error while generating summary",
                extra={"article_id": request.article_id},
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

    return router
