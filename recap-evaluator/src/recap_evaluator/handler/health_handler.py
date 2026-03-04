"""Health check handler."""

import structlog
from fastapi import APIRouter, Request

logger = structlog.get_logger()

router = APIRouter()


@router.get("/health")
async def health_check(request: Request) -> dict:
    checks: dict[str, str] = {"db": "ok"}
    status = "healthy"

    try:
        db = request.app.state.db
        pool = db._pool
        await pool.fetchval("SELECT 1")
    except Exception:
        checks["db"] = "unavailable"
        status = "degraded"
        logger.warning("Health check: DB unavailable")

    return {
        "status": status,
        "service": "recap-evaluator",
        "version": "0.1.0",
        "checks": checks,
    }
