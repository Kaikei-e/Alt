"""Characterization tests for GenreLearningService.generate_learning_result."""

from __future__ import annotations

from datetime import UTC, datetime
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from recap_subworker.services.genre_learning import (
    DEFAULT_CLUSTER_GENRES,
    GenreLearningService,
)

DEFAULT_GRAPH_MARGIN = 0.15
ZERO_BOOST_SAMPLE_COUNT = 120


@pytest.mark.asyncio
async def test_generate_learning_result_returns_empty_summary_when_no_rows() -> None:
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = []
    session = AsyncMock()
    session.execute = AsyncMock(return_value=mock_result)

    service = GenreLearningService(session, graph_margin=DEFAULT_GRAPH_MARGIN)
    result = await service.generate_learning_result()

    assert result.summary.total_records == 0
    assert result.summary.graph_margin_reference == DEFAULT_GRAPH_MARGIN
    assert result.entries == []
    assert result.cluster_draft is None


@pytest.mark.asyncio
async def test_generate_learning_result_skips_bayes_when_all_top_boost_zero() -> None:
    job_id = uuid4()
    rows = [
        {
            "job_id": job_id,
            "article_id": f"art-{i}",
            "created_at": datetime.now(UTC),
            "coarse_candidates": [
                {"score": 0.8, "graph_boost": 0.0, "genre": "society_justice"},
            ],
            "refine_decision": {
                "final_genre": "society_justice",
                "strategy": "graph_boost" if i % 2 == 0 else "weighted_score",
                "confidence": 0.85,
            },
            "tag_profile": {
                "top_tags": [{"label": "政治", "confidence": 0.9}],
                "entropy": 1.2,
            },
        }
        for i in range(ZERO_BOOST_SAMPLE_COUNT)
    ]

    snapshot_result = MagicMock()
    snapshot_result.mappings.return_value.all.return_value = rows
    graph_result = MagicMock()
    graph_result.all.return_value = []

    session = AsyncMock()
    session.execute = AsyncMock(side_effect=[snapshot_result, graph_result])

    service = GenreLearningService(
        session,
        graph_margin=DEFAULT_GRAPH_MARGIN,
        bayes_enabled=True,
        bayes_min_samples=100,
    )
    result = await service.generate_learning_result()

    assert result.summary.boost_threshold_reference == 0.0
    assert result.summary.tag_count_threshold_reference == 0
    assert result.summary.accuracy_estimate is None


@pytest.mark.asyncio
async def test_generate_learning_result_auto_detect_genres_fallback_on_error() -> None:
    job_id = uuid4()
    rows = [
        {
            "job_id": job_id,
            "article_id": f"art-{i}",
            "created_at": datetime.now(UTC),
            "coarse_candidates": [
                {"score": 0.8, "graph_boost": 0.1, "genre": DEFAULT_CLUSTER_GENRES[0]},
            ],
            "refine_decision": {
                "final_genre": DEFAULT_CLUSTER_GENRES[0],
                "strategy": "graph_boost",
                "confidence": 0.9,
            },
            "tag_profile": {"top_tags": [{"label": "tag", "confidence": 0.8}], "entropy": 1.0},
        }
        for i in range(12)
    ]

    snapshot_result = MagicMock()
    snapshot_result.mappings.return_value.all.return_value = rows
    graph_result = MagicMock()
    graph_result.all.return_value = []

    session = AsyncMock()
    session.execute = AsyncMock(
        side_effect=[snapshot_result, graph_result, RuntimeError("db error")]
    )

    service = GenreLearningService(
        session,
        graph_margin=DEFAULT_GRAPH_MARGIN,
        auto_detect_genres=True,
        bayes_enabled=False,
    )
    result = await service.generate_learning_result()

    assert result.cluster_draft is not None
    assert result.cluster_draft["genres"][0]["genre"] == DEFAULT_CLUSTER_GENRES[0]
