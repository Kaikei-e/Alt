"""Tests for domain models."""

from datetime import UTC, datetime
from uuid import uuid4

from recap_evaluator.domain.models import (
    AlertLevel,
    ClusterMetrics,
    EvaluationRun,
    EvaluationType,
    GenreEvaluationResult,
    GenreMetrics,
    MetricValue,
    PipelineMetrics,
    SummaryMetrics,
)

EXPECTED_NMI = 0.8
EXPECTED_ARI = 0.7
EXPECTED_MACRO_F1 = 0.82
EXPECTED_SILHOUETTE = 0.35
EXPECTED_OVERALL_SCORE = 0.7
EXPECTED_SUCCESS_RATE = 0.95


class TestAlertLevel:
    def test_values(self) -> None:
        assert AlertLevel.OK == "ok"
        assert AlertLevel.WARN == "warn"
        assert AlertLevel.CRITICAL == "critical"


class TestEvaluationType:
    def test_values(self) -> None:
        assert EvaluationType.FULL == "full"
        assert EvaluationType.GENRE == "genre"
        assert EvaluationType.SUMMARY == "summary"


class TestMetricValue:
    def test_defaults(self) -> None:
        mv = MetricValue(name="test", value=0.5)
        assert mv.alert_level == AlertLevel.OK
        assert mv.threshold_warn is None


class TestGenreMetrics:
    def test_defaults(self) -> None:
        gm = GenreMetrics(genre="technology")
        assert gm.precision == 0.0
        assert gm.support == 0


class TestGenreEvaluationResult:
    def test_defaults(self) -> None:
        result = GenreEvaluationResult()
        assert result.macro_f1 == 0.0
        assert result.per_genre_metrics == []
        assert result.alert_level == AlertLevel.OK


class TestClusterMetrics:
    def test_external_metrics_default_to_none(self) -> None:
        cm = ClusterMetrics()
        assert cm.nmi is None
        assert cm.ari is None
        assert cm.v_measure is None

    def test_with_external_metrics(self) -> None:
        cm = ClusterMetrics(nmi=EXPECTED_NMI, ari=EXPECTED_ARI)
        assert cm.nmi == EXPECTED_NMI
        assert cm.ari == EXPECTED_ARI


class TestSummaryMetrics:
    def test_defaults(self) -> None:
        sm = SummaryMetrics()
        assert sm.coherence == 0.0
        assert sm.overall_quality_score == 0.0
        assert sm.sample_count == 0


class TestPipelineMetrics:
    def test_defaults(self) -> None:
        pm = PipelineMetrics()
        assert pm.total_jobs == 0
        assert pm.stage_success_rates == {}


class TestEvaluationRun:
    def test_full_construction(self) -> None:
        run = EvaluationRun(
            evaluation_id=uuid4(),
            evaluation_type=EvaluationType.FULL,
            job_ids=[uuid4()],
            created_at=datetime(2025, 1, 1, tzinfo=UTC),
            window_days=7,
        )
        assert run.genre_metrics is None
        assert run.overall_alert_level == AlertLevel.OK

    def test_to_metrics_dict_empty(self) -> None:
        run = EvaluationRun(
            evaluation_id=uuid4(),
            evaluation_type=EvaluationType.FULL,
            job_ids=[],
            created_at=datetime(2025, 1, 1, tzinfo=UTC),
            window_days=7,
        )
        metrics = run.to_metrics_dict()
        assert metrics["overall_alert_level"] == "ok"
        assert "genre" not in metrics
        assert "summary" not in metrics

    def test_to_metrics_dict_with_all_dimensions(self) -> None:
        run = EvaluationRun(
            evaluation_id=uuid4(),
            evaluation_type=EvaluationType.FULL,
            job_ids=[uuid4()],
            created_at=datetime(2025, 1, 1, tzinfo=UTC),
            window_days=7,
            genre_metrics=GenreEvaluationResult(macro_f1=EXPECTED_MACRO_F1),
            cluster_metrics={
                "tech": ClusterMetrics(silhouette_score=EXPECTED_SILHOUETTE),
            },
            summary_metrics=SummaryMetrics(overall_quality_score=EXPECTED_OVERALL_SCORE),
            pipeline_metrics=PipelineMetrics(success_rate=EXPECTED_SUCCESS_RATE),
            overall_alert_level=AlertLevel.OK,
        )
        metrics = run.to_metrics_dict()
        assert metrics["genre"]["macro_f1"] == EXPECTED_MACRO_F1
        assert metrics["cluster"]["tech"]["silhouette_score"] == EXPECTED_SILHOUETTE
        assert metrics["summary"]["overall_quality_score"] == EXPECTED_OVERALL_SCORE
        assert metrics["pipeline"]["success_rate"] == EXPECTED_SUCCESS_RATE
