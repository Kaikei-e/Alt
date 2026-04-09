"""Unit tests for QueryFacet domain model + decompose_queries + render_query_string."""

from __future__ import annotations

from acolyte.domain.query_facet import (
    WEAK_FACET_THRESHOLD,
    QueryFacet,
    decompose_queries,
    render_query_string,
)


def test_query_facet_defaults() -> None:
    """Construct with required fields only; assert sensible defaults."""
    facet = QueryFacet(intent="investigate", raw_query="AI market size")
    assert facet.entities == []
    assert facet.must_have_terms == []
    assert facet.optional_terms == []
    assert facet.time_range is None
    assert facet.source_bias == "article"


def test_query_facet_roundtrip() -> None:
    """model_dump / model_validate roundtrip preserves all fields."""
    original = QueryFacet(
        intent="compare",
        raw_query="NVIDIA vs AMD GPU",
        entities=["NVIDIA", "AMD"],
        must_have_terms=["NVIDIA", "AMD", "GPU"],
        optional_terms=["semiconductor"],
        time_range="2026-Q1",
        source_bias="article",
    )
    dumped = original.model_dump()
    restored = QueryFacet.model_validate(dumped)
    assert restored == original


def test_decompose_single_query() -> None:
    """Single query string decomposes to one facet with correct intent."""
    section = {"section_role": "analysis", "synthesis_only": False}
    brief = {"topic": "AI semiconductor"}

    facets = decompose_queries(["AI market size 2026"], brief, section)

    assert len(facets) == 1
    assert facets[0].raw_query == "AI market size 2026"
    assert facets[0].intent == "investigate"


def test_decompose_with_brief_entities() -> None:
    """Brief entities that appear in query are included in facet entities."""
    section = {"section_role": "analysis", "synthesis_only": False}
    brief = {"topic": "AI chips", "entities": ["NVIDIA", "AMD"]}

    facets = decompose_queries(["NVIDIA GPU market forecast"], brief, section)

    assert len(facets) == 1
    assert "NVIDIA" in facets[0].entities


def test_decompose_with_time_range() -> None:
    """Brief time_range propagated to facet."""
    section = {"section_role": "analysis", "synthesis_only": False}
    brief = {"topic": "AI trends", "time_range": "2026-Q1"}

    facets = decompose_queries(["AI adoption rates"], brief, section)

    assert len(facets) == 1
    assert facets[0].time_range == "2026-Q1"


def test_decompose_synthesis_only_returns_empty() -> None:
    """synthesis_only=True section yields empty facets."""
    section = {"section_role": "conclusion", "synthesis_only": True}
    brief = {"topic": "AI trends"}

    facets = decompose_queries(["AI conclusion"], brief, section)

    assert facets == []


def test_decompose_infers_compare_intent() -> None:
    """Query with comparison keywords should yield 'compare' intent."""
    section = {"section_role": "analysis", "synthesis_only": False}
    brief = {"topic": "AI chips"}

    facets = decompose_queries(["NVIDIA vs AMD performance"], brief, section)

    assert len(facets) == 1
    assert facets[0].intent == "compare"


def test_decompose_infers_trend_intent() -> None:
    """Query with trend keywords should yield 'trend' intent."""
    section = {"section_role": "analysis", "synthesis_only": False}
    brief = {"topic": "AI adoption"}

    facets = decompose_queries(["AI adoption trend forecast"], brief, section)

    assert len(facets) == 1
    assert facets[0].intent == "trend"


def test_render_query_string_basic() -> None:
    """Rendered string includes must_have_terms and entities."""
    facet = QueryFacet(
        intent="investigate",
        raw_query="AI market size",
        must_have_terms=["AI", "market"],
        entities=["NVIDIA"],
    )

    rendered = render_query_string(facet)

    assert "AI" in rendered
    assert "market" in rendered
    assert "NVIDIA" in rendered


def test_render_query_string_caps_length() -> None:
    """Rendered string capped at 120 characters."""
    facet = QueryFacet(
        intent="investigate",
        raw_query="A" * 200,
        must_have_terms=["long"] * 30,
        entities=["entity"] * 20,
    )

    rendered = render_query_string(facet)

    assert len(rendered) <= 120


def test_weak_facet_threshold_is_positive() -> None:
    """WEAK_FACET_THRESHOLD is a positive integer."""
    assert isinstance(WEAK_FACET_THRESHOLD, int)
    assert WEAK_FACET_THRESHOLD > 0
