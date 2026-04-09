"""Unit tests for contract-driven critic checks (FM9, FM10)."""

from __future__ import annotations

from acolyte.domain.critic_taxonomy import FailureMode
from acolyte.usecase.graph.nodes.critic_node import (
    detect_insufficient_citations,
    detect_novelty_violation,
)


# --- FM9: Insufficient citations ---


def test_detect_insufficient_citations_below_minimum() -> None:
    """Section with min_citations=3 but only 1 citation → FM9 detection."""
    outline = [{"key": "analysis", "section_role": "analysis", "min_citations": 3}]
    section_citations = {"analysis": [{"claim_id": "a-1", "source_id": "art-1"}]}

    detections = detect_insufficient_citations(outline, section_citations)
    assert len(detections) == 1
    assert detections[0].mode == FailureMode.FM9_INSUFFICIENT_CITATIONS
    assert detections[0].section_key == "analysis"
    assert detections[0].severity == "warning"


def test_detect_insufficient_citations_meets_minimum() -> None:
    """Section with min_citations=2 and 2 citations → no detection."""
    outline = [{"key": "analysis", "section_role": "analysis", "min_citations": 2}]
    section_citations = {
        "analysis": [
            {"claim_id": "a-1", "source_id": "art-1"},
            {"claim_id": "a-2", "source_id": "art-2"},
        ]
    }

    detections = detect_insufficient_citations(outline, section_citations)
    assert len(detections) == 0


def test_detect_insufficient_citations_zero_minimum() -> None:
    """Section with min_citations=0 → always passes regardless of citation count."""
    outline = [{"key": "analysis", "section_role": "analysis", "min_citations": 0}]
    section_citations = {"analysis": []}

    detections = detect_insufficient_citations(outline, section_citations)
    assert len(detections) == 0


def test_detect_insufficient_citations_missing_field() -> None:
    """Section without min_citations in outline → no detection (backward compat)."""
    outline = [{"key": "analysis", "section_role": "analysis"}]
    section_citations = {"analysis": []}

    detections = detect_insufficient_citations(outline, section_citations)
    assert len(detections) == 0


# --- FM10: Novelty violation ---


def test_detect_novelty_violation_high_overlap() -> None:
    """Section with novelty_against=['analysis'] that repeats analysis → FM10 detection."""
    sections = {
        "analysis": "AIの市場規模は急速に拡大しており、年間成長率は20%を超えています。半導体産業への影響も大きいです。",
        "conclusion": "AIの市場規模は急速に拡大しており、年間成長率は20%を超えています。半導体産業への影響も大きいです。",
    }
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion", "novelty_against": ["analysis"]},
    ]

    detections = detect_novelty_violation(sections, outline)
    assert len(detections) == 1
    assert detections[0].mode == FailureMode.FM10_NOVELTY_VIOLATION
    assert detections[0].section_key == "conclusion"


def test_detect_novelty_violation_no_overlap() -> None:
    """Distinct content → no FM10 detection."""
    sections = {
        "analysis": "AIの市場規模は急速に拡大しており年間成長率は20%を超えています。",
        "conclusion": "総合的に判断すると今後の投資優先順位はクラウドインフラが最も高い。",
    }
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion", "novelty_against": ["analysis"]},
    ]

    detections = detect_novelty_violation(sections, outline)
    assert len(detections) == 0


def test_detect_novelty_violation_no_contract_field() -> None:
    """Section without novelty_against in outline → no detection (backward compat)."""
    sections = {
        "analysis": "Same content everywhere.",
        "conclusion": "Same content everywhere.",
    }
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion"},
    ]

    detections = detect_novelty_violation(sections, outline)
    assert len(detections) == 0
