"""Plan Query handler - REST endpoint for Augur structured query planning."""

import logging
from fastapi import APIRouter, HTTPException

from news_creator.domain.models import PlanQueryRequest, PlanQueryResponse
from news_creator.usecase.plan_query_usecase import PlanQueryUsecase
from news_creator.utils.context_logger import (
    set_ai_pipeline,
    set_processing_stage,
    clear_context,
)

logger = logging.getLogger(__name__)


def create_plan_query_router(plan_query_usecase: PlanQueryUsecase) -> APIRouter:
    """Create plan query router with dependency injection."""
    router = APIRouter()

    @router.post("/api/v1/plan-query", response_model=PlanQueryResponse)
    async def plan_query_endpoint(request: PlanQueryRequest) -> PlanQueryResponse:
        """
        Plan retrieval strategy for an Augur query.

        Produces a structured QueryPlan with resolved query, search queries,
        intent classification, and retrieval policy using LLM structured output.
        """
        set_ai_pipeline("query-planning")
        set_processing_stage("handler")

        try:
            logger.info(
                "Received plan-query request",
                extra={
                    "query_length": len(request.query),
                    "has_history": request.conversation_history is not None,
                    "article_scoped": request.article_id is not None,
                },
            )

            response = await plan_query_usecase.plan_query(request)
            return response

        except ValueError as exc:
            logger.warning("Invalid plan-query request", extra={"error": str(exc)})
            raise HTTPException(status_code=400, detail=str(exc)) from exc

        except RuntimeError as exc:
            logger.error(
                "Failed to plan query", extra={"error": str(exc)}, exc_info=True
            )
            raise HTTPException(status_code=502, detail=str(exc)) from exc

        except Exception as exc:
            logger.exception("Unexpected error in plan-query")
            raise HTTPException(
                status_code=500, detail="Internal server error"
            ) from exc

        finally:
            clear_context()

    return router
