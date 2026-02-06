"""Metrics HTTP handlers."""

from fastapi import APIRouter, Request

from recap_evaluator.handler.schemas import (
    LatestMetricsResponse,
    TrendsResponse,
)

router = APIRouter(prefix="/api/v1")


@router.get("/metrics/latest", response_model=LatestMetricsResponse)
async def get_latest_metrics(request: Request):
    get_metrics = request.app.state.get_metrics
    data = await get_metrics.get_latest()

    return LatestMetricsResponse(
        genre_macro_f1=data.get("genre_macro_f1"),
        genre_alert_level=data.get("genre_alert_level"),
        cluster_avg_silhouette=data.get("cluster_avg_silhouette"),
        cluster_alert_level=data.get("cluster_alert_level"),
        pipeline_success_rate=data.get("pipeline_success_rate"),
        pipeline_alert_level=data.get("pipeline_alert_level"),
        last_evaluation_at=data.get("last_evaluation_at"),
    )


@router.get("/metrics/trends", response_model=TrendsResponse)
async def get_metrics_trends(window_days: int = 30):
    return TrendsResponse(trends=[], window_days=window_days)
