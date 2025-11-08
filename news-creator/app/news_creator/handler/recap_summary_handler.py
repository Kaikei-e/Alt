"""Recap summary handler - endpoint for recap-worker structured summaries."""

import logging
from fastapi import APIRouter, HTTPException

from news_creator.domain.models import RecapSummaryRequest, RecapSummaryResponse
from news_creator.usecase.recap_summary_usecase import RecapSummaryUsecase

logger = logging.getLogger(__name__)

router = APIRouter()


def create_recap_summary_router(usecase: RecapSummaryUsecase) -> APIRouter:
    """
    Create recap summary router with dependency injection.

    Args:
        usecase: Recap summary usecase instance

    Returns:
        Configured APIRouter
    """

    @router.post("/v1/summary/generate", response_model=RecapSummaryResponse)
    async def recap_summary_endpoint(request: RecapSummaryRequest) -> RecapSummaryResponse:
        """
        Generate a Japanese recap summary for clustered evidence.

        Args:
            request: Clustering evidence payload

        Returns:
            Structured recap summary response
        """

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

    return router

