"""Unit tests for GenreLearningService."""

from __future__ import annotations

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4

import pytest

from recap_subworker.services.genre_learning import (
    ClusterBuilder,
    GenreLearningService,
    GenreLearningSummary,
    build_graph_boost_snapshot_entries,
)


@pytest.fixture
def mock_session():
    """Create a mock async session."""
    session = AsyncMock()
    return session


@pytest.fixture
def sample_rows():
    """Sample rows from recap_genre_learning_results."""
    job_id = uuid4()
    return [
        {
            "job_id": job_id,
            "article_id": "article-1",
            "created_at": datetime.now(timezone.utc),
            "coarse_candidates": [
                {"score": 0.8, "graph_boost": 0.2, "genre": "society_justice"},
                {"score": 0.6, "graph_boost": 0.1, "genre": "art_culture"},
            ],
            "refine_decision": {
                "final_genre": "society_justice",
                "strategy": "graph_boost",
                "confidence": 0.85,
            },
            "tag_profile": {
                "top_tags": [
                    {"label": "政治", "confidence": 0.9},
                    {"label": "社会", "confidence": 0.8},
                ],
                "entropy": 1.5,
            },
        },
        {
            "job_id": job_id,
            "article_id": "article-2",
            "created_at": datetime.now(timezone.utc),
            "coarse_candidates": [
                {"score": 0.7, "graph_boost": 0.05, "genre": "society_justice"},
                {"score": 0.65, "graph_boost": 0.0, "genre": "art_culture"},
            ],
            "refine_decision": {
                "final_genre": "society_justice",
                "strategy": "weighted_score",
                "confidence": 0.75,
            },
            "tag_profile": {
                "top_tags": [{"label": "経済", "confidence": 0.7}],
                "entropy": 0.8,
            },
        },
    ]


@pytest.mark.asyncio
async def test_fetch_snapshot_rows(mock_session, sample_rows):
    """Test fetching snapshot rows from database."""
    # Mock the database query result
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = [
        MagicMock(**{k: v for k, v in row.items() if k != "coarse_candidates"})
        for row in sample_rows
    ]
    mock_execute = AsyncMock(return_value=mock_result)
    mock_session.execute = mock_execute

    service = GenreLearningService(mock_session, graph_margin=0.15)
    rows = await service.fetch_snapshot_rows(hours=24, limit=100)

    assert len(rows) == 2
    mock_execute.assert_awaited_once()


@pytest.mark.asyncio
async def test_generate_learning_result(mock_session, sample_rows):
    """Test generating a complete learning result."""
    # Mock database query
    mock_result = MagicMock()
    mock_result.mappings.return_value.all.return_value = [
        MagicMock(**{k: v for k, v in row.items() if k != "coarse_candidates"})
        for row in sample_rows
    ]
    mock_execute = AsyncMock(return_value=mock_result)
    mock_session.execute = mock_execute

    service = GenreLearningService(
        mock_session,
        graph_margin=0.15,
        cluster_genres=["society_justice", "art_culture"],
    )
    result = await service.generate_learning_result()

    assert result.summary.total_records == 2
    assert result.summary.graph_boost_count == 1
    assert result.summary.graph_boost_percentage == 50.0
    assert len(result.entries) == 2
    assert result.entries[0]["strategy"] == "graph_boost"
    assert result.entries[1]["strategy"] == "weighted_score"


def test_build_graph_boost_snapshot_entries(sample_rows):
    """Test building snapshot entries from raw rows."""
    entries = build_graph_boost_snapshot_entries(sample_rows, graph_margin=0.15)

    assert len(entries) == 2
    assert entries[0]["job_id"] == str(sample_rows[0]["job_id"])
    assert entries[0]["article_id"] == "article-1"
    assert entries[0]["strategy"] == "graph_boost"
    assert entries[0]["margin"] > 0
    assert entries[0]["top_boost"] == 0.2
    assert entries[0]["tag_count"] == 2
    assert entries[0]["graph_boost_available"] is True

    assert entries[1]["strategy"] == "weighted_score"
    assert entries[1]["tag_count"] == 1


def test_cluster_builder_builds_clusters():
    """Test ClusterBuilder generates cluster drafts."""
    entries = [
        {
            "final_genre": "society_justice",
            "margin": 0.2,
            "top_boost": 0.15,
            "tag_count": 3,
            "candidate_count": 2,
            "tag_entropy": 1.5,
            "graph_boost_available": True,
            "top_tags": ["政治", "社会"],
        }
        for _ in range(15)  # Enough samples for clustering
    ]

    builder = ClusterBuilder(max_clusters=4, random_state=42)
    draft = builder.build(entries, genres=["society_justice"], min_samples=10)

    assert draft is not None
    assert draft["draft_id"].startswith("graph-boost-reorg-")
    assert len(draft["genres"]) == 1
    assert draft["genres"][0]["genre"] == "society_justice"
    assert draft["genres"][0]["cluster_count"] > 0


def test_cluster_builder_returns_none_for_insufficient_samples():
    """Test ClusterBuilder returns None when samples are insufficient."""
    entries = [
        {
            "final_genre": "society_justice",
            "margin": 0.2,
            "top_boost": 0.15,
            "tag_count": 3,
            "candidate_count": 2,
            "tag_entropy": 1.5,
            "graph_boost_available": True,
            "top_tags": ["政治"],
        }
        for _ in range(5)  # Not enough samples
    ]

    builder = ClusterBuilder(max_clusters=4, random_state=42)
    draft = builder.build(entries, genres=["society_justice"], min_samples=10)

    assert draft is None


def test_summarize_entries():
    """Test summarizing entries into GenreLearningSummary."""
    entries = [
        {
            "strategy": "graph_boost",
            "margin": 0.2,
            "top_boost": 0.15,
            "tag_count": 3,
            "confidence": 0.85,
        },
        {
            "strategy": "weighted_score",
            "margin": 0.1,
            "top_boost": 0.0,
            "tag_count": 1,
            "confidence": 0.75,
        },
        {
            "strategy": "graph_boost",
            "margin": 0.18,
            "top_boost": 0.12,
            "tag_count": 2,
            "confidence": 0.8,
        },
    ]

    service = GenreLearningService(AsyncMock(), graph_margin=0.15)
    summary = service._summarize_entries(entries)

    assert summary.total_records == 3
    assert summary.graph_boost_count == 2
    assert summary.graph_boost_percentage == pytest.approx(66.67, abs=0.01)
    assert summary.avg_margin is not None
    assert summary.avg_top_boost is not None
    assert summary.avg_confidence is not None
    assert summary.tag_coverage_pct == 100.0
    assert summary.graph_margin_reference == 0.15

