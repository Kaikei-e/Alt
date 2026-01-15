"""Recap summary handler - endpoint for recap-worker structured summaries."""

import logging
from fastapi import APIRouter, HTTPException

from news_creator.domain.models import (
    BatchRecapSummaryRequest,
    BatchRecapSummaryResponse,
    RecapSummaryRequest,
    RecapSummaryResponse,
)
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase
from news_creator.utils.context_logger import (
    set_job_id,
    set_ai_pipeline,
    set_processing_stage,
    clear_context,
)

logger = logging.getLogger(__name__)


def create_recap_summary_router(usecase: RecapSummaryUsecase) -> APIRouter:
    """
    Create recap summary router with dependency injection.

    Args:
        usecase: Recap summary usecase instance

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.post("/v1/summary/generate", response_model=RecapSummaryResponse)
    async def recap_summary_endpoint(request: RecapSummaryRequest) -> RecapSummaryResponse:
        """
        Generate a Japanese recap summary for clustered evidence.

        Args:
            request: Clustering evidence payload

        Returns:
            Structured recap summary response
        """
        # Set ADR 98 business context for logging
        set_job_id(str(request.job_id))
        set_ai_pipeline("recap-summary")
        set_processing_stage("handler")

        try:
            return await usecase.generate_summary(request)

        except ValueError as exc:
            logger.warning(
                "Invalid recap summary request",
                extra={"error": str(exc), "job_id": str(request.job_id)},
            )
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        except RuntimeError as exc:
            logger.error(
                "Recap summary generation failed",
                extra={"error": str(exc), "job_id": str(request.job_id), "genre": request.genre},
            )
            raise HTTPException(status_code=502, detail=str(exc)) from exc

        except Exception as exc:
            logger.exception(
                "Unexpected error while generating recap summary",
                extra={"job_id": str(request.job_id)},
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

        finally:
            clear_context()

    @router.post("/v1/summary/generate/batch", response_model=BatchRecapSummaryResponse)
    async def batch_recap_summary_endpoint(
        request: BatchRecapSummaryRequest,
    ) -> BatchRecapSummaryResponse:
        """
        Generate multiple Japanese recap summaries in a single request.

        This endpoint reduces the "chatty microservices" anti-pattern by allowing
        multiple genres to be processed in a single HTTP request.

        Args:
            request: Batch request containing multiple individual recap requests

        Returns:
            Batch response with successful summaries and any errors
        """
        # Set ADR 98 business context for logging
        set_ai_pipeline("recap-summary-batch")
        set_processing_stage("handler")

        try:
            logger.info(
                "Received batch recap summary request",
                extra={"request_count": len(request.requests)},
            )

            return await usecase.generate_batch_summary(request)

        except Exception as exc:
            logger.exception(
                "Unexpected error while processing batch recap summary",
                extra={"request_count": len(request.requests)},
            )
            raise HTTPException(status_code=500, detail="Internal server error") from exc

        finally:
            clear_context()

    return router

