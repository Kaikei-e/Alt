"""Morning Letter HTTP handler."""

import logging

from fastapi import APIRouter

from news_creator.domain.models import MorningLetterRequest, MorningLetterResponse
from news_creator.usecase.morning_letter_usecase import MorningLetterUsecase

logger = logging.getLogger(__name__)


def create_morning_letter_router(usecase: MorningLetterUsecase) -> APIRouter:
    router = APIRouter(prefix="/v1/morning-letter", tags=["morning-letter"])

    @router.post("/generate", response_model=MorningLetterResponse)
    async def generate_morning_letter(request: MorningLetterRequest):
        try:
            return await usecase.generate_letter(request)
        except ValueError as e:
            logger.warning("Morning Letter validation failed", extra={"error": str(e)})
            from fastapi.responses import JSONResponse

            return JSONResponse(status_code=400, content={"error": "Invalid request"})
        except Exception as e:
            logger.error("Morning Letter generation failed", extra={"error": str(e)})
            from fastapi.responses import JSONResponse

            return JSONResponse(
                status_code=500, content={"error": "Internal server error"}
            )

    return router
