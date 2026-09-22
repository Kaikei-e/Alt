"""Recap card handler - endpoint for topic card generation."""

from __future__ import annotations

import logging
from fastapi import APIRouter, HTTPException
from fastapi.responses import JSONResponse

from news_creator.domain.models import (
    CardGenerate422Response,
    CardGenerateRequest,
    CardGenerateResponse,
    CardGenerationRejectedError,
)
from news_creator.gateway.hybrid_priority_semaphore import (
    PreemptedException,
    QueueFullError,
)
from news_creator.usecase.recap_card_usecase import RecapCardUsecase
from news_creator.utils.context_logger import (
    clear_context,
    set_ai_pipeline,
    set_job_id,
    set_processing_stage,
)

logger = logging.getLogger(__name__)


def create_recap_card_router(usecase: RecapCardUsecase) -> APIRouter:
    """
    Create recap card router with dependency injection.

    Args:
        usecase: Recap card usecase instance

    Returns:
        Configured APIRouter
    """
    router = APIRouter()

    @router.post(
        "/v1/cards/generate",
        response_model=CardGenerateResponse,
        responses={
            422: {
                "model": CardGenerate422Response,
                "description": "Card generation rejected (caller drops card)",
            },
            429: {
                "description": "Queue full, retry after backoff",
            },
        },
    )
    async def recap_card_endpoint(
        request: CardGenerateRequest,
    ) -> CardGenerateResponse | JSONResponse:
        """
        Generate a Japanese summary card from candidate items.

        Args:
            request: Candidate items payload

        Returns:
            CardGenerateResponse on 200 OK
            CardGenerate422Response on 422 Unprocessable Entity
        """
        set_job_id(str(request.job_id))
        set_ai_pipeline("recap-card")
        set_processing_stage("handler")

        try:
            return await usecase.generate_card(request)

        except QueueFullError as exc:
            logger.warning(
                "Queue full, returning 429",
                extra={"error": str(exc), "job_id": str(request.job_id)},
            )
            return JSONResponse(
                status_code=429,
                content={"error": "queue full"},
                headers={"Retry-After": "30"},
            )

        except CardGenerationRejectedError as exc:
            logger.warning(
                "Card generation rejected",
                extra={
                    "reason": exc.reason,
                    "attempts": exc.attempts,
                    "job_id": str(request.job_id),
                    "candidate_id": str(request.candidate_id),
                },
            )
            return JSONResponse(
                status_code=422,
                content=exc.to_response_dict(),
                headers={"X-Card-Rejection": "1"},
            )

        except ValueError as exc:
            logger.warning(
                "Invalid recap card request",
                extra={"error": str(exc), "job_id": str(request.job_id)},
            )
            raise HTTPException(status_code=400, detail="Invalid request") from exc

        except PreemptedException as exc:
            logger.warning(
                "Recap card generation preempted by higher-priority request",
                extra={"error": str(exc), "job_id": str(request.job_id)},
            )
            raise HTTPException(
                status_code=502, detail="Upstream service error"
            ) from exc

        except RuntimeError as exc:
            logger.error(
                "Recap card generation failed with runtime error",
                extra={"error": str(exc), "job_id": str(request.job_id)},
                exc_info=True,
            )
            raise HTTPException(
                status_code=500, detail="Internal server error"
            ) from exc

        finally:
            clear_context()

    return router
