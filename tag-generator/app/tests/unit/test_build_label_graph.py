from __future__ import annotations

from datetime import UTC, datetime

import pytest

from scripts import build_label_graph as blg


def _ts(hour: int) -> datetime:
    return datetime(2025, 11, 12, hour, 0, 0, tzinfo=UTC)


def test_aggregate_tag_edges_basic() -> None:
    rows = [
        blg.LearningRow(
            genre="Tech",
            tags=[
                {"label": "AI", "confidence": 0.9},
                {"label": "Robotics", "confidence": 0.55},
            ],
            updated_at=_ts(1),
        ),
        blg.LearningRow(
            genre="tech ",
            tags=[{"label": "AI", "confidence": 0.7}],
            updated_at=_ts(2),
        ),
    ]

    edges = blg.aggregate_tag_edges(
        rows,
        max_tags=3,
        min_confidence=0.5,
        min_support=1,
    )

    assert len(edges) == 2
    ai_edge = next(e for e in edges if e.tag == "ai")
    assert ai_edge.genre == "tech"
    assert ai_edge.sample_size == 2
    assert ai_edge.last_observed_at == _ts(2)
    assert ai_edge.weight == pytest.approx((0.9 + 0.7) / 2, abs=1e-6)


def test_aggregate_tag_edges_respects_thresholds() -> None:
    rows = [
        blg.LearningRow(
            genre="Business",
            tags=[
                {"label": "Funding", "confidence": 0.4},
                {"label": "Startups", "confidence": 0.61},
            ],
            updated_at=_ts(3),
        )
    ]

    edges = blg.aggregate_tag_edges(
        rows,
        max_tags=1,
        min_confidence=0.3,
        min_support=1,
    )

    assert len(edges) == 1
    assert edges[0].tag == "funding"
    # `max_tags=1` prevents the higher-confidence "Startups" entry from being recorded.


def test_aggregate_tag_edges_filters_low_confidence() -> None:
    rows = [
        blg.LearningRow(
            genre="AI",
            tags=[{"label": "LLM", "confidence": 0.2}],
            updated_at=_ts(4),
        )
    ]

    edges = blg.aggregate_tag_edges(
        rows,
        max_tags=5,
        min_confidence=0.5,
        min_support=1,
    )

    assert edges == []


def test_aggregate_tag_edges_applies_min_support() -> None:
    rows = [
        blg.LearningRow(
            genre="AI",
            tags=[{"label": "LLM", "confidence": 0.8}],
            updated_at=_ts(5),
        )
    ]

    edges = blg.aggregate_tag_edges(
        rows,
        max_tags=5,
        min_confidence=0.1,
        min_support=2,
    )

    assert edges == []
