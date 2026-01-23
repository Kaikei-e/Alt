"""API routes for recap-evaluator."""

from datetime import datetime
from uuid import uuid4

import structlog
from fastapi import APIRouter, HTTPException

from recap_evaluator.api.schemas import (
    ClusterEvaluationRequest,
    ClusterMetricsResponse,
    EvaluationListResponse,
    EvaluationRequest,
    EvaluationRunResponse,
    GenreEvaluationRequest,
    GenreEvaluationResponse,
    GenreMetricsResponse,
    LatestMetricsResponse,
    PipelineMetricsResponse,
    SummaryEvaluationRequest,
    SummaryMetricsResponse,
    TrendsResponse,
)
from recap_evaluator.domain.models import EvaluationType
from recap_evaluator.evaluators.cluster import cluster_evaluator
from recap_evaluator.evaluators.genre import genre_evaluator
from recap_evaluator.evaluators.pipeline import pipeline_evaluator
from recap_evaluator.evaluators.summary import summary_evaluator
from recap_evaluator.infra.database import db

logger = structlog.get_logger()

router = APIRouter(prefix="/api/v1")


@router.get("/evaluations", response_model=EvaluationListResponse)
async def list_evaluations(
    evaluation_type: str | None = None,
    limit: int = 30,
):
    """List evaluation run history."""
    history = await db.fetch_evaluation_history(
        evaluation_type=evaluation_type,
        limit=limit,
    )

    evaluations = []
    for run in history:
        evaluations.append(
            EvaluationRunResponse(
                evaluation_id=str(run["evaluation_id"]),
                evaluation_type=run["evaluation_type"],
                job_ids=[str(jid) for jid in run.get("job_ids", [])],
                created_at=run["created_at"],
                window_days=run.get("metrics", {}).get("window_days", 14),
                overall_alert_level=run.get("metrics", {}).get("overall_alert_level", "ok"),
            )
        )

    return EvaluationListResponse(
        evaluations=evaluations,
        total=len(evaluations),
    )


@router.post("/evaluations/run", response_model=EvaluationRunResponse)
async def run_full_evaluation(request: EvaluationRequest):
    """Run a full evaluation across all dimensions."""
    evaluation_id = uuid4()
    created_at = datetime.utcnow()

    logger.info(
        "Starting full evaluation",
        evaluation_id=str(evaluation_id),
        window_days=request.window_days,
    )

    # Fetch recent jobs
    jobs = await db.fetch_recent_jobs(days=request.window_days)
    if not jobs:
        raise HTTPException(status_code=404, detail="No jobs found in the window")

    job_ids = [job["job_id"] for job in jobs]

    response = EvaluationRunResponse(
        evaluation_id=str(evaluation_id),
        evaluation_type=EvaluationType.FULL.value,
        job_ids=[str(jid) for jid in job_ids],
        created_at=created_at,
        window_days=request.window_days,
        overall_alert_level="ok",
    )

    # Run genre evaluation
    if request.include_genre:
        genre_result = await genre_evaluator.fetch_latest_evaluation()
        if genre_result:
            response.genre_metrics = GenreEvaluationResponse(
                **genre_result.to_dict(),
            )

    # Run cluster evaluation
    if request.include_cluster:
        cluster_results = await cluster_evaluator.evaluate_batch(job_ids)
        response.cluster_metrics = {
            genre: ClusterMetricsResponse(**metrics.to_dict())
            for genre, metrics in cluster_results.items()
        }

    # Run summary evaluation
    if request.include_summary:
        summary_result = await summary_evaluator.evaluate_batch(
            job_ids,
            sample_per_job=3,
        )
        response.summary_metrics = SummaryMetricsResponse(**summary_result.to_dict())

    # Run pipeline evaluation
    if request.include_pipeline:
        pipeline_result = await pipeline_evaluator.evaluate_batch(job_ids)
        response.pipeline_metrics = PipelineMetricsResponse(**pipeline_result.to_dict())

    # Determine overall alert level
    alert_levels = []
    if response.genre_metrics:
        alert_levels.append(response.genre_metrics.alert_level)
    if response.summary_metrics:
        alert_levels.append(response.summary_metrics.alert_level)
    if response.pipeline_metrics:
        alert_levels.append(response.pipeline_metrics.alert_level)

    if "critical" in alert_levels:
        response.overall_alert_level = "critical"
    elif "warn" in alert_levels:
        response.overall_alert_level = "warn"
    else:
        response.overall_alert_level = "ok"

    logger.info(
        "Full evaluation completed",
        evaluation_id=str(evaluation_id),
        overall_alert_level=response.overall_alert_level,
    )

    return response


