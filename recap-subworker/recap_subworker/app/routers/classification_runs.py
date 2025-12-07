"""Classification run endpoints for async job pattern."""

from __future__ import annotations

from typing import Optional
from uuid import UUID

import structlog
from fastapi import APIRouter, Depends, Header, HTTPException, status
from sqlalchemy.exc import DatabaseError, IntegrityError, OperationalError

from ...domain.models import (
    ClassificationJobPayload,
    ClassificationJobResponse,
    ClassificationResult,
)
from ...services.run_manager import (
    ClassificationRunSubmission,
    ConcurrentRunError,
    IdempotencyMismatchError,
    RunManager,
)
from ..deps import get_run_manager_dep

router = APIRouter()
LOGGER = structlog.get_logger(__name__)


def _record_to_classification_response(record) -> ClassificationJobResponse:
    """Convert RunRecord to ClassificationJobResponse."""
    results = None
    if record.response_payload and "results" in record.response_payload:
        results = [
            ClassificationResult(**r) for r in record.response_payload["results"]
        ]

    return ClassificationJobResponse(
        run_id=record.run_id,
        job_id=str(record.job_id),
        status=record.status,
        result_count=record.cluster_count,  # Reuse cluster_count for result_count
        results=results,
        error_message=record.error_message,
    )


@router.post(
    "/v1/classify-runs",
    response_model=ClassificationJobResponse,
    status_code=status.HTTP_202_ACCEPTED,
)
async def create_classification_run(
    payload: ClassificationJobPayload,
    job_id_header: str = Header(..., alias="X-Alt-Job-Id"),
    idempotency_key: Optional[str] = Header(default=None, alias="Idempotency-Key"),
    manager: RunManager = Depends(get_run_manager_dep),
) -> ClassificationJobResponse:
    """Create a new classification run (async job pattern)."""
    try:
        job_id = UUID(job_id_header)
    except ValueError as exc:
        raise HTTPException(
            status_code=400, detail="X-Alt-Job-Id must be a valid UUID"
        ) from exc

    submission = ClassificationRunSubmission(
        job_id=job_id,
        payload=payload,
        idempotency_key=idempotency_key,
    )
    try:
        record = await manager.create_classification_run(submission)
    except IdempotencyMismatchError as exc:
        LOGGER.warning(
            "classification.run.idempotency_mismatch",
            job_id=str(job_id),
            error=str(exc),
        )
        raise HTTPException(status_code=422, detail=str(exc)) from exc
    except ConcurrentRunError as exc:
        LOGGER.warning(
            "classification.run.concurrent",
            job_id=str(job_id),
            error=str(exc),
        )
        raise HTTPException(status_code=409, detail=str(exc)) from exc
    except (OperationalError, IntegrityError, DatabaseError) as exc:
        LOGGER.error(
            "classification.run.database_error",
            job_id=str(job_id),
            error_type=type(exc).__name__,
            error=str(exc),
            exc_info=True,
        )
        raise HTTPException(
            status_code=503,
            detail="Database service temporarily unavailable",
        ) from exc
    except FileNotFoundError as exc:
        LOGGER.error(
            "classification.run.model_not_found",
            job_id=str(job_id),
            error=str(exc),
            exc_info=True,
        )
        raise HTTPException(
            status_code=503,
            detail="Classification service temporarily unavailable",
        ) from exc
    except Exception as exc:
        LOGGER.exception(
            "classification.run.unexpected_error",
            job_id=str(job_id),
            error_type=type(exc).__name__,
            error=str(exc),
        )
        raise HTTPException(
            status_code=500,
            detail="Internal server error",
        ) from exc

    return _record_to_classification_response(record)


@router.get("/v1/classify-runs/{run_id}", response_model=ClassificationJobResponse)
async def get_classification_run(
    run_id: int,
    manager: RunManager = Depends(get_run_manager_dep),
) -> ClassificationJobResponse:
    """Get classification run status and results."""
    record = await manager.get_run(run_id)
    if not record:
        raise HTTPException(status_code=404, detail="run not found")

    return _record_to_classification_response(record)

