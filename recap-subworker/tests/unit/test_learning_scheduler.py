"""Unit tests for LearningScheduler."""

from __future__ import annotations

import asyncio
from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import pytest

from recap_subworker.infra.config import Settings
from recap_subworker.services.learning_scheduler import LearningScheduler


@pytest.fixture
def mock_settings():
    """Create a mock Settings object."""
    settings = MagicMock(spec=Settings)
    settings.learning_cluster_genres = "society_justice,art_culture"
    settings.learning_graph_margin = 0.15
    settings.recap_worker_learning_url = "http://localhost:9005/admin/genre-learning"
    settings.learning_request_timeout_seconds = 5.0
    return settings


@pytest.fixture
def scheduler(mock_settings):
    """Create a LearningScheduler instance."""
    return LearningScheduler(mock_settings, interval_hours=0.01)  # Short interval for testing


@pytest.mark.asyncio
async def test_scheduler_start_stop(scheduler):
    """Test starting and stopping the scheduler."""
    assert not scheduler._running
    assert scheduler._task is None

    await scheduler.start()
    assert scheduler._running
    assert scheduler._task is not None

    await scheduler.stop()
    assert not scheduler._running


@pytest.mark.asyncio
async def test_scheduler_start_idempotent(scheduler):
    """Test that starting scheduler multiple times is idempotent."""
    await scheduler.start()
    task1 = scheduler._task

    await scheduler.start()  # Should not create a new task
    assert scheduler._task is task1

    await scheduler.stop()


@pytest.mark.asyncio
async def test_scheduler_execute_learning_success(mock_settings):
    """Test successful execution of learning task."""
    scheduler = LearningScheduler(mock_settings, interval_hours=0.01)

    # Mock session factory
    mock_session = AsyncMock()
    mock_session_factory = AsyncMock()
    mock_session_factory.return_value.__aenter__.return_value = mock_session
    mock_session_factory.return_value.__aexit__.return_value = False

    # Mock database query result
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = []
    mock_session.execute = AsyncMock(return_value=mock_result)

    # Mock HTTP client
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_client = AsyncMock()
    mock_client.send_learning_payload = AsyncMock(return_value=mock_response)
    mock_client.close = AsyncMock()

    with patch(
        "recap_subworker.services.learning_scheduler.get_session_factory",
        return_value=mock_session_factory,
    ), patch(
        "recap_subworker.services.learning_scheduler.LearningClient.create",
        return_value=mock_client,
    ):
        await scheduler._execute_learning()

    mock_session.execute.assert_awaited()
    mock_client.send_learning_payload.assert_awaited_once()
    mock_client.close.assert_awaited_once()


@pytest.mark.asyncio
async def test_scheduler_execute_learning_handles_errors(mock_settings):
    """Test that scheduler handles errors gracefully."""
    scheduler = LearningScheduler(mock_settings, interval_hours=0.01)

    # Mock session factory that raises an error
    mock_session_factory = AsyncMock()
    mock_session_factory.side_effect = Exception("Database error")

    with patch(
        "recap_subworker.services.learning_scheduler.get_session_factory",
        return_value=mock_session_factory,
    ):
        # Should not raise, but log the error
        await scheduler._execute_learning()


@pytest.mark.asyncio
async def test_scheduler_run_loop_executes_periodically(mock_settings):
    """Test that scheduler loop executes tasks periodically."""
    scheduler = LearningScheduler(mock_settings, interval_hours=0.01)
    execution_count = 0

    async def mock_execute():
        nonlocal execution_count
        execution_count += 1
        if execution_count >= 2:
            scheduler._running = False  # Stop after 2 executions

    scheduler._execute_learning = mock_execute
    scheduler._running = True

    await scheduler._run_loop()

    assert execution_count == 2


@pytest.mark.asyncio
async def test_scheduler_build_learning_payload(mock_settings):
    """Test building learning payload from result."""
    scheduler = LearningScheduler(mock_settings, interval_hours=0.01)

    from recap_subworker.services.genre_learning import (
        GenreLearningResult,
        GenreLearningSummary,
    )

    summary = GenreLearningSummary(
        total_records=10,
        graph_boost_count=7,
        graph_boost_percentage=70.0,
        avg_margin=0.18,
        avg_top_boost=0.12,
        avg_confidence=0.85,
        tag_coverage_pct=90.0,
        graph_margin_reference=0.15,
    )

    result = GenreLearningResult(
        summary=summary,
        entries=[],
        cluster_draft=None,
    )

    payload = scheduler._build_learning_payload(result)

    assert payload["summary"]["total_records"] == 10
    assert payload["graph_override"]["graph_margin"] == 0.15
    assert "metadata" in payload
    assert "captured_at" in payload["metadata"]


@pytest.mark.asyncio
async def test_scheduler_cancellation(mock_settings):
    """Test that scheduler handles cancellation gracefully."""
    scheduler = LearningScheduler(mock_settings, interval_hours=0.01)

    await scheduler.start()
    assert scheduler._running

    # Cancel the task
    await scheduler.stop()

    # Wait a bit to ensure cancellation is handled
    await asyncio.sleep(0.1)

    assert not scheduler._running



