"""GET /v1/voices endpoint."""

from __future__ import annotations

from fastapi import APIRouter, Depends, Request
from starlette.responses import JSONResponse

from ...infra.auth import verify_service_token

router = APIRouter()


@router.get("/v1/voices", dependencies=[Depends(verify_service_token)])
async def list_voices(request: Request) -> JSONResponse:
    """Return available Japanese voices."""
    pipeline = request.app.state.pipeline
    return JSONResponse(content={"voices": pipeline.voices})
