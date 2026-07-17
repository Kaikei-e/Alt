"""Administrative endpoints."""

from __future__ import annotations

from datetime import datetime
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel

from ...domain.models import WarmupResponse
from ...infra.config import Settings
from ...services.async_jobs import AdminJobService, ConcurrentAdminJobError
from ...services.tag_label_graph_builder import TagLabelGraphBuilder
from ..container import ServiceContainer
from ..deps import (
    get_admin_job_service_dep,
    get_container,
    get_pipeline_dep,
    get_pipeline_runner_dep,
    get_settings_dep,
)

router = APIRouter(tags=["admin"])


class AdminJobResponse(BaseModel):
    job_id: UUID
    kind: str
    status: str
    started_at: datetime
    finished_at: datetime | None = None
    result: dict[str, object] | None = None
    error: str | None = None


@router.post("/warmup", response_model=WarmupResponse)
async def warmup(
    pipeline=Depends(get_pipeline_dep),
    runner=Depends(get_pipeline_runner_dep),
) -> WarmupResponse:
    if runner is not None:
        return await runner.warmup()
    return pipeline.warmup()


@router.post("/build-graph", status_code=status.HTTP_200_OK)
async def build_tag_label_graph(
    settings: Settings = Depends(get_settings_dep),
    container: ServiceContainer = Depends(get_container),
) -> dict[str, object]:
    """Manually trigger tag_label_graph rebuild."""
    import structlog

    logger = structlog.get_logger(__name__)
    logger.info("manually triggering tag_label_graph rebuild")

    if not settings.graph_build_enabled:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="graph_build_enabled is False",
        )

    try:
        async with container.db.session_factory() as session:
            builder = TagLabelGraphBuilder(
                session=session,
                max_tags=settings.graph_build_max_tags,
                min_confidence=settings.graph_build_min_confidence,
                min_support=settings.graph_build_min_support,
            )
            windows = [
                int(w.strip())
                for w in settings.graph_build_windows.split(",")
                if w.strip()
            ]

            results: dict[str, int] = {}
            for window_days in windows:
                edge_count = await builder.build_graph(window_days)
                results[f"{window_days}d"] = edge_count
                logger.info(
                    "tag_label_graph built",
                    window_days=window_days,
                    edge_count=edge_count,
                )

            return {
                "status": "success",
                "edge_counts": results,
                "total_edges": sum(results.values()),
            }
    except Exception as exc:
        logger.error(
            "failed to build tag_label_graph",
            error=str(exc),
            error_type=type(exc).__name__,
            exc_info=True,
        )
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="failed to build tag_label_graph",
        ) from exc


@router.post("/learning", status_code=status.HTTP_202_ACCEPTED)
async def trigger_genre_learning(
    service: AdminJobService = Depends(get_admin_job_service_dep),
) -> dict[str, object]:
    """Enqueue a genre learning run and return immediately.

    Delegates to the same background job mechanism as `/learning-jobs`
    instead of running graph rebuild + learning + recap-worker delivery
    synchronously before responding, which contradicted the declared 202.
    """
    import structlog

    logger = structlog.get_logger(__name__)
    try:
        job_id = await service.enqueue_learning_job()
        logger.info("genre learning job enqueued", job_id=str(job_id))
    except ConcurrentAdminJobError as exc:
        logger.warning("learning job already running", error=str(exc))
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    except Exception as exc:
        logger.error("failed to enqueue learning job", error=str(exc), exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="failed to enqueue learning job",
        ) from exc
    return {"job_id": job_id}


def _job_to_response(record) -> AdminJobResponse:
    return AdminJobResponse(
        job_id=record.job_id,
        kind=record.kind,
        status=record.status,
        started_at=record.started_at,
        finished_at=record.finished_at,
        result=record.result,
        error=record.error,
    )


@router.post("/graph-jobs", status_code=status.HTTP_202_ACCEPTED)
async def create_graph_job(
    service: AdminJobService = Depends(get_admin_job_service_dep),
) -> dict[str, object]:
    import structlog
    logger = structlog.get_logger(__name__)
    try:
        job_id = await service.enqueue_graph_job()
        logger.info("graph job enqueued", job_id=str(job_id))
    except ConcurrentAdminJobError as exc:
        logger.warning("graph job already running", error=str(exc))
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    except Exception as exc:
        logger.error("failed to enqueue graph job", error=str(exc), exc_info=True)
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="failed to enqueue graph job",
        ) from exc
    return {"job_id": job_id}


@router.get("/graph-jobs/{job_id}", response_model=AdminJobResponse)
async def get_graph_job(
    job_id: UUID,
    service: AdminJobService = Depends(get_admin_job_service_dep),
) -> AdminJobResponse:
    record = await service.get_job(job_id)
    if not record:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="job not found")
    return _job_to_response(record)


@router.post("/learning-jobs", status_code=status.HTTP_202_ACCEPTED)
async def create_learning_job(
    service: AdminJobService = Depends(get_admin_job_service_dep),
) -> dict[str, object]:
    try:
        job_id = await service.enqueue_learning_job()
    except ConcurrentAdminJobError as exc:
        raise HTTPException(status_code=status.HTTP_409_CONFLICT, detail=str(exc)) from exc
    except Exception as exc:
        raise HTTPException(
            status_code=status.HTTP_500_INTERNAL_SERVER_ERROR,
            detail="failed to enqueue learning job",
        ) from exc
    return {"job_id": job_id}


@router.get("/learning-jobs/{job_id}", response_model=AdminJobResponse)
async def get_learning_job(
    job_id: UUID,
    service: AdminJobService = Depends(get_admin_job_service_dep),
) -> AdminJobResponse:
    record = await service.get_job(job_id)
    if not record:
        raise HTTPException(status_code=status.HTTP_404_NOT_FOUND, detail="job not found")
    return _job_to_response(record)
