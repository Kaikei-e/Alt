"""Evaluation HTTP handlers (thin)."""

from uuid import UUID

import structlog
from fastapi import APIRouter, HTTPException, Request

from recap_evaluator.handler.schemas import (
    ClusterEvaluationRequest,
    ClusterMetricsResponse,
    EvaluationListResponse,
    EvaluationRequest,
    EvaluationRunResponse,
    GenreEvaluationRequest,
    GenreEvaluationResponse,
    PipelineMetricsResponse,
    SummaryEvaluationRequest,
    SummaryMetricsResponse,
)

logger = structlog.get_logger()

router = APIRouter(prefix="/api/v1")


@router.get("/evaluations", response_model=EvaluationListResponse)
async def list_evaluations(
    request: Request,
    evaluation_type: str | None = None,
    limit: int = 30,
):
    get_metrics = request.app.state.get_metrics
    history = await get_metrics.get_evaluation_history(
        evaluation_type=evaluation_type, limit=limit
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

    return EvaluationListResponse(evaluations=evaluations, total=len(evaluations))


@router.post("/evaluations/run", response_model=EvaluationRunResponse)
async def run_full_evaluation(request: Request, body: EvaluationRequest):
    run_evaluation = request.app.state.run_evaluation

    run = await run_evaluation.execute(
        window_days=body.window_days,
        include_genre=body.include_genre,
        include_cluster=body.include_cluster,
        include_summary=body.include_summary,
        include_pipeline=body.include_pipeline,
        sample_per_job=body.sample_per_job,
    )

    if not run.job_ids:
        raise HTTPException(status_code=404, detail="No jobs found in the window")

    response = EvaluationRunResponse(
        evaluation_id=str(run.evaluation_id),
        evaluation_type=run.evaluation_type.value,
        job_ids=[str(jid) for jid in run.job_ids],
        created_at=run.created_at,
        window_days=run.window_days,
        overall_alert_level=run.overall_alert_level.value,
    )

    if run.genre_metrics:
        response.genre_metrics = GenreEvaluationResponse.from_domain(run.genre_metrics)
    if run.cluster_metrics:
        response.cluster_metrics = {
            genre: ClusterMetricsResponse.from_domain(m)
            for genre, m in run.cluster_metrics.items()
        }
    if run.summary_metrics:
        response.summary_metrics = SummaryMetricsResponse.from_domain(run.summary_metrics)
    if run.pipeline_metrics:
        response.pipeline_metrics = PipelineMetricsResponse.from_domain(run.pipeline_metrics)

    return response


@router.post("/evaluations/genre", response_model=GenreEvaluationResponse)
async def run_genre_evaluation(request: Request, body: GenreEvaluationRequest):
    genre_eval = request.app.state.genre_evaluator

    if body.trigger_new:
        result = await genre_eval.trigger_evaluation()
        if not result:
            raise HTTPException(status_code=500, detail="Failed to trigger genre evaluation")

    genre_result = await genre_eval.fetch_latest_evaluation()
    if not genre_result:
        raise HTTPException(status_code=404, detail="No genre evaluation results available")

    return GenreEvaluationResponse.from_domain(genre_result)


@router.post("/evaluations/cluster")
async def run_cluster_evaluation(
    request: Request, body: ClusterEvaluationRequest
) -> dict[str, ClusterMetricsResponse]:
    db = request.app.state.db

    jobs = await db.fetch_recent_jobs(days=body.window_days)
    if not jobs:
        raise HTTPException(status_code=404, detail="No jobs found in the window")

    job_ids = [job["job_id"] for job in jobs]
    cluster_eval = request.app.state.cluster_evaluator
    cluster_results = await cluster_eval.evaluate_batch(job_ids)

    return {
        genre: ClusterMetricsResponse.from_domain(m)
        for genre, m in cluster_results.items()
    }


@router.post("/evaluations/summary", response_model=SummaryMetricsResponse)
async def run_summary_evaluation(request: Request, body: SummaryEvaluationRequest):
    db = request.app.state.db

    jobs = await db.fetch_recent_jobs(days=body.window_days)
    if not jobs:
        raise HTTPException(status_code=404, detail="No jobs found in the window")

    job_ids = [job["job_id"] for job in jobs]
    summary_eval = request.app.state.summary_evaluator
    summary_result = await summary_eval.evaluate_batch(
        job_ids, sample_per_job=body.sample_per_job
    )

    return SummaryMetricsResponse.from_domain(summary_result)


@router.get("/evaluations/{evaluation_id}", response_model=EvaluationRunResponse)
async def get_evaluation(request: Request, evaluation_id: UUID):
    get_metrics = request.app.state.get_metrics
    run = await get_metrics.get_evaluation_by_id(evaluation_id)

    if not run:
        raise HTTPException(status_code=404, detail="Evaluation not found")

    return EvaluationRunResponse(
        evaluation_id=str(run["evaluation_id"]),
        evaluation_type=run["evaluation_type"],
        job_ids=[str(jid) for jid in run.get("job_ids", [])],
        created_at=run["created_at"],
        window_days=run.get("metrics", {}).get("window_days", 14),
        overall_alert_level=run.get("metrics", {}).get("overall_alert_level", "ok"),
    )
