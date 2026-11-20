"""Integration tests for genre learning flow."""

from __future__ import annotations

import asyncio
from collections.abc import Iterator
from http import HTTPStatus
from typing import Never
from unittest.mock import AsyncMock, MagicMock, patch

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from recap_subworker.app import deps as app_deps
from recap_subworker.app.main import create_app
from recap_subworker.infra.config import Settings
from recap_subworker.services.async_jobs import ConcurrentAdminJobError
from recap_subworker.services.learning_scheduler import LearningScheduler

pytestmark = pytest.mark.usefixtures("stub_startup_sweep")


class _AlreadyRunningJobError(ConcurrentAdminJobError):
    def __init__(self) -> None:
        super().__init__("learning job already running")


@pytest.fixture
def client() -> Iterator[TestClient]:
    """Create a test client with the lifespan active (needed for app.state.container)."""
    app = create_app()
    with TestClient(app) as test_client:
        yield test_client


@pytest.mark.asyncio
async def test_learning_endpoint_integration(client: TestClient) -> None:
    """`/admin/learning` enqueues a background job and returns immediately.

    It delegates to the same AdminJobService as `/admin/learning-jobs`
    instead of running graph rebuild + learning + recap-worker delivery
    synchronously before responding (which contradicted the declared 202).
    """
    response = client.post("/admin/learning")

    assert response.status_code == HTTPStatus.ACCEPTED
    assert response.json() == {"job_id": "stub-job"}


@pytest.mark.asyncio
async def test_learning_endpoint_conflict_when_already_running(client: TestClient) -> None:
    """A concurrent learning job surfaces as 409, not a silently accepted 202."""

    class _ConflictingAdminJobService:
        async def enqueue_learning_job(self) -> Never:
            raise _AlreadyRunningJobError

    app = client.app
    assert isinstance(app, FastAPI)
    app.dependency_overrides[app_deps.get_admin_job_service_dep] = _ConflictingAdminJobService
    try:
        response = client.post("/admin/learning")
    finally:
        del app.dependency_overrides[app_deps.get_admin_job_service_dep]

    assert response.status_code == HTTPStatus.CONFLICT


@pytest.mark.asyncio
async def test_learning_scheduler_integration() -> None:
    """Test that learning scheduler integrates with services correctly."""
    settings = Settings(
        learning_cluster_genres="society_justice,art_culture",
        learning_graph_margin=0.15,
        recap_worker_learning_url="http://localhost:9005/admin/genre-learning",
        learning_request_timeout_seconds=5.0,
        learning_scheduler_enabled=True,
        learning_scheduler_interval_hours=0.01,
    )

    scheduler = LearningScheduler(settings, interval_hours=0.01)

    # Mock all dependencies
    mock_session = AsyncMock()
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = []
    mock_session.execute = AsyncMock(return_value=mock_result)

    # session_factory itself is a plain (sync) callable that returns an
    # async context manager — AsyncMock() would make the call itself async,
    # so `async with session_factory() as session` fails with "'coroutine'
    # object does not support the asynchronous context manager protocol".
    session_ctx = MagicMock()
    session_ctx.__aenter__ = AsyncMock(return_value=mock_session)
    session_ctx.__aexit__ = AsyncMock(return_value=False)
    mock_session_factory = MagicMock(return_value=session_ctx)
    scheduler._db_resources = MagicMock(session_factory=mock_session_factory, aclose=AsyncMock())

    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_client = AsyncMock()
    mock_client.send_learning_payload = AsyncMock(return_value=mock_response)
    mock_client.close = AsyncMock()

    with patch(
        "recap_subworker.services.learning_scheduler.LearningClient.create",
        return_value=mock_client,
    ):
        await scheduler.start()
        await asyncio.sleep(0.1)  # Wait for first execution
        await scheduler.stop()

    mock_session.execute.assert_awaited()
    mock_client.send_learning_payload.assert_awaited()
    mock_client.close.assert_awaited()
