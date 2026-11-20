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
        await db.health_check()
    except Exception:  # noqa: BLE001 — health endpoint must degrade rather than 500 on any DB check error
        checks["db"] = "unavailable"
        status = "degraded"
        logger.warning("Health check: DB unavailable")

    return {
        "status": status,
        "service": "recap-evaluator",
        "version": "0.1.0",
        "checks": checks,
    }
