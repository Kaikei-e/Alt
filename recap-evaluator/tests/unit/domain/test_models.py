"""Tests for domain models."""

from datetime import datetime, timezone
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


class TestAlertLevel:
    def test_values(self):
        assert AlertLevel.OK == "ok"
        assert AlertLevel.WARN == "warn"
        assert AlertLevel.CRITICAL == "critical"


class TestEvaluationType:
    def test_values(self):
        assert EvaluationType.FULL == "full"
        assert EvaluationType.GENRE == "genre"
        assert EvaluationType.SUMMARY == "summary"


class TestMetricValue:
    def test_defaults(self):
        mv = MetricValue(name="test", value=0.5)
        assert mv.alert_level == AlertLevel.OK
        assert mv.threshold_warn is None


class TestGenreMetrics:
    def test_defaults(self):
        gm = GenreMetrics(genre="technology")
        assert gm.precision == 0.0
        assert gm.support == 0


class TestGenreEvaluationResult:
    def test_defaults(self):
        result = GenreEvaluationResult()
        assert result.macro_f1 == 0.0
        assert result.per_genre_metrics == []
        assert result.alert_level == AlertLevel.OK


class TestClusterMetrics:
    def test_external_metrics_default_to_none(self):
        cm = ClusterMetrics()
        assert cm.nmi is None
        assert cm.ari is None
        assert cm.v_measure is None

    def test_with_external_metrics(self):
        cm = ClusterMetrics(nmi=0.8, ari=0.7)
        assert cm.nmi == 0.8
        assert cm.ari == 0.7


class TestSummaryMetrics:
    def test_defaults(self):
        sm = SummaryMetrics()
        assert sm.coherence == 0.0
        assert sm.overall_quality_score == 0.0
        assert sm.sample_count == 0


class TestPipelineMetrics:
    def test_defaults(self):
        pm = PipelineMetrics()
        assert pm.total_jobs == 0
        assert pm.stage_success_rates == {}


class TestEvaluationRun:
    def test_full_construction(self):
        run = EvaluationRun(
            evaluation_id=uuid4(),
            evaluation_type=EvaluationType.FULL,
            job_ids=[uuid4()],
            created_at=datetime(2025, 1, 1, tzinfo=timezone.utc),
            window_days=7,
        )
        assert run.genre_metrics is None
        assert run.overall_alert_level == AlertLevel.OK
