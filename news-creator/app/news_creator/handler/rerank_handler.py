"""Rerank handler - REST endpoint for cross-encoder re-ranking.

This endpoint is called by rag-orchestrator to re-rank retrieval candidates
using a cross-encoder model for improved relevance scoring.
"""

import logging
from fastapi import APIRouter, HTTPException

from news_creator.domain.models import (
    RerankRequest,
    RerankResponse,
    RerankResultItem,
)
from news_creator.usecase.rerank_usecase import RerankUsecase

logger = logging.getLogger(__name__)


def create_rerank_router(rerank_usecase: RerankUsecase) -> APIRouter:
    """
    Create rerank router with dependency injection.

    Args:
        rerank_usecase: Rerank usecase instance

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.post("/v1/rerank", response_model=RerankResponse)
    async def rerank_endpoint(request: RerankRequest) -> RerankResponse:
        """
        Re-rank candidates using cross-encoder scoring.

        This endpoint is called by rag-orchestrator to improve retrieval
        quality by scoring query-candidate pairs with a cross-encoder model.

        Research basis:
        - Pinecone: +15-30% NDCG@10 improvement
        - ZeroEntropy: -35% LLM hallucinations

        Args:
            request: RerankRequest with query and candidates

        Returns:
            RerankResponse with scored and sorted results

        Raises:
            HTTPException: 400 for invalid request, 502 for model errors, 500 for unexpected errors
        """
        logger.info(
            "Received rerank request",
            extra={
                "query_length": len(request.query) if request.query else 0,
                "candidate_count": len(request.candidates),
                "model": request.model,
                "top_k": request.top_k,
            }
        )

        try:
            # Use request model if specified, otherwise usecase default
            if request.model:
                usecase = RerankUsecase(model_name=request.model)
            else:
                usecase = rerank_usecase

            results, model, processing_time_ms = await usecase.rerank(
                query=request.query,
                candidates=request.candidates,
                top_k=request.top_k,
            )

            return RerankResponse(
                results=[
                    RerankResultItem(index=idx, score=score)
                    for idx, score in results
                ],
                model=model,
                processing_time_ms=processing_time_ms,
            )

        except ValueError as exc:
            logger.warning(
                "Invalid rerank request",
                extra={"error": str(exc), "query": request.query[:100] if request.query else ""}
            )
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        except RuntimeError as exc:
            logger.error(
                "Failed to rerank candidates",
                extra={
                    "error": str(exc),
                    "query": request.query[:100] if request.query else "",
                    "candidate_count": len(request.candidates),
                },
                exc_info=True,
            )
            raise HTTPException(status_code=502, detail=str(exc)) from exc

        except Exception as exc:
            logger.exception(
                "Unexpected error while re-ranking",
                extra={"query": request.query[:100] if request.query else ""},
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

    return router
