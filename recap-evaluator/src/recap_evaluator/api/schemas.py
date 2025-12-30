"""Pydantic schemas for API requests and responses."""

from datetime import datetime
from typing import Any

from pydantic import BaseModel, Field


class HealthResponse(BaseModel):
    """Health check response."""

    status: str
    service: str
    version: str


class EvaluationRequest(BaseModel):
    """Request to run an evaluation."""

    window_days: int = Field(default=14, ge=1, le=30)
    include_genre: bool = Field(default=True)
    include_cluster: bool = Field(default=True)
    include_summary: bool = Field(default=True)
    include_pipeline: bool = Field(default=True)
    summary_sample_size: int = Field(default=50, ge=1, le=200)


class GenreEvaluationRequest(BaseModel):
    """Request for genre-only evaluation."""

    trigger_new: bool = Field(
        default=False,
        description="Trigger a new evaluation on recap-worker",
    )


class ClusterEvaluationRequest(BaseModel):
    """Request for cluster evaluation."""

    window_days: int = Field(default=14, ge=1, le=30)


class SummaryEvaluationRequest(BaseModel):
    """Request for summary (G-Eval) evaluation."""

    window_days: int = Field(default=14, ge=1, le=30)
    sample_size: int = Field(default=50, ge=1, le=200)
    sample_per_job: int = Field(default=3, ge=1, le=10)


class MetricResponse(BaseModel):
    """Generic metric response."""

    name: str
    value: float
    alert_level: str
    threshold_warn: float | None = None
    threshold_critical: float | None = None


class GenreMetricsResponse(BaseModel):
    """Per-genre metrics response."""

    genre: str
    tp: int
    fp: int
    fn: int
    precision: float
    recall: float
    f1_score: float
    support: int


class GenreEvaluationResponse(BaseModel):
    """Genre evaluation response."""

    macro_precision: float
    macro_recall: float
    macro_f1: float
    micro_precision: float
    micro_recall: float
    micro_f1: float
    weighted_f1: float
    total_samples: int
    alert_level: str
    per_genre: list[GenreMetricsResponse] = Field(default_factory=list)


class ClusterMetricsResponse(BaseModel):
    """Cluster metrics response."""

    silhouette_score: float
    davies_bouldin_index: float
    calinski_harabasz_index: float
    num_clusters: int
    avg_cluster_size: float
    min_cluster_size: int
    max_cluster_size: int
    alert_level: str
    # Optional external metrics
    nmi: float | None = None
    ari: float | None = None
    homogeneity: float | None = None
    completeness: float | None = None
    v_measure: float | None = None


class SummaryMetricsResponse(BaseModel):
    """Summary (G-Eval) metrics response."""

    coherence: float
    consistency: float
    fluency: float
    relevance: float
    overall: float
    sample_count: int
    success_count: int
    alert_level: str


class PipelineMetricsResponse(BaseModel):
    """Pipeline health metrics response."""

    total_jobs: int
    completed_jobs: int
    failed_jobs: int
    success_rate: float
    avg_articles_per_job: float
    avg_processing_time_seconds: float
    stage_success_rates: dict[str, float]
    alert_level: str


class EvaluationRunResponse(BaseModel):
    """Complete evaluation run response."""

    evaluation_id: str
    evaluation_type: str
    job_ids: list[str]
    created_at: datetime
    window_days: int
    overall_alert_level: str
    genre_metrics: GenreEvaluationResponse | None = None
    cluster_metrics: dict[str, ClusterMetricsResponse] | None = None
    summary_metrics: SummaryMetricsResponse | None = None
    pipeline_metrics: PipelineMetricsResponse | None = None


class EvaluationListResponse(BaseModel):
    """List of evaluation runs."""

    evaluations: list[EvaluationRunResponse]
    total: int


class TrendDataPoint(BaseModel):
    """Single data point in a trend."""

    timestamp: datetime
    value: float


class MetricTrend(BaseModel):
    """Trend data for a metric."""

    metric_name: str
    data_points: list[TrendDataPoint]
    current_value: float
    change_7d: float | None = None
    change_30d: float | None = None


class TrendsResponse(BaseModel):
    """Trends response for multiple metrics."""

    trends: list[MetricTrend]
    window_days: int


class AnnotationExportRequest(BaseModel):
    """Request to export data for annotation."""

    job_id: str
    genre: str


class AnnotationImportRequest(BaseModel):
    """Request to import annotation data."""

    annotations: list[dict[str, Any]]


class LatestMetricsResponse(BaseModel):
    """Latest metrics summary response."""

    genre_macro_f1: float | None = None
    genre_alert_level: str | None = None
    cluster_avg_silhouette: float | None = None
    cluster_alert_level: str | None = None
    summary_overall: float | None = None
    summary_alert_level: str | None = None
    pipeline_success_rate: float | None = None
    pipeline_alert_level: str | None = None
    last_evaluation_at: datetime | None = None
