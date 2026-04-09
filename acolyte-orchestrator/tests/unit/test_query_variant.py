"""Unit tests for query variant generation.

TDD RED phase: Define expected behavior of generate_query_variants
before implementation exists.
"""

from __future__ import annotations

from acolyte.domain.query_variant import generate_query_variants


def _facet(
    *,
    raw_query: str = "AI market trends",
    must_have_terms: list[str] | None = None,
    entities: list[str] | None = None,
    optional_terms: list[str] | None = None,
) -> dict:
    return {
        "intent": "investigate",
        "raw_query": raw_query,
        "entities": entities or [],
        "must_have_terms": must_have_terms or ["AI", "market", "trends"],
        "optional_terms": optional_terms or [],
        "time_range": None,
        "source_bias": "article",
    }


def _brief(
    topic: str = "AI market analysis",
    entities: list[str] | None = None,
) -> dict:
    return {
        "topic": topic,
        "entities": entities if entities is not None else ["OpenAI", "Google"],
    }


class TestGenerateQueryVariants:
    def test_primary_variant_uses_render_query_string(self) -> None:
        """Primary variant should use render_query_string from the facet."""
        facet = _facet(must_have_terms=["AI", "market", "trends"])
        variants = generate_query_variants(facet, "AI market analysis", _brief())

        # First variant is always "primary" using render_query_string
        primary = [(q, label) for q, label in variants if label == "primary"]
        assert len(primary) == 1
        assert "AI" in primary[0][0]
        assert "market" in primary[0][0]

    def test_broad_variant_includes_entities(self) -> None:
        """Broad variant should include topic + brief entities."""
        facet = _facet()
        brief = _brief(topic="AI market analysis", entities=["OpenAI", "Google"])
        variants = generate_query_variants(facet, "AI market analysis", brief)

        broad = [(q, label) for q, label in variants if label == "broad"]
        assert len(broad) == 1
        query = broad[0][0]
        assert "AI market analysis" in query
        assert "OpenAI" in query or "Google" in query

    def test_narrow_variant_must_have_only(self) -> None:
        """Narrow variant should use only must_have_terms."""
        facet = _facet(
            must_have_terms=["quantum", "computing", "adoption"],
            entities=["IBM"],  # entities make primary differ from narrow
        )
        variants = generate_query_variants(facet, "tech trends", _brief())

        narrow = [(q, label) for q, label in variants if label == "narrow"]
        assert len(narrow) == 1
        query = narrow[0][0]
        assert "quantum" in query
        assert "computing" in query

    def test_narrow_skipped_when_few_terms(self) -> None:
        """Narrow variant should be skipped when must_have has fewer than 2 terms."""
        facet = _facet(must_have_terms=["AI"])
        variants = generate_query_variants(facet, "tech trends", _brief())

        narrow = [label for _, label in variants if label == "narrow"]
        assert len(narrow) == 0

    def test_max_three_variants(self) -> None:
        """Should produce at most 3 variants."""
        facet = _facet(must_have_terms=["AI", "market", "trends"])
        variants = generate_query_variants(facet, "AI market analysis", _brief())

        assert len(variants) <= 3

    def test_always_has_primary(self) -> None:
        """Should always produce at least a primary variant."""
        facet = _facet()
        variants = generate_query_variants(facet, "topic", _brief())

        labels = [label for _, label in variants]
        assert "primary" in labels

    def test_no_entities_skips_broad(self) -> None:
        """Broad variant should be skipped when no entities in brief."""
        facet = _facet()
        brief = _brief(entities=[])
        variants = generate_query_variants(facet, "topic", brief)

        broad = [label for _, label in variants if label == "broad"]
        assert len(broad) == 0
