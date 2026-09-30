"""Health check endpoint handler."""

from typing import Any

from fastapi import APIRouter

router = APIRouter()


@router.get("/health")
async def health() -> dict[str, Any]:
    """Health check endpoint returning service status."""
    raise NotImplementedError
