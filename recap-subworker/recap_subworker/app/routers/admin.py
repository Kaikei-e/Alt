"""Administrative endpoints."""

from __future__ import annotations

from dataclasses import asdict
from datetime import datetime, timezone
from uuid import UUID

from fastapi import APIRouter, Depends, HTTPException, status
from pydantic import BaseModel

from ...domain.models import WarmupResponse
from ..deps import (
    get_admin_job_service_dep,
    get_learning_client,
    get_learning_service,
    get_pipeline_dep,
    get_pipeline_runner_dep,
    get_settings_dep,
)
from ...services.learning_client import LearningClient
from ...services.genre_learning import GenreLearningResult, GenreLearningService
from ...services.tag_label_graph_builder import TagLabelGraphBuilder
from ...infra.config import Settings
from ...services.async_jobs import AdminJobService, ConcurrentAdminJobError


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
) -> dict[str, object]:
    """Manually trigger tag_label_graph rebuild."""
    import structlog
    from ...db.session import get_session_factory

    logger = structlog.get_logger(__name__)
    logger.info("manually triggering tag_label_graph rebuild")

    if not settings.graph_build_enabled:
        raise HTTPException(
            status_code=status.HTTP_400_BAD_REQUEST,
            detail="graph_build_enabled is False",
        )

    try:
        session_factory = get_session_factory(settings)
        async with session_factory() as session:
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
            detail=f"failed to build tag_label_graph: {exc}",
        ) from exc


@router.post("/learning", status_code=status.HTTP_202_ACCEPTED)
async def trigger_genre_learning(
    service: GenreLearningService = Depends(get_learning_service),
    client: LearningClient = Depends(get_learning_client),
    settings: Settings = Depends(get_settings_dep),
) -> dict[str, object]:
    import structlog
    from ...db.session import get_session_factory

    logger = structlog.get_logger(__name__)
    logger.info("triggering genre learning task")

    # Phase 1: Rebuild tag_label_graph BEFORE learning
    if settings.graph_build_enabled:
        try:
            logger.debug("rebuilding tag_label_graph before learning")
            from ...db.session import get_session_factory
            session_factory = get_session_factory(settings)
            async with session_factory() as session:
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
                for window_days in windows:
                    edge_count = await builder.build_graph(window_days)
                    logger.info(
                        "tag_label_graph rebuilt before learning",
                        window_days=window_days,
                        edge_count=edge_count,
                    )
        except Exception as exc:
            logger.error(
                "failed to rebuild tag_label_graph before learning",
                error=str(exc),
                error_type=type(exc).__name__,
                exc_info=True,
            )
            # Continue with learning even if graph rebuild fails

    try:
        logger.debug("generating learning result")
        learning_result = await service.generate_learning_result(
            days=settings.learning_snapshot_days
        )
        logger.info(
            "learning result generated",
            total_records=learning_result.summary.total_records,
            graph_boost_count=learning_result.summary.graph_boost_count,
        )
        payload = _build_learning_payload(learning_result)
        logger.debug("sending learning payload to recap-worker")
        response = await client.send_learning_payload(payload)
        logger.info(
            "learning payload sent successfully",
            recap_worker_status=response.status_code,
        )
    except Exception as exc:  # pragma: no cover - HTTP interactions
        logger.error(
            "failed to send learning payload",
            error=str(exc),
            error_type=type(exc).__name__,
            exc_info=True,
        )
        raise HTTPException(
            status_code=status.HTTP_502_BAD_GATEWAY,
            detail="failed to send learning payload",
        ) from exc
    data: dict[str, object] = {
        "status": "sent",
        "recap_worker_status": response.status_code,
    }
    if response.headers.get("content-type", "").startswith("application/json"):
        data["recap_worker_response"] = response.json()
    return data


def _build_learning_payload(result: GenreLearningResult) -> dict[str, object]:
    summary = asdict(result.summary)
    graph_override: dict[str, object] = {
        "graph_margin": result.summary.graph_margin_reference,
    }
    # Add optimized thresholds if available (always include if set, even if 0.0/0)
    if result.summary.boost_threshold_reference is not None:
        graph_override["boost_threshold"] = result.summary.boost_threshold_reference
    if result.summary.tag_count_threshold_reference is not None:
        graph_override["tag_count_threshold"] = result.summary.tag_count_threshold_reference

    metadata: dict[str, object] = {
        "captured_at": datetime.now(timezone.utc).isoformat(),
        "entries_observed": result.summary.total_records,
    }
    if result.summary.accuracy_estimate is not None:
        metadata["accuracy_estimate"] = result.summary.accuracy_estimate
    if result.summary.test_accuracy is not None:
        metadata["test_accuracy"] = result.summary.test_accuracy

    payload: dict[str, object] = {
        "summary": summary,
        "graph_override": graph_override,
        "metadata": metadata,
    }
    if result.cluster_draft:
        payload["cluster_draft"] = result.cluster_draft
    return payload


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