@router.post("/evaluations/genre", response_model=GenreEvaluationResponse)
async def run_genre_evaluation(request: GenreEvaluationRequest):
    """Run genre classification evaluation."""
    if request.trigger_new:
        # Trigger new evaluation on recap-worker
        result = await genre_evaluator.trigger_evaluation()
        if not result:
            raise HTTPException(
                status_code=500,
                detail="Failed to trigger genre evaluation",
            )

    # Fetch latest results
    genre_result = await genre_evaluator.fetch_latest_evaluation()
    if not genre_result:
        raise HTTPException(
            status_code=404,
            detail="No genre evaluation results available",
        )

    return GenreEvaluationResponse(
        **genre_result.to_dict(),
    )


@router.post("/evaluations/cluster")
async def run_cluster_evaluation(
    request: ClusterEvaluationRequest,
) -> dict[str, ClusterMetricsResponse]:
    """Run clustering quality evaluation."""
    jobs = await db.fetch_recent_jobs(days=request.window_days)
    if not jobs:
        raise HTTPException(status_code=404, detail="No jobs found in the window")

    job_ids = [job["job_id"] for job in jobs]
    cluster_results = await cluster_evaluator.evaluate_batch(job_ids)

    return {
        genre: ClusterMetricsResponse(**metrics.to_dict())
        for genre, metrics in cluster_results.items()
    }


@router.post("/evaluations/summary", response_model=SummaryMetricsResponse)
async def run_summary_evaluation(request: SummaryEvaluationRequest):
    """Run summary quality evaluation using G-Eval."""
    jobs = await db.fetch_recent_jobs(days=request.window_days)
    if not jobs:
        raise HTTPException(status_code=404, detail="No jobs found in the window")

    job_ids = [job["job_id"] for job in jobs]
    summary_result = await summary_evaluator.evaluate_batch(
        job_ids,
        sample_per_job=request.sample_per_job,
    )

    return SummaryMetricsResponse(**summary_result.to_dict())


@router.get("/evaluations/{evaluation_id}", response_model=EvaluationRunResponse)
async def get_evaluation(evaluation_id: str):
    """Get a specific evaluation by ID."""
    history = await db.fetch_evaluation_history(limit=100)

    for run in history:
        if str(run["evaluation_id"]) == evaluation_id:
            return EvaluationRunResponse(
                evaluation_id=str(run["evaluation_id"]),
                evaluation_type=run["evaluation_type"],
                job_ids=[str(jid) for jid in run.get("job_ids", [])],
                created_at=run["created_at"],
                window_days=run.get("metrics", {}).get("window_days", 14),
                overall_alert_level=run.get("metrics", {}).get("overall_alert_level", "ok"),
            )

    raise HTTPException(status_code=404, detail="Evaluation not found")


@router.get("/metrics/latest", response_model=LatestMetricsResponse)
async def get_latest_metrics():
    """Get the latest metrics summary."""
    response = LatestMetricsResponse()

    # Fetch latest genre evaluation
    genre_result = await genre_evaluator.fetch_latest_evaluation()
    if genre_result:
        response.genre_macro_f1 = genre_result.macro_f1
        response.genre_alert_level = genre_result.alert_level.value

    # Fetch latest jobs for other evaluations
    jobs = await db.fetch_recent_jobs(days=7)
    if jobs:
        job_ids = [job["job_id"] for job in jobs]

        # Cluster metrics (simplified)
        cluster_results = await cluster_evaluator.evaluate_batch(job_ids[:5])
        if cluster_results:
            avg_silhouette = sum(m.silhouette_score for m in cluster_results.values()) / len(
                cluster_results
            )
            response.cluster_avg_silhouette = avg_silhouette
            # Determine alert level based on average
            if avg_silhouette < 0.15:
                response.cluster_alert_level = "critical"
            elif avg_silhouette < 0.25:
                response.cluster_alert_level = "warn"
            else:
                response.cluster_alert_level = "ok"

        # Pipeline metrics
        pipeline_result = await pipeline_evaluator.evaluate_batch(job_ids)
        response.pipeline_success_rate = pipeline_result.success_rate
        response.pipeline_alert_level = pipeline_result.alert_level.value

        if jobs:
            response.last_evaluation_at = jobs[0]["kicked_at"]

    return response


@router.get("/metrics/trends", response_model=TrendsResponse)
async def get_metrics_trends(window_days: int = 30):
    """Get metrics trends over time."""
    # This would require storing historical metrics
    # For now, return empty trends
    return TrendsResponse(
        trends=[],
        window_days=window_days,
    )
