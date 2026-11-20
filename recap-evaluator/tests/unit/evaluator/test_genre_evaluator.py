"""Tests for GenreEvaluator."""

from unittest.mock import AsyncMock

import pytest

from recap_evaluator.config import AlertThresholds
from recap_evaluator.domain.models import AlertLevel
from recap_evaluator.evaluator.genre_evaluator import GenreEvaluator
from tests.fixtures.evaluation_data import SAMPLE_GENRE_API_RESPONSE

EXPECTED_MACRO_F1 = 0.82
EXPECTED_TOTAL_SAMPLES = 88


@pytest.fixture
def genre_evaluator(
    mock_db: AsyncMock,
    mock_recap_worker: AsyncMock,
    alert_thresholds: AlertThresholds,
) -> GenreEvaluator:
    return GenreEvaluator(mock_recap_worker, mock_db, alert_thresholds)


class TestGenreEvaluator:
    async def test_fetch_latest_evaluation_returns_result(
        self, genre_evaluator: GenreEvaluator, mock_recap_worker: AsyncMock
    ) -> None:
        mock_recap_worker.fetch_latest_genre_evaluation.return_value = SAMPLE_GENRE_API_RESPONSE

        result = await genre_evaluator.fetch_latest_evaluation()

        assert result is not None
        assert result.macro_f1 == EXPECTED_MACRO_F1
        assert result.total_samples == EXPECTED_TOTAL_SAMPLES
        assert len(result.per_genre_metrics) == 1

    async def test_fetch_latest_evaluation_returns_none_on_error(
        self, genre_evaluator: GenreEvaluator, mock_recap_worker: AsyncMock
    ) -> None:
        mock_recap_worker.fetch_latest_genre_evaluation.return_value = None

        result = await genre_evaluator.fetch_latest_evaluation()

        assert result is None

    async def test_alert_level_ok_when_above_threshold(
        self, genre_evaluator: GenreEvaluator, mock_recap_worker: AsyncMock
    ) -> None:
        mock_recap_worker.fetch_latest_genre_evaluation.return_value = {
            **SAMPLE_GENRE_API_RESPONSE,
            "macro_f1": 0.85,
        }

        result = await genre_evaluator.fetch_latest_evaluation()

        assert result is not None
        assert result.alert_level == AlertLevel.OK

    async def test_alert_level_warn_when_below_warn_threshold(
        self, genre_evaluator: GenreEvaluator, mock_recap_worker: AsyncMock
    ) -> None:
        mock_recap_worker.fetch_latest_genre_evaluation.return_value = {
            **SAMPLE_GENRE_API_RESPONSE,
            "macro_f1": 0.65,
        }

        result = await genre_evaluator.fetch_latest_evaluation()

        assert result is not None
        assert result.alert_level == AlertLevel.WARN

    async def test_alert_level_critical_when_below_critical_threshold(
        self, genre_evaluator: GenreEvaluator, mock_recap_worker: AsyncMock
    ) -> None:
        mock_recap_worker.fetch_latest_genre_evaluation.return_value = {
            **SAMPLE_GENRE_API_RESPONSE,
            "macro_f1": 0.55,
        }

        result = await genre_evaluator.fetch_latest_evaluation()

        assert result is not None
        assert result.alert_level == AlertLevel.CRITICAL

    async def test_trigger_evaluation_delegates_to_recap_worker(
        self, genre_evaluator: GenreEvaluator, mock_recap_worker: AsyncMock
    ) -> None:
        await genre_evaluator.trigger_evaluation()

        mock_recap_worker.trigger_genre_evaluation.assert_called_once()

    async def test_analyze_learning_results_empty(
        self, genre_evaluator: GenreEvaluator, mock_db: AsyncMock
    ) -> None:
        mock_db.fetch_genre_learning_results.return_value = []

        result = await genre_evaluator.analyze_learning_results([])

        assert result == {"total_articles": 0}
