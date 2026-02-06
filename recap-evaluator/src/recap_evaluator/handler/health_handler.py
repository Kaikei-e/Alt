"""Health check handler."""

from fastapi import APIRouter

router = APIRouter()


@router.get("/health")
async def health_check() -> dict:
    return {
        "status": "healthy",
        "service": "recap-evaluator",
        "version": "0.1.0",
    }
