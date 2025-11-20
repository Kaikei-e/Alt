"""Administrative endpoints."""

from __future__ import annotations

from dataclasses import asdict
from datetime import datetime, timezone

from fastapi import APIRouter, Depends, HTTPException, status

from ...domain.models import WarmupResponse
from ..deps import (
    get_learning_client,
    get_learning_service,
    get_pipeline_dep,
    get_pipeline_runner_dep,
    get_settings_dep,
)
from ...services.learning_client import LearningClient
from ...services.genre_learning import GenreLearningResult, GenreLearningService
from ...infra.config import Settings


router = APIRouter(tags=["admin"])


@router.post("/warmup", response_model=WarmupResponse)
async def warmup(
    pipeline=Depends(get_pipeline_dep),
    runner=Depends(get_pipeline_runner_dep),
) -> WarmupResponse:
    if runner is not None:
        return await runner.warmup()
    return pipeline.warmup()


@router.post("/learning", status_code=status.HTTP_202_ACCEPTED)
async def trigger_genre_learning(
    service: GenreLearningService = Depends(get_learning_service),
    client: LearningClient = Depends(get_learning_client),
    settings: Settings = Depends(get_settings_dep),
) -> dict[str, object]:
    import structlog

    logger = structlog.get_logger(__name__)
    logger.info("triggering genre learning task")
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
    # Add optimized thresholds if available
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

    payload: dict[str, object] = {
        "summary": summary,
        "graph_override": graph_override,
        "metadata": metadata,
    }
    if result.cluster_draft:
        payload["cluster_draft"] = result.cluster_draft
    return payload
