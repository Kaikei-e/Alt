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

    # Composite score (0-1 scale)
    overall_quality_score: float = 0.0

    # Metadata
    sample_count: int = 0
    success_count: int = 0
    alert_level: AlertLevel = AlertLevel.OK


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
