"""Gatherer must translate ``brief.time_range`` into a published_after
bound so retrieval cannot surface articles from outside the requested
window. Mirrors the ``weekly_briefing`` default P7D established in
ReportBrief.from_scope."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import UTC, datetime, timedelta
from typing import Any, cast

import pytest

from acolyte.usecase.graph.nodes.gatherer_node import GathererNode
from acolyte.usecase.graph.state import ReportGenerationState


@dataclass
class _Article:
    article_id: str
    title: str
    tags: list[str]
    score: float


class _RecordingEvidence:
    def __init__(self) -> None:
        self.calls: list[dict] = []

    async def search_articles(
        self,
        query: str,
        *,
        limit: int = 10,
        published_after: datetime | None = None,
        published_before: datetime | None = None,
    ) -> list[_Article]:
        self.calls.append(
            {
                "query": query,
                "limit": limit,
                "published_after": published_after,
                "published_before": published_before,
            }
        )
        return []

    async def search_recaps(self, query: str, limit: int = 10) -> list[Any]:
        _ = (query, limit)
        return []


def _outline_with_facets() -> list[dict]:
    return [
        {
            "key": "analysis",
            "query_facets": [
                {"raw_query": "イラン情勢 軍事", "must_have_terms": ["イラン"]},
            ],
        }
    ]


@pytest.mark.asyncio
async def test_gatherer_forwards_weekly_window_as_published_after() -> None:
    evidence = _RecordingEvidence()
    node = GathererNode(evidence)  # type: ignore[arg-type]
    state = cast(
        ReportGenerationState,
        {
            "brief": {
                "topic": "イラン情勢 2026",
                "time_range": "P7D",
                "report_type": "weekly_briefing",
            },
            "outline": _outline_with_facets(),
        },
    )

    before_call = datetime.now(UTC)
    await node(state)
    after_call = datetime.now(UTC)

    assert evidence.calls, "gatherer must issue at least one search_articles call"
    for call in evidence.calls:
        pa = call["published_after"]
        assert pa is not None, "weekly_briefing time_range must surface as a published_after bound"
        expected_low = before_call - timedelta(days=7, seconds=1)
        expected_high = after_call - timedelta(days=7)
        assert expected_low <= pa <= expected_high, f"published_after must be ~now-7d; got {pa.isoformat()}"


@pytest.mark.asyncio
async def test_gatherer_omits_date_window_when_time_range_absent() -> None:
    evidence = _RecordingEvidence()
    node = GathererNode(evidence)  # type: ignore[arg-type]
    state = cast(
        ReportGenerationState,
        {
            "brief": {"topic": "AI semiconductor", "report_type": "market_analysis"},
            "outline": _outline_with_facets(),
        },
    )

    await node(state)

    assert evidence.calls
    for call in evidence.calls:
        assert call["published_after"] is None
        assert call["published_before"] is None
