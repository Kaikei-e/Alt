"""Domain models for recap-evaluator."""

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from uuid import UUID


class EvaluationType(str, Enum):
    """Types of evaluation."""

    GENRE = "genre"
    CLUSTER = "cluster"
    SUMMARY = "summary"
    PIPELINE = "pipeline"
    FULL = "full"


class AlertLevel(str, Enum):
    """Alert severity levels."""

    OK = "ok"
    WARN = "warn"
    CRITICAL = "critical"


@dataclass
class MetricValue:
    """A single metric value with optional alert status."""

    name: str
    value: float
    alert_level: AlertLevel = AlertLevel.OK
    threshold_warn: float | None = None
    threshold_critical: float | None = None


@dataclass
class GenreMetrics:
    """Per-genre classification metrics."""

    genre: str
    tp: int = 0
    fp: int = 0
    fn: int = 0
    precision: float = 0.0
    recall: float = 0.0
    f1_score: float = 0.0
    support: int = 0

    def to_dict(self) -> dict:
        return {
            "genre": self.genre,
            "tp": self.tp,
            "fp": self.fp,
            "fn": self.fn,
            "precision": self.precision,
            "recall": self.recall,
            "f1_score": self.f1_score,
            "support": self.support,
        }


@dataclass
class GenreEvaluationResult:
    """Genre classification evaluation result."""

    macro_precision: float = 0.0
    macro_recall: float = 0.0
    macro_f1: float = 0.0
    micro_precision: float = 0.0
    micro_recall: float = 0.0
    micro_f1: float = 0.0
    weighted_f1: float = 0.0
    per_genre_metrics: list[GenreMetrics] = field(default_factory=list)
    total_samples: int = 0
    alert_level: AlertLevel = AlertLevel.OK

    def to_dict(self) -> dict:
        return {
            "macro_precision": self.macro_precision,
            "macro_recall": self.macro_recall,
            "macro_f1": self.macro_f1,
            "micro_precision": self.micro_precision,
            "micro_recall": self.micro_recall,
            "micro_f1": self.micro_f1,
            "weighted_f1": self.weighted_f1,
            "total_samples": self.total_samples,
            "alert_level": self.alert_level.value,
            "per_genre_metrics": [m.to_dict() for m in self.per_genre_metrics],
        }


@dataclass
class ClusterMetrics:
    """Clustering quality metrics."""

    silhouette_score: float = 0.0
    davies_bouldin_index: float = 0.0
    calinski_harabasz_index: float = 0.0
    nmi: float | None = None
    ari: float | None = None
    homogeneity: float | None = None
    completeness: float | None = None
    v_measure: float | None = None
    num_clusters: int = 0
    avg_cluster_size: float = 0.0
    min_cluster_size: int = 0
    max_cluster_size: int = 0
    alert_level: AlertLevel = AlertLevel.OK

    def to_dict(self) -> dict:
        d: dict = {
            "silhouette_score": self.silhouette_score,
            "davies_bouldin_index": self.davies_bouldin_index,
            "calinski_harabasz_index": self.calinski_harabasz_index,
            "num_clusters": self.num_clusters,
            "avg_cluster_size": self.avg_cluster_size,
            "min_cluster_size": self.min_cluster_size,
            "max_cluster_size": self.max_cluster_size,
            "alert_level": self.alert_level.value,
        }
        if self.nmi is not None:
            d["nmi"] = self.nmi
        if self.ari is not None:
            d["ari"] = self.ari
        if self.homogeneity is not None:
            d["homogeneity"] = self.homogeneity
        if self.completeness is not None:
            d["completeness"] = self.completeness
        if self.v_measure is not None:
            d["v_measure"] = self.v_measure
        return d


