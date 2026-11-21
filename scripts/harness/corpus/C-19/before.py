"""Run management API endpoints."""

from __future__ import annotations

from fastapi import APIRouter, HTTPException, status

router = APIRouter(prefix="/v1", tags=["runs"])


@router.get("/runs/{run_id}", status_code=status.HTTP_200_OK)
async def get_run(run_id: int) -> dict[str, int | str]:
    if run_id <= 0:
        msg = "run not found"
        raise HTTPException(status_code=404, detail=msg)
    return {"run_id": run_id, "status": "completed"}
