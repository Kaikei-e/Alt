"""HyDE cross-lingual expansion must also fire on the legacy
``_search_by_queries`` path so reports without query_facets still get
cross-lingual recall (ADR-000695 parity for Acolyte)."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, cast

import pytest
from structlog.testing import capture_logs

from acolyte.usecase.graph.nodes.gatherer_node import GathererNode
from acolyte.usecase.graph.state import ReportGenerationState


@dataclass
class _Article:
    article_id: str
    title: str
    tags: list[str]
    score: float


class _FakeEvidenceProvider:
    def __init__(self) -> None:
        self.search_calls: list[str] = []

    async def search_articles(self, query: str, limit: int = 10) -> list[_Article]:  # noqa: ARG002
        self.search_calls.append(query)
        return [
            _Article(
                article_id=f"a-{len(self.search_calls)}",
                title=f"hit for {query[:20]}",
                tags=[],
                score=1.0,
            )
        ]

    async def search_recaps(self, query: str, limit: int = 10) -> list[Any]:  # noqa: ARG002
        _ = (query, limit)
        return []


class _FakeHyDE:
    def __init__(self, doc: str | None = "English HyDE passage " * 10) -> None:
        self._doc = doc
        self.calls: list[tuple[str, str]] = []

    async def generate_hypothetical_doc(self, topic: str, target_lang: str) -> str | None:
        self.calls.append((topic, target_lang))
        return self._doc


def _outline_without_facets() -> list[dict]:
    """Legacy outline: has ``search_queries`` but no ``query_facets``."""
    return [
        {
            "key": "analysis",
            "search_queries": ["イラン情勢 軍事"],
        }
    ]


@pytest.mark.asyncio
async def test_gatherer_applies_hyde_on_legacy_path() -> None:
    evidence = _FakeEvidenceProvider()
    hyde_doc = "English HyDE passage about Iran " * 5
    hyde = _FakeHyDE(doc=hyde_doc)

    node = GathererNode(evidence, hyde_generator=hyde)  # type: ignore[arg-type]
    state = cast(
        ReportGenerationState,
        {
            "brief": {"topic": "イラン情勢 分析 2026"},
            "outline": _outline_without_facets(),
        },
    )

    await node(state)

    assert len(hyde.calls) == 1, "HyDE must be invoked on the legacy path"
    assert hyde.calls[0][1] == "en", "Japanese topic must request an English HyDE passage"
    assert any(hyde_doc in q for q in evidence.search_calls), (
        "HyDE passage must be queued as a search variant on the legacy path"
    )


@pytest.mark.asyncio
async def test_gatherer_logs_warning_when_hyde_not_wired() -> None:
    evidence = _FakeEvidenceProvider()
    node = GathererNode(evidence)  # type: ignore[arg-type]  # hyde_generator=None
    state = cast(
        ReportGenerationState,
        {
            "brief": {"topic": "イラン情勢 分析 2026"},
            "outline": [
                {
                    "key": "analysis",
                    "query_facets": [
                        {"raw_query": "イラン情勢 軍事", "must_have_terms": ["イラン"]},
                    ],
                }
            ],
        },
    )

    with capture_logs() as logs:
        await node(state)

    messages = " ".join(entry.get("event", "") for entry in logs).lower()
    assert "hyde" in messages or "cross-lingual" in messages, (
        f"missing HyDE wiring must surface as a warning for ops visibility, got logs={logs!r}"
    )


@pytest.mark.asyncio
async def test_gatherer_logs_hyde_warning_once_per_call() -> None:
    """Two sections share the same GathererNode call — warning should only
    appear once, not per-facet, per-section, or per-variant."""
    evidence = _FakeEvidenceProvider()
    node = GathererNode(evidence)  # type: ignore[arg-type]
    state = cast(
        ReportGenerationState,
        {
            "brief": {"topic": "イラン情勢 2026"},
            "outline": [
                {
                    "key": "analysis",
                    "query_facets": [
                        {"raw_query": "イラン情勢 軍事", "must_have_terms": []},
                        {"raw_query": "イラン情勢 外交", "must_have_terms": []},
                    ],
                },
                {
                    "key": "executive_summary",
                    "query_facets": [
                        {"raw_query": "イラン情勢 概観", "must_have_terms": []},
                    ],
                },
            ],
        },
    )

    with capture_logs() as logs:
        await node(state)

    hyde_warnings = [entry for entry in logs if "hyde" in entry.get("event", "").lower()]
    assert len(hyde_warnings) <= 1, (
        f"HyDE warning must be rate-limited to at most once per call, got {len(hyde_warnings)}"
    )
