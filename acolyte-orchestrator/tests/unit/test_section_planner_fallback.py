"""Unit tests for SectionPlannerNode deterministic fallback (Issue 4).

When LLM fails or returns empty claims for conclusion/ES, deterministic
fallback creates synthesis claims from analysis claim_plans or extracted_facts.
"""

from __future__ import annotations

from acolyte.usecase.graph.nodes.section_planner_node import (
    _deterministic_conclusion_claims,
    _deterministic_es_claims,
)


def _make_facts(n: int = 3, *, with_numeric: bool = False) -> list[dict]:
    facts = []
    for i in range(1, n + 1):
        fact: dict = {
            "claim": f"Fact {i} about topic",
            "data_type": "quote",
            "source_id": f"art-{i}",
            "source_title": f"Article {i}",
            "verbatim_quote": f"Quote from article {i}",
            "confidence": 0.9 - i * 0.1,
        }
        if with_numeric and i <= 2:
            fact["data_type"] = "statistic"
            fact["numeric_facts"] = [f"{i * 10}%"]
        facts.append(fact)
    return facts


def _make_analysis_claims(n: int = 3) -> list[dict]:
    return [
        {
            "claim_id": f"analysis-{i}",
            "claim": f"Analysis claim {i}",
            "claim_type": "factual",
            "evidence_ids": [f"art-{i}"],
            "supporting_quotes": [f"Quote {i}"],
            "numeric_facts": [f"{i * 10}%"] if i <= 2 else [],
            "novelty_against": [],
            "must_cite": True,
        }
        for i in range(1, n + 1)
    ]


# --- Conclusion fallback ---


def test_conclusion_fallback_when_analysis_claims_empty() -> None:
    """analysis claims=[] + extracted_facts exist → deterministic claims generated."""
    facts = _make_facts(3)
    result = _deterministic_conclusion_claims([], facts)
    assert len(result) > 0
    for claim in result:
        assert claim["claim_type"] == "synthesis"
        assert claim["claim"]  # non-empty string


def test_conclusion_fallback_from_analysis_claims() -> None:
    """When analysis claims exist, use them for synthesis."""
    analysis_claims = _make_analysis_claims(3)
    result = _deterministic_conclusion_claims(analysis_claims, [])
    assert len(result) > 0
    for claim in result:
        assert claim["claim_type"] == "synthesis"


def test_conclusion_fallback_prefers_numeric_facts() -> None:
    """Facts with numeric data are preferred in fallback ranking."""
    facts = _make_facts(5, with_numeric=True)
    result = _deterministic_conclusion_claims([], facts)
    # At least one claim should have numeric_facts
    has_numeric = any(claim.get("numeric_facts") for claim in result)
    assert has_numeric


def test_conclusion_fallback_empty_when_no_input() -> None:
    """No analysis claims AND no extracted_facts → empty list."""
    result = _deterministic_conclusion_claims([], [])
    assert result == []


# --- ES fallback ---


def test_es_fallback_when_all_claims_empty() -> None:
    """All section claims empty → extracted_facts used for ES."""
    facts = _make_facts(3)
    result = _deterministic_es_claims({}, facts)
    assert len(result) > 0
    for claim in result:
        assert claim["claim_type"] == "synthesis"


def test_es_never_returns_empty_when_facts_exist() -> None:
    """Even 1 fact should produce at least 1 ES claim."""
    facts = _make_facts(1)
    result = _deterministic_es_claims({}, facts)
    assert len(result) >= 1


def test_es_fallback_from_existing_claims() -> None:
    """When section claim_plans exist, pick top claims for ES."""
    all_claims = {
        "analysis": _make_analysis_claims(4),
        "conclusion": [
            {
                "claim_id": "conclusion-1",
                "claim": "Conclusion synthesis",
                "claim_type": "synthesis",
                "evidence_ids": ["art-1"],
                "supporting_quotes": ["Q"],
                "numeric_facts": [],
                "novelty_against": ["analysis"],
                "must_cite": True,
            },
        ],
    }
    result = _deterministic_es_claims(all_claims, [])
    assert len(result) > 0
    for claim in result:
        assert claim["claim_type"] == "synthesis"


def test_deterministic_synthesis_prefers_source_diversity() -> None:
    """Claims from different sources should be preferred over same-source claims."""
    facts = [
        {
            "claim": f"Fact {i}",
            "data_type": "quote",
            "source_id": "art-1",
            "source_title": "Article 1",
            "verbatim_quote": f"Q{i}",
            "confidence": 0.9,
        }
        for i in range(1, 6)
    ] + [
        {
            "claim": "Diverse fact",
            "data_type": "statistic",
            "source_id": "art-2",
            "source_title": "Article 2",
            "verbatim_quote": "QD",
            "confidence": 0.8,
            "numeric_facts": ["42%"],
        }
    ]
    result = _deterministic_es_claims({}, facts)
    # Should include the diverse source (art-2)
    source_ids = set()
    for claim in result:
        source_ids.update(claim.get("evidence_ids", []))
    assert len(source_ids) > 1
