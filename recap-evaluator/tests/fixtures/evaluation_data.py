"""Test fixtures for evaluation result data."""

from recap_evaluator.domain.models import (
    AlertLevel,
    ClusterMetrics,
    GenreEvaluationResult,
    GenreMetrics,
    PipelineMetrics,
    SummaryMetrics,
)

SAMPLE_GENRE_EVALUATION = GenreEvaluationResult(
    macro_precision=0.85,
    macro_recall=0.80,
    macro_f1=0.82,
    micro_precision=0.88,
    micro_recall=0.83,
    micro_f1=0.85,
    weighted_f1=0.84,
    per_genre_metrics=[
        GenreMetrics(
            genre="technology",
            tp=50,
            fp=5,
            fn=3,
            precision=0.91,
            recall=0.94,
            f1_score=0.92,
            support=53,
        ),
        GenreMetrics(
            genre="business",
            tp=30,
            fp=8,
            fn=5,
            precision=0.79,
            recall=0.86,
            f1_score=0.82,
            support=35,
        ),
    ],
    total_samples=88,
    alert_level=AlertLevel.OK,
)

SAMPLE_CLUSTER_METRICS = {
    "technology": ClusterMetrics(
        silhouette_score=0.35,
        num_clusters=5,
        avg_cluster_size=10.0,
        min_cluster_size=3,
        max_cluster_size=20,
        alert_level=AlertLevel.OK,
    ),
    "business": ClusterMetrics(
        silhouette_score=0.28,
        num_clusters=3,
        avg_cluster_size=8.0,
        min_cluster_size=2,
        max_cluster_size=15,
        alert_level=AlertLevel.OK,
    ),
}

SAMPLE_SUMMARY_METRICS = SummaryMetrics(
    coherence=4.2,
    consistency=4.0,
    fluency=4.5,
    relevance=3.8,
    geval_overall=4.125,
    rouge_1_f1=0.45,
    rouge_2_f1=0.22,
    rouge_l_f1=0.38,
    bertscore_precision=0.72,
    bertscore_recall=0.68,
    bertscore_f1=0.70,
    faithfulness_score=0.75,
    hallucination_rate=0.25,
    overall_quality_score=0.68,
    sample_count=10,
    success_count=9,
    alert_level=AlertLevel.OK,
)

SAMPLE_PIPELINE_METRICS = PipelineMetrics(
    total_jobs=10,
    completed_jobs=9,
    failed_jobs=1,
    success_rate=0.9,
    avg_articles_per_job=95.0,
    avg_processing_time_seconds=3600.0,
    stage_success_rates={
        "preprocess": 1.0,
        "classify": 1.0,
        "cluster": 0.95,
        "summarize": 0.90,
        "output": 0.90,
    },
    alert_level=AlertLevel.WARN,
)

SAMPLE_GENRE_API_RESPONSE = {
    "macro_precision": 0.85,
    "macro_recall": 0.80,
    "macro_f1": 0.82,
    "micro_precision": 0.88,
    "micro_recall": 0.83,
    "micro_f1": 0.85,
    "weighted_f1": 0.84,
    "total_items": 88,
    "per_genre_metrics": [
        {
            "genre": "technology",
            "tp": 50,
            "fp": 5,
            "fn_count": 3,
            "precision": 0.91,
            "recall": 0.94,
            "f1_score": 0.92,
        },
    ],
}
