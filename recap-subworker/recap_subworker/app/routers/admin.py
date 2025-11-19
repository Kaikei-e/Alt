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
)
from ...services.learning_client import LearningClient
from ...services.genre_learning import GenreLearningResult, GenreLearningService


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
) -> dict[str, object]:
    learning_result = await service.generate_learning_result()
    payload = _build_learning_payload(learning_result)
    try:
        response = await client.send_learning_payload(payload)
    except Exception as exc:  # pragma: no cover - HTTP interactions
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
    payload: dict[str, object] = {
        "summary": summary,
        "graph_override": {
            "graph_margin": result.summary.graph_margin_reference,
        },
        "metadata": {
            "captured_at": datetime.now(timezone.utc).isoformat(),
            "entries_observed": result.summary.total_records,
        },
    }
    if result.cluster_draft:
        payload["cluster_draft"] = result.cluster_draft
    return payload
