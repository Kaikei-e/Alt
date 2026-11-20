"""Classification run endpoints for async job pattern."""

from __future__ import annotations

from typing import Annotated, cast
from uuid import UUID

import structlog
from fastapi import APIRouter, Depends, Header, HTTPException, status
from sqlalchemy.exc import DatabaseError, IntegrityError, OperationalError

from ...db.dao import RunRecord
from ...domain.models import (
    ClassificationJobPayload,
    ClassificationJobResponse,
    ClassificationResult,
    RunStatusLiteral,
)
from ...services.run_manager import (
    ClassificationRunSubmission,
    ConcurrentRunError,
    IdempotencyMismatchError,
)
from ...usecase.submit_run import (
    GetClassificationRunUsecase,
    SubmitClassificationRunUsecase,
)
from ..deps import (
    get_get_classification_run_usecase_dep,
    get_submit_classification_run_usecase_dep,
)

router = APIRouter()
LOGGER = structlog.get_logger(__name__)


def _record_to_classification_response(record: RunRecord) -> ClassificationJobResponse:
    """Convert RunRecord to ClassificationJobResponse."""
    results = None
    if record.response_payload and "results" in record.response_payload:
        results = [ClassificationResult(**r) for r in record.response_payload["results"]]

    return ClassificationJobResponse(
        run_id=record.run_id,
        job_id=str(record.job_id),
        status=cast(RunStatusLiteral, record.status),
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
    job_id_header: Annotated[str, Header(alias="X-Alt-Job-Id")],
    submit_uc: Annotated[
        SubmitClassificationRunUsecase, Depends(get_submit_classification_run_usecase_dep)
    ],
    idempotency_key: Annotated[str | None, Header(alias="Idempotency-Key")] = None,
) -> ClassificationJobResponse:
    """Create a new classification run (async job pattern)."""
    try:
        job_id = UUID(job_id_header)
    except ValueError as exc:
        raise HTTPException(status_code=400, detail="X-Alt-Job-Id must be a valid UUID") from exc
    submission = ClassificationRunSubmission(
        job_id=job_id,
        payload=payload,
        idempotency_key=idempotency_key,
    )
    try:
        record = await submit_uc.execute(submission)
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
        LOGGER.exception(
            "classification.run.database_error",
            job_id=str(job_id),
            error_type=type(exc).__name__,
            error=str(exc),
        )
        raise HTTPException(
            status_code=503,
            detail="Database service temporarily unavailable",
        ) from exc
    except FileNotFoundError as exc:
        LOGGER.exception(
            "classification.run.model_not_found",
            job_id=str(job_id),
            error=str(exc),
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
    get_uc: Annotated[GetClassificationRunUsecase, Depends(get_get_classification_run_usecase_dep)],
) -> ClassificationJobResponse:
    """Get classification run status and results."""
    record = await get_uc.execute(run_id)
    if not record:
        raise HTTPException(status_code=404, detail="run not found")

    return _record_to_classification_response(record)
