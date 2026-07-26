"""Tests for EvaluationScheduler."""

from unittest.mock import AsyncMock

import pytest
import structlog.testing

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
        """Idempotent no-op: stopping a scheduler that never started must not
        raise, and the underlying APScheduler must remain not-running."""
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)

        scheduler.stop()

        assert not scheduler._scheduler.running

    async def test_run_scheduled_evaluation_calls_usecase(
        self, mock_usecase, scheduler_settings
    ):
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)

        await scheduler._run_scheduled_evaluation()

        mock_usecase.execute.assert_called_once_with(window_days=7)

    async def test_run_scheduled_evaluation_handles_error(
        self, mock_usecase, scheduler_settings
    ):
        """A failed evaluation run must be logged (not silently dropped) and
        must not propagate out of the scheduled job — an uncaught exception
        here would kill the APScheduler job permanently (CLAUDE.md rule 8:
        failures must surface, not vanish)."""
        mock_usecase.execute.side_effect = Exception("DB down")
        scheduler = EvaluationScheduler(mock_usecase, scheduler_settings)

        with structlog.testing.capture_logs() as logs:
            await scheduler._run_scheduled_evaluation()  # must not raise

        mock_usecase.execute.assert_called_once_with(window_days=7)
        assert any(
            entry["event"] == "Scheduled evaluation failed"
            and entry["log_level"] == "error"
            for entry in logs
        )
