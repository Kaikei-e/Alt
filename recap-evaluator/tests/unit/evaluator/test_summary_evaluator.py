"""Tests for SummaryEvaluator."""

from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from recap_evaluator.config import AlertThresholds, EvaluatorWeights, Settings
from recap_evaluator.domain.models import AlertLevel, SummaryMetrics
from recap_evaluator.evaluator.summary_evaluator import SummaryEvaluator
from tests.fixtures.job_data import SAMPLE_ARTICLE, SAMPLE_OUTPUT


@pytest.fixture
def mock_rouge():
    rouge = MagicMock()
    rouge.compute_batch.return_value = {
        "rouge_1_f1": 0.45,
        "rouge_2_f1": 0.22,
        "rouge_l_f1": 0.38,
        "num_samples": 1,
    }
    return rouge


@pytest.fixture
def mock_bertscore():
    bs = MagicMock()
    bs.evaluate_batch.return_value = {
        "mean_precision": 0.72,
        "mean_recall": 0.68,
        "mean_f1": 0.70,
        "num_samples": 1,
    }
    return bs


@pytest.fixture
def mock_faithfulness_evaluator():
    fe = MagicMock()
    result = MagicMock()
    result.faithfulness_score = 0.75
    result.hallucination_score = 0.25
    fe.detect_batch.return_value = [result]
    return fe


@pytest.fixture
def summary_evaluator(
    mock_db, mock_ollama, mock_settings, alert_thresholds, evaluator_weights,
    mock_rouge, mock_bertscore, mock_faithfulness_evaluator
):
    return SummaryEvaluator(
        ollama=mock_ollama,
        db=mock_db,
        settings=mock_settings,
        thresholds=alert_thresholds,
        weights=evaluator_weights,
        rouge=mock_rouge,
        bertscore=mock_bertscore,
        faithfulness=mock_faithfulness_evaluator,
    )


class TestSummaryEvaluator:
    async def test_evaluate_batch_returns_metrics(self, summary_evaluator, mock_db):
        mock_db.fetch_outputs.return_value = [SAMPLE_OUTPUT]
        mock_db.fetch_job_articles.return_value = [SAMPLE_ARTICLE]

        from uuid import uuid4
        result = await summary_evaluator.evaluate_batch([uuid4()])

        assert isinstance(result, SummaryMetrics)
        assert result.sample_count > 0

    async def test_evaluate_batch_empty_outputs(self, summary_evaluator, mock_db):
        mock_db.fetch_outputs.return_value = []

        from uuid import uuid4
        result = await summary_evaluator.evaluate_batch([uuid4()])

        assert result.sample_count == 0

    def test_calculate_composite_score(self, summary_evaluator):
        """Composite score is a weighted average (default EvaluatorWeights:
        geval=0.40, bertscore=0.25, faithfulness=0.25, rouge_l=0.10).

        Expected = 0.40*((4.0-1)/4) + 0.25*0.7 + 0.25*0.8 + 0.10*0.4
                 = 0.40*0.75 + 0.175 + 0.2 + 0.04 = 0.715

        A bounds-only check (0 < score < 1) would pass even if a weight were
        silently swapped or the geval normalization dropped, so pin the
        exact value instead.
        """
        metrics = SummaryMetrics(
            geval_overall=4.0,  # Normalized: (4-1)/4 = 0.75
            bertscore_f1=0.7,
            faithfulness_score=0.8,
            rouge_l_f1=0.4,
        )

        score = summary_evaluator._calculate_composite_score(metrics)

        assert score == pytest.approx(0.715, abs=0.001)

    def test_determine_alert_level_ok(self, summary_evaluator):
        metrics = SummaryMetrics(
            coherence=4.5,
            consistency=4.0,
            fluency=4.5,
            relevance=4.0,
            hallucination_rate=0.1,
            overall_quality_score=0.7,
        )

        level = summary_evaluator._determine_alert_level(metrics)

        assert level == AlertLevel.OK

    def test_determine_alert_level_critical_on_high_hallucination(
        self, summary_evaluator
    ):
        metrics = SummaryMetrics(
            coherence=2.5,
            consistency=2.5,
            fluency=4.0,
            relevance=2.5,
            hallucination_rate=0.6,
            overall_quality_score=0.25,
        )

        level = summary_evaluator._determine_alert_level(metrics)

        assert level == AlertLevel.CRITICAL
