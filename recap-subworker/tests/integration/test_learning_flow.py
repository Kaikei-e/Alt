"""Integration tests for genre learning flow."""

from __future__ import annotations

import asyncio
from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import pytest
from fastapi.testclient import TestClient

from recap_subworker.app.main import create_app


@pytest.fixture
def client():
    """Create a test client."""
    app = create_app()
    return TestClient(app)


@pytest.fixture
def sample_learning_data():
    """Sample data for learning results."""
    job_id = uuid4()
    return [
        {
            "job_id": job_id,
            "article_id": f"article-{i}",
            "created_at": datetime.now(timezone.utc),
            "coarse_candidates": [
                {"score": 0.8, "graph_boost": 0.2, "genre": "society_justice"},
                {"score": 0.6, "graph_boost": 0.1, "genre": "art_culture"},
            ],
            "refine_decision": {
                "final_genre": "society_justice",
                "strategy": "graph_boost" if i % 2 == 0 else "weighted_score",
                "confidence": 0.85,
            },
            "tag_profile": {
                "top_tags": [
                    {"label": "政治", "confidence": 0.9},
                    {"label": "社会", "confidence": 0.8},
                ],
                "entropy": 1.5,
            },
        }
        for i in range(20)
    ]


@pytest.mark.asyncio
async def test_learning_endpoint_integration(client, sample_learning_data):
    """Test the /admin/learning endpoint end-to-end."""
    # Mock database session
    mock_session = AsyncMock()
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = [
        MagicMock(**{k: v for k, v in row.items() if k != "coarse_candidates"})
        for row in sample_learning_data
    ]
    mock_session.execute = AsyncMock(return_value=mock_result)

    # Mock HTTP client for recap-worker
    mock_response = MagicMock()
    mock_response.status_code = 200
    mock_response.headers = {"content-type": "application/json"}
    mock_response.json.return_value = {
        "status": "success",
        "config_saved": True,
        "message": "configuration saved successfully",
    }
    mock_client = AsyncMock()
    mock_client.send_learning_payload = AsyncMock(return_value=mock_response)
    mock_client.close = AsyncMock()

    with patch(
        "recap_subworker.app.deps.get_session",
        return_value=mock_session,
    ), patch(
        "recap_subworker.app.deps.get_learning_client",
        return_value=mock_client,
    ):
        response = client.post("/admin/learning")

    assert response.status_code == 202
    data = response.json()
    assert data["status"] == "sent"
    assert data["recap_worker_status"] == 200
    assert "recap_worker_response" in data


@pytest.mark.asyncio
async def test_learning_endpoint_handles_http_errors(client, sample_learning_data):
    """Test that learning endpoint handles HTTP errors from recap-worker."""
    mock_session = AsyncMock()
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = []
    mock_session.execute = AsyncMock(return_value=mock_result)

    # Mock HTTP client that raises an error
    mock_client = AsyncMock()
    mock_client.send_learning_payload = AsyncMock(side_effect=Exception("Connection error"))
    mock_client.close = AsyncMock()

    with patch(
        "recap_subworker.app.deps.get_session",
        return_value=mock_session,
    ), patch(
        "recap_subworker.app.deps.get_learning_client",
        return_value=mock_client,
    ):
        response = client.post("/admin/learning")

    assert response.status_code == 502
    assert "failed to send learning payload" in response.json()["detail"]


@pytest.mark.asyncio
async def test_learning_scheduler_integration():
    """Test that learning scheduler integrates with services correctly."""
    from recap_subworker.infra.config import Settings
    from recap_subworker.services.learning_scheduler import LearningScheduler

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

    mock_session_factory = AsyncMock()
    mock_session_factory.return_value.__aenter__.return_value = mock_session
    mock_session_factory.return_value.__aexit__.return_value = False

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
        await scheduler.start()
        await asyncio.sleep(0.1)  # Wait for first execution
        await scheduler.stop()

    mock_session.execute.assert_awaited()
    mock_client.send_learning_payload.assert_awaited()
    mock_client.close.assert_awaited()

