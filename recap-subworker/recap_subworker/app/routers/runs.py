"""Run management API endpoints."""

from __future__ import annotations

from uuid import UUID

from fastapi import APIRouter, Depends, Header, HTTPException, status

from ...db.dao import RunRecord
from ...domain.models import ClusterJobPayload, ClusterJobResponse
from ...infra.config import Settings
from ...services.run_manager import (
    ConcurrentRunError,
    IdempotencyMismatchError,
    RunManager,
    RunSubmission,
)
from ..deps import get_run_manager_dep, get_settings_dep


router = APIRouter(prefix="/v1", tags=["runs"])


def _record_to_response(record: RunRecord) -> ClusterJobResponse:
    payload = record.response_payload or {}
    if payload.get("run_id"):
        return ClusterJobResponse(**payload)
    return ClusterJobResponse(
        run_id=record.run_id,
        job_id=str(record.job_id),
        genre=record.genre,
        status=record.status,
        cluster_count=record.cluster_count,
        clusters=[],
        diagnostics={"error_message": record.error_message} if record.error_message else {},
    )


@router.post("/runs", response_model=ClusterJobResponse, status_code=status.HTTP_202_ACCEPTED)
async def create_run(
    payload: ClusterJobPayload,
    job_id_header: str = Header(..., alias="X-Alt-Job-Id"),
    genre: str = Header(..., alias="X-Alt-Genre"),
    idempotency_key: str | None = Header(default=None, alias="Idempotency-Key"),
    settings: Settings = Depends(get_settings_dep),
    manager: RunManager = Depends(get_run_manager_dep),
) -> ClusterJobResponse:
    try:
        job_id = UUID(job_id_header)
    except ValueError as exc:  # pragma: no cover - defensive
        raise HTTPException(status_code=400, detail="X-Alt-Job-Id must be a valid UUID") from exc

    if len(payload.documents) > settings.max_docs_per_genre:
        raise HTTPException(status_code=422, detail="document limit exceeded")

    submission = RunSubmission(
        job_id=job_id,
        genre=genre,
        payload=payload,
        idempotency_key=idempotency_key,
    )
    try:
        record = await manager.create_run(submission)
    except IdempotencyMismatchError as exc:
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    except ConcurrentRunError as exc:
        raise HTTPException(status_code=409, detail=str(exc)) from exc

    return _record_to_response(record)


@router.get("/runs/{run_id}", response_model=ClusterJobResponse)
async def get_run(
    run_id: int,
    manager: RunManager = Depends(get_run_manager_dep),
) -> ClusterJobResponse:
    record = await manager.get_run(run_id)
    if not record:
        raise HTTPException(status_code=404, detail="run not found")
    return _record_to_response(record)
