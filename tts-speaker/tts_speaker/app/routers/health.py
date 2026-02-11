"""GET /health endpoint."""

from __future__ import annotations

from fastapi import APIRouter, Request
from starlette.responses import JSONResponse

router = APIRouter()


@router.get("/health")
async def health(request: Request) -> JSONResponse:
    """Return service health status. No authentication required."""
    pipeline = request.app.state.pipeline

    if not pipeline.is_ready:
        return JSONResponse(
            status_code=503,
            content={"status": "loading", "model": "kokoro-82m", "lang": "ja"},
        )

    return JSONResponse(
        content={"status": "ok", "model": "kokoro-82m", "lang": "ja"},
    )
