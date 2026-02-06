"""Tests for EvaluationScheduler."""

from unittest.mock import AsyncMock

import pytest

from recap_evaluator.config import Settings
from recap_evaluator.scheduler.evaluation_scheduler import EvaluationScheduler


@pytest.fixture
def mock_usecase():
    uc = AsyncMock()
    uc.execute.return_value = AsyncMock(
        evaluation_id="test-id",
        overall_alert_level=AsyncMock(value="ok"),
    )
    return uc


@pytest.fixture
def scheduler_settings():
    return Settings(
        recap_db_dsn="postgres://test:test@localhost/test",
        enable_scheduler=True,
        evaluation_schedule="0 6 * * *",
        evaluation_window_days=7,
    )


class TestEvaluationScheduler:
    def test_start_disabled(self, mock_usecase):
        settings = Settings(
            recap_db_dsn="postgres://test:test@localhost/test",
            enable_scheduler=False,
        )
        scheduler = EvaluationScheduler(mock_usecase, settings)
        scheduler.start()

        assert not scheduler._scheduler.running

    async def test_start_enabled(self, mock_usecase, scheduler_settings):
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)
        scheduler.start()

        assert scheduler._scheduler.running
        scheduler.stop()

    def test_stop_when_not_running(self, mock_usecase, scheduler_settings):
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)
        # Should not raise
        scheduler.stop()

    async def test_run_scheduled_evaluation_calls_usecase(
        self, mock_usecase, scheduler_settings
    ):
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)

        await scheduler._run_scheduled_evaluation()

        mock_usecase.execute.assert_called_once_with(window_days=7)

    async def test_run_scheduled_evaluation_handles_error(
        self, mock_usecase, scheduler_settings
    ):
        mock_usecase.execute.side_effect = Exception("DB down")
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)

        # Should not raise
        await scheduler._run_scheduled_evaluation()
