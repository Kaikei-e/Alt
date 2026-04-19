"""Tests for HyDE integration into GathererNode."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import pytest

from acolyte.usecase.graph.nodes.gatherer_node import GathererNode, _detect_topic_language

# ---- helpers ------------------------------------------------------------


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
        return [_Article(article_id=f"a-{len(self.search_calls)}", title=f"hit for {query[:20]}", tags=[], score=1.0)]

    async def search_recaps(self, query: str, limit: int = 10) -> list[Any]:  # noqa: ARG002
        return []


class _FakeHyDE:
    def __init__(self, doc: str | None = "English HyDE passage " * 10) -> None:
        self._doc = doc
        self.calls: list[tuple[str, str]] = []

    async def generate_hypothetical_doc(self, topic: str, target_lang: str) -> str | None:
        self.calls.append((topic, target_lang))
        return self._doc


# ---- language detector --------------------------------------------------


class TestDetectTopicLanguage:
    @pytest.mark.parametrize(
        ("topic", "expected"),
        [
            ("", "und"),
            ("ab", "und"),
            ("Middle East tensions 2026", "en"),
            ("イラン情勢 分析レポート", "ja"),
            ("JD Vance 副大統領", "ja"),
            ("AI chips 2026 チップ市場", "ja"),
        ],
    )
    def test_cases(self, topic: str, expected: str) -> None:
        assert _detect_topic_language(topic) == expected


# ---- gatherer integration ----------------------------------------------


def _minimal_outline() -> list[dict]:
    return [
        {
            "key": "analysis",
            "query_facets": [
                {"raw_query": "AIチップ市場 2026", "must_have_terms": ["AI", "chip"]},
            ],
        }
    ]


@pytest.mark.asyncio
async def test_gatherer_requests_hyde_en_for_japanese_topic():
    evidence = _FakeEvidenceProvider()
    hyde = _FakeHyDE()
    node = GathererNode(evidence, hyde_generator=hyde)  # type: ignore[arg-type]
    state = {
        "brief": {"topic": "イラン情勢 分析レポート 2026"},
        "outline": _minimal_outline(),
    }
    await node(state)
    assert len(hyde.calls) == 1
    assert hyde.calls[0][1] == "en"


@pytest.mark.asyncio
async def test_gatherer_requests_hyde_ja_for_english_topic():
    evidence = _FakeEvidenceProvider()
    hyde = _FakeHyDE(doc="日本語のHyDEパッセージです。" * 5)
    node = GathererNode(evidence, hyde_generator=hyde)  # type: ignore[arg-type]
    state = {
        "brief": {"topic": "GPU shortage impact on AI training"},
        "outline": _minimal_outline(),
    }
    await node(state)
    assert len(hyde.calls) == 1
    assert hyde.calls[0][1] == "ja"


@pytest.mark.asyncio
async def test_gatherer_skips_hyde_when_generator_absent():
    evidence = _FakeEvidenceProvider()
    node = GathererNode(evidence)  # type: ignore[arg-type]  # no hyde_generator
    state = {
        "brief": {"topic": "イラン情勢 2026"},
        "outline": _minimal_outline(),
    }
    result = await node(state)
    # Only primary + narrow variants hit search (broad is skipped when entities is empty).
    # The key assertion: no HyDE request was made.
    assert "evidence" in result
    assert all("hyde" not in q for q in evidence.search_calls)


@pytest.mark.asyncio
async def test_gatherer_adds_hyde_variant_to_search_calls():
    evidence = _FakeEvidenceProvider()
    hyde_doc = "English HyDE passage " * 10
    hyde = _FakeHyDE(doc=hyde_doc)
    node = GathererNode(evidence, hyde_generator=hyde)  # type: ignore[arg-type]
    state = {
        "brief": {"topic": "イラン情勢 2026"},
        "outline": _minimal_outline(),
    }
    await node(state)
    # HyDE doc should appear in the search_articles calls as one of the variants.
    assert any(hyde_doc in q for q in evidence.search_calls)


@pytest.mark.asyncio
async def test_gatherer_continues_when_hyde_returns_none():
    evidence = _FakeEvidenceProvider()
    hyde = _FakeHyDE(doc=None)
    node = GathererNode(evidence, hyde_generator=hyde)  # type: ignore[arg-type]
    state = {
        "brief": {"topic": "イラン情勢 2026"},
        "outline": _minimal_outline(),
    }
    result = await node(state)
    assert "evidence" in result
    # HyDE was consulted once, but no extra variant was queued.
    assert len(hyde.calls) == 1


@pytest.mark.asyncio
async def test_gatherer_does_not_request_hyde_for_und_topic():
    evidence = _FakeEvidenceProvider()
    hyde = _FakeHyDE()
    node = GathererNode(evidence, hyde_generator=hyde)  # type: ignore[arg-type]
    state = {
        "brief": {"topic": "a b"},  # detector returns "und"
        "outline": _minimal_outline(),
    }
    await node(state)
    assert hyde.calls == []
