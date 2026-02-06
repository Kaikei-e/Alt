"""Pydantic schemas for API requests and responses."""

from datetime import datetime

from pydantic import BaseModel, Field

from recap_evaluator.domain.models import (
    ClusterMetrics,
    GenreEvaluationResult,
    GenreMetrics,
    PipelineMetrics,
    SummaryMetrics,
)

# --- Request schemas ---


class EvaluationRequest(BaseModel):
    window_days: int = Field(default=14, ge=1, le=90)
    include_genre: bool = Field(default=True)
    include_cluster: bool = Field(default=True)
    include_summary: bool = Field(default=True)
    include_pipeline: bool = Field(default=True)
    sample_per_job: int = Field(default=3, ge=1, le=20)


class GenreEvaluationRequest(BaseModel):
    trigger_new: bool = Field(default=False)


class ClusterEvaluationRequest(BaseModel):
    window_days: int = Field(default=14, ge=1, le=90)


class SummaryEvaluationRequest(BaseModel):
    window_days: int = Field(default=14, ge=1, le=90)
    sample_per_job: int = Field(default=3, ge=1, le=20)


# --- Response schemas ---


class GenreMetricsResponse(BaseModel):
    genre: str
    tp: int
    fp: int
    fn: int
    precision: float
    recall: float
    f1_score: float
    support: int

    @classmethod
    def from_domain(cls, m: GenreMetrics) -> "GenreMetricsResponse":
        return cls(
            genre=m.genre, tp=m.tp, fp=m.fp, fn=m.fn,
            precision=round(m.precision, 4),
            recall=round(m.recall, 4),
            f1_score=round(m.f1_score, 4),
            support=m.support,
        )


class GenreEvaluationResponse(BaseModel):
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

    @classmethod
    def from_domain(cls, r: GenreEvaluationResult) -> "GenreEvaluationResponse":
        return cls(
            macro_precision=round(r.macro_precision, 4),
            macro_recall=round(r.macro_recall, 4),
            macro_f1=round(r.macro_f1, 4),
            micro_precision=round(r.micro_precision, 4),
            micro_recall=round(r.micro_recall, 4),
            micro_f1=round(r.micro_f1, 4),
            weighted_f1=round(r.weighted_f1, 4),
            total_samples=r.total_samples,
            alert_level=r.alert_level.value,
            per_genre=[GenreMetricsResponse.from_domain(m) for m in r.per_genre_metrics],
        )


class ClusterMetricsResponse(BaseModel):
    silhouette_score: float
    davies_bouldin_index: float
    calinski_harabasz_index: float
    num_clusters: int
    avg_cluster_size: float
    min_cluster_size: int
    max_cluster_size: int
    alert_level: str
    nmi: float | None = None
    ari: float | None = None
    homogeneity: float | None = None
    completeness: float | None = None
    v_measure: float | None = None

    @classmethod
    def from_domain(cls, m: ClusterMetrics) -> "ClusterMetricsResponse":
        return cls(
            silhouette_score=round(m.silhouette_score, 4),
            davies_bouldin_index=round(m.davies_bouldin_index, 4),
            calinski_harabasz_index=round(m.calinski_harabasz_index, 4),
            num_clusters=m.num_clusters,
            avg_cluster_size=round(m.avg_cluster_size, 2),
            min_cluster_size=m.min_cluster_size,
            max_cluster_size=m.max_cluster_size,
            alert_level=m.alert_level.value,
            nmi=round(m.nmi, 4) if m.nmi is not None else None,
            ari=round(m.ari, 4) if m.ari is not None else None,
            homogeneity=round(m.homogeneity, 4) if m.homogeneity is not None else None,
            completeness=round(m.completeness, 4) if m.completeness is not None else None,
            v_measure=round(m.v_measure, 4) if m.v_measure is not None else None,
        )


class SummaryMetricsResponse(BaseModel):
    coherence: float
    consistency: float
    fluency: float
    relevance: float
    geval_overall: float = 0.0
    rouge_1_f1: float = 0.0
    rouge_2_f1: float = 0.0
    rouge_l_f1: float = 0.0
    bertscore_precision: float = 0.0
    bertscore_recall: float = 0.0
    bertscore_f1: float = 0.0
    faithfulness_score: float = 0.0
    hallucination_rate: float = 0.0
    overall_quality_score: float = 0.0
    sample_count: int
    success_count: int
    alert_level: str

    @classmethod
    def from_domain(cls, m: SummaryMetrics) -> "SummaryMetricsResponse":
        return cls(
            coherence=round(m.coherence, 3),
            consistency=round(m.consistency, 3),
            fluency=round(m.fluency, 3),
            relevance=round(m.relevance, 3),
            geval_overall=round(m.geval_overall, 3),
            rouge_1_f1=round(m.rouge_1_f1, 4),
            rouge_2_f1=round(m.rouge_2_f1, 4),
            rouge_l_f1=round(m.rouge_l_f1, 4),
            bertscore_precision=round(m.bertscore_precision, 4),
            bertscore_recall=round(m.bertscore_recall, 4),
            bertscore_f1=round(m.bertscore_f1, 4),
            faithfulness_score=round(m.faithfulness_score, 4),
            hallucination_rate=round(m.hallucination_rate, 4),
            overall_quality_score=round(m.overall_quality_score, 4),
            sample_count=m.sample_count,
            success_count=m.success_count,
            alert_level=m.alert_level.value,
        )


class PipelineMetricsResponse(BaseModel):
    total_jobs: int
    completed_jobs: int
    failed_jobs: int
    success_rate: float
    avg_articles_per_job: float
    avg_processing_time_seconds: float
    stage_success_rates: dict[str, float]
    alert_level: str

    @classmethod
    def from_domain(cls, m: PipelineMetrics) -> "PipelineMetricsResponse":
        return cls(
            total_jobs=m.total_jobs,
            completed_jobs=m.completed_jobs,
            failed_jobs=m.failed_jobs,
            success_rate=round(m.success_rate, 4),
            avg_articles_per_job=round(m.avg_articles_per_job, 2),
            avg_processing_time_seconds=round(m.avg_processing_time_seconds, 2),
            stage_success_rates={k: round(v, 4) for k, v in m.stage_success_rates.items()},
            alert_level=m.alert_level.value,
        )


class EvaluationRunResponse(BaseModel):
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
    evaluations: list[EvaluationRunResponse]
    total: int


class LatestMetricsResponse(BaseModel):
    genre_macro_f1: float | None = None
    genre_alert_level: str | None = None
    cluster_avg_silhouette: float | None = None
    cluster_alert_level: str | None = None
    summary_overall: float | None = None
    summary_alert_level: str | None = None
    pipeline_success_rate: float | None = None
    pipeline_alert_level: str | None = None
    last_evaluation_at: datetime | None = None


class TrendDataPoint(BaseModel):
    timestamp: datetime
    value: float


class MetricTrend(BaseModel):
    metric_name: str
    data_points: list[TrendDataPoint]
    current_value: float
    change_7d: float | None = None
    change_30d: float | None = None


class TrendsResponse(BaseModel):
    trends: list[MetricTrend]
    window_days: int