@dataclass
class SummaryMetrics:
    """Summary quality metrics from multi-dimensional evaluation."""

    # G-Eval metrics (1-5 scale)
    coherence: float = 0.0
    consistency: float = 0.0
    fluency: float = 0.0
    relevance: float = 0.0
    geval_overall: float = 0.0

    # ROUGE metrics (0-1 scale)
    rouge_1_f1: float = 0.0
    rouge_2_f1: float = 0.0
    rouge_l_f1: float = 0.0

    # BERTScore metrics (0-1 scale)
    bertscore_precision: float = 0.0
    bertscore_recall: float = 0.0
    bertscore_f1: float = 0.0

    # Faithfulness metrics (0-1 scale)
    faithfulness_score: float = 0.0
    hallucination_rate: float = 0.0

    # Morning-letter quality axes (0-1 except readability which is 1-5)
    fallback_rate: float = 0.0
    json_repair_rate: float = 0.0
    redundancy_score: float = 0.0
    readability_score: float = 0.0
    source_grounding_score: float = 0.0

    # Composite score (0-1 scale)
    overall_quality_score: float = 0.0

    # Metadata
    sample_count: int = 0
    success_count: int = 0
    alert_level: AlertLevel = AlertLevel.OK

    def to_dict(self) -> dict:
        return {
            "coherence": self.coherence,
            "consistency": self.consistency,
            "fluency": self.fluency,
            "relevance": self.relevance,
            "geval_overall": self.geval_overall,
            "rouge_1_f1": self.rouge_1_f1,
            "rouge_2_f1": self.rouge_2_f1,
            "rouge_l_f1": self.rouge_l_f1,
            "bertscore_precision": self.bertscore_precision,
            "bertscore_recall": self.bertscore_recall,
            "bertscore_f1": self.bertscore_f1,
            "faithfulness_score": self.faithfulness_score,
            "hallucination_rate": self.hallucination_rate,
            "fallback_rate": self.fallback_rate,
            "json_repair_rate": self.json_repair_rate,
            "redundancy_score": self.redundancy_score,
            "readability_score": self.readability_score,
            "source_grounding_score": self.source_grounding_score,
            "overall_quality_score": self.overall_quality_score,
            "sample_count": self.sample_count,
            "success_count": self.success_count,
            "alert_level": self.alert_level.value,
        }


@dataclass
class PipelineMetrics:
    """Pipeline health metrics."""

    total_jobs: int = 0
    completed_jobs: int = 0
    failed_jobs: int = 0
    success_rate: float = 0.0
    avg_articles_per_job: float = 0.0
    avg_processing_time_seconds: float = 0.0
    stage_success_rates: dict[str, float] = field(default_factory=dict)
    alert_level: AlertLevel = AlertLevel.OK

    def to_dict(self) -> dict:
        return {
            "total_jobs": self.total_jobs,
            "completed_jobs": self.completed_jobs,
            "failed_jobs": self.failed_jobs,
            "success_rate": self.success_rate,
            "avg_articles_per_job": self.avg_articles_per_job,
            "avg_processing_time_seconds": self.avg_processing_time_seconds,
            "stage_success_rates": self.stage_success_rates,
            "alert_level": self.alert_level.value,
        }


@dataclass
class EvaluationRun:
    """Complete evaluation run result."""

    evaluation_id: UUID
    evaluation_type: EvaluationType
    job_ids: list[UUID]
    created_at: datetime
    window_days: int
    genre_metrics: GenreEvaluationResult | None = None
    cluster_metrics: dict[str, ClusterMetrics] = field(default_factory=dict)
    summary_metrics: SummaryMetrics | None = None
    pipeline_metrics: PipelineMetrics | None = None
    overall_alert_level: AlertLevel = AlertLevel.OK

    def to_metrics_dict(self) -> dict:
        """Serialize evaluation metrics to a JSON-compatible dict."""
        metrics: dict = {
            "overall_alert_level": self.overall_alert_level.value,
        }
        if self.genre_metrics is not None:
            metrics["genre"] = self.genre_metrics.to_dict()
        if self.cluster_metrics:
            metrics["cluster"] = {
                genre: m.to_dict() for genre, m in self.cluster_metrics.items()
            }
        if self.summary_metrics is not None:
            metrics["summary"] = self.summary_metrics.to_dict()
        if self.pipeline_metrics is not None:
            metrics["pipeline"] = self.pipeline_metrics.to_dict()
        return metrics
