"""Domain models for recap-evaluator."""

from dataclasses import dataclass, field
from datetime import datetime
from enum import Enum
from typing import Any
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

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "name": self.name,
            "value": round(self.value, 4),
            "alert_level": self.alert_level.value,
            "threshold_warn": self.threshold_warn,
            "threshold_critical": self.threshold_critical,
        }


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

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "genre": self.genre,
            "tp": self.tp,
            "fp": self.fp,
            "fn": self.fn,
            "precision": round(self.precision, 4),
            "recall": round(self.recall, 4),
            "f1_score": round(self.f1_score, 4),
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

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "macro_precision": round(self.macro_precision, 4),
            "macro_recall": round(self.macro_recall, 4),
            "macro_f1": round(self.macro_f1, 4),
            "micro_precision": round(self.micro_precision, 4),
            "micro_recall": round(self.micro_recall, 4),
            "micro_f1": round(self.micro_f1, 4),
            "weighted_f1": round(self.weighted_f1, 4),
            "total_samples": self.total_samples,
            "alert_level": self.alert_level.value,
            "per_genre": [m.to_dict() for m in self.per_genre_metrics],
        }


@dataclass
class ClusterMetrics:
    """Clustering quality metrics."""

    silhouette_score: float = 0.0
    davies_bouldin_index: float = 0.0
    calinski_harabasz_index: float = 0.0
    # External metrics (require ground truth)
    nmi: float | None = None
    ari: float | None = None
    homogeneity: float | None = None
    completeness: float | None = None
    v_measure: float | None = None
    # Cluster statistics
    num_clusters: int = 0
    avg_cluster_size: float = 0.0
    min_cluster_size: int = 0
    max_cluster_size: int = 0
    alert_level: AlertLevel = AlertLevel.OK

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        result = {
            "silhouette_score": round(self.silhouette_score, 4),
            "davies_bouldin_index": round(self.davies_bouldin_index, 4),
            "calinski_harabasz_index": round(self.calinski_harabasz_index, 4),
            "num_clusters": self.num_clusters,
            "avg_cluster_size": round(self.avg_cluster_size, 2),
            "min_cluster_size": self.min_cluster_size,
            "max_cluster_size": self.max_cluster_size,
            "alert_level": self.alert_level.value,
        }
        # Include external metrics if available
        if self.nmi is not None:
            result["nmi"] = round(self.nmi, 4)
        if self.ari is not None:
            result["ari"] = round(self.ari, 4)
        if self.homogeneity is not None:
            result["homogeneity"] = round(self.homogeneity, 4)
        if self.completeness is not None:
            result["completeness"] = round(self.completeness, 4)
        if self.v_measure is not None:
            result["v_measure"] = round(self.v_measure, 4)
        return result


@dataclass
class SummaryMetrics:
    """Summary quality metrics from G-Eval."""

    coherence: float = 0.0
    consistency: float = 0.0
    fluency: float = 0.0
    relevance: float = 0.0
    overall: float = 0.0
    sample_count: int = 0
    success_count: int = 0
    alert_level: AlertLevel = AlertLevel.OK

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "coherence": round(self.coherence, 3),
            "consistency": round(self.consistency, 3),
            "fluency": round(self.fluency, 3),
            "relevance": round(self.relevance, 3),
            "overall": round(self.overall, 3),
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

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        return {
            "total_jobs": self.total_jobs,
            "completed_jobs": self.completed_jobs,
            "failed_jobs": self.failed_jobs,
            "success_rate": round(self.success_rate, 4),
            "avg_articles_per_job": round(self.avg_articles_per_job, 2),
            "avg_processing_time_seconds": round(self.avg_processing_time_seconds, 2),
            "stage_success_rates": {k: round(v, 4) for k, v in self.stage_success_rates.items()},
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

    def to_dict(self) -> dict[str, Any]:
        """Convert to dictionary."""
        result: dict[str, Any] = {
            "evaluation_id": str(self.evaluation_id),
            "evaluation_type": self.evaluation_type.value,
            "job_ids": [str(jid) for jid in self.job_ids],
            "created_at": self.created_at.isoformat(),
            "window_days": self.window_days,
            "overall_alert_level": self.overall_alert_level.value,
        }
        if self.genre_metrics:
            result["genre_metrics"] = self.genre_metrics.to_dict()
        if self.cluster_metrics:
            result["cluster_metrics"] = {k: v.to_dict() for k, v in self.cluster_metrics.items()}
        if self.summary_metrics:
            result["summary_metrics"] = self.summary_metrics.to_dict()
        if self.pipeline_metrics:
            result["pipeline_metrics"] = self.pipeline_metrics.to_dict()
        return result
