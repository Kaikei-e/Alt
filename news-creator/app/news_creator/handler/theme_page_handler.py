"""Theme page handler - endpoint for theme page generation."""

from __future__ import annotations

import logging
from typing import Any

from fastapi import APIRouter, HTTPException
from fastapi.responses import JSONResponse

from news_creator.domain.errors import (
    CardGenerationInvariantError,
    PreemptedException,
    QueueFullError,
)
from news_creator.domain.models import (
    CardGenerate422Response,
    CardGenerationRejectedError,
    ThemeGenerateRequest,
    ThemeGenerateResponse,
)
from news_creator.usecase.theme_page_usecase import ThemePageUsecase
from news_creator.utils.context_logger import (
    clear_context,
    set_ai_pipeline,
    set_job_id,
    set_processing_stage,
)

logger = logging.getLogger(__name__)


def create_theme_page_router(usecase: ThemePageUsecase) -> APIRouter:
    """Create theme page router with dependency injection."""
    router = APIRouter()

    @router.post(
        "/v1/themes/generate",
        response_model=ThemeGenerateResponse,
        responses={
            422: {
                "model": CardGenerate422Response,
                "description": "Theme page generation rejected",
            },
            429: {
                "description": "Queue full, retry after backoff",
            },
        },
    )
    async def theme_page_endpoint(
        request: ThemeGenerateRequest,
    ) -> ThemeGenerateResponse | JSONResponse:
        """Generate a Japanese theme page from numbered source items."""
        set_job_id(str(request.job_id))
        set_ai_pipeline("theme-page")
        set_processing_stage("handler")

        try:
            return await usecase.generate_theme_page(request)

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
            extra: dict[str, Any] = {
                "reason": exc.reason,
                "attempts": exc.attempts,
                "raw_text_len": len(exc.raw_text),
                "job_id": str(request.job_id),
                "theme_id": str(request.theme_id),
            }
            if exc.detail is not None:
                extra["detail"] = exc.detail

            logger.warning(
                "Theme page generation rejected",
                extra=extra,
            )
            return JSONResponse(
                status_code=422,
                content=exc.to_response_dict(),
                headers={"X-Card-Rejection": "1"},
            )

        except ValueError as exc:
            logger.warning(
                "Invalid theme page request",
                extra={"error": str(exc), "job_id": str(request.job_id)},
            )
            raise HTTPException(status_code=400, detail="Invalid request") from exc

        except PreemptedException as exc:
            logger.warning(
                "Theme page generation preempted by higher-priority request",
                extra={"error": str(exc), "job_id": str(request.job_id)},
            )
            raise HTTPException(
                status_code=502, detail="Upstream service error"
            ) from exc

        except CardGenerationInvariantError as exc:
            logger.error(
                "Theme page generation failed with invariant violation",
                extra={"error": str(exc), "job_id": str(request.job_id)},
                exc_info=True,
            )
            raise HTTPException(
                status_code=500, detail="Internal server error"
            ) from exc

        except RuntimeError as exc:
            logger.error(
                "Theme page generation failed with runtime error",
                extra={"error": str(exc), "job_id": str(request.job_id)},
                exc_info=True,
            )
            raise HTTPException(
                status_code=502, detail="Upstream service error"
            ) from exc

        except Exception as exc:
            logger.exception(
                "Unexpected error while generating theme page",
                extra={"job_id": str(request.job_id)},
            )
            raise HTTPException(
                status_code=500, detail="Internal server error"
            ) from exc

        finally:
            clear_context()

    return router
