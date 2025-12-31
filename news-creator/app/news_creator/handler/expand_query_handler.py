"""Expand Query handler - REST endpoint for RAG query expansion."""

import logging
from fastapi import APIRouter, HTTPException

from news_creator.domain.models import ExpandQueryRequest, ExpandQueryResponse
from news_creator.usecase.expand_query_usecase import ExpandQueryUsecase

logger = logging.getLogger(__name__)


def create_expand_query_router(expand_query_usecase: ExpandQueryUsecase) -> APIRouter:
    """
    Create expand query router with dependency injection.

    Args:
        expand_query_usecase: Expand query usecase instance

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.post("/api/v1/expand-query", response_model=ExpandQueryResponse)
    async def expand_query_endpoint(request: ExpandQueryRequest) -> ExpandQueryResponse:
        """
        Generate expanded search queries for RAG retrieval.

        This endpoint is called by rag-orchestrator to generate diverse
        query variations for improved vector search coverage.

        Args:
            request: ExpandQueryRequest with query and count parameters

        Returns:
            ExpandQueryResponse with expanded queries and metadata

        Raises:
            HTTPException: 400 for invalid request, 502 for LLM errors, 500 for unexpected errors
        """
        logger.info(
            "Received expand-query request",
            extra={
                "query_length": len(request.query) if request.query else 0,
                "japanese_count": request.japanese_count,
                "english_count": request.english_count,
            }
        )

        try:
            expanded_queries, model, processing_time_ms = await expand_query_usecase.expand_query(
                query=request.query,
                japanese_count=request.japanese_count,
                english_count=request.english_count,
            )

            return ExpandQueryResponse(
                expanded_queries=expanded_queries,
                original_query=request.query,
                model=model,
                processing_time_ms=processing_time_ms,
            )

        except ValueError as exc:
            logger.warning(
                "Invalid expand-query request",
                extra={"error": str(exc), "query": request.query[:100] if request.query else ""}
            )
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        except RuntimeError as exc:
            logger.error(
                "Failed to expand query",
                extra={
                    "error": str(exc),
                    "query": request.query[:100] if request.query else "",
                },
                exc_info=True,
            )
            raise HTTPException(status_code=502, detail=str(exc)) from exc

        except Exception as exc:
            logger.exception(
                "Unexpected error while expanding query",
                extra={"query": request.query[:100] if request.query else ""},
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

    return router
