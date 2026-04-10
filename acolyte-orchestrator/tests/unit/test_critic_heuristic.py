"""Unit tests for Critic heuristic-first redesign (Issue 5).

New heuristics: FM4 (empty body → blocking), FM5 (zero claims → blocking),
FM11 (ES numeric absence → warning). claim_feedbacks for paragraph-level feedback.
"""

from __future__ import annotations

import pytest

from acolyte.domain.critic_taxonomy import FailureMode
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.critic_node import (
    CriticNode,
    detect_empty_body,
    detect_es_numeric_absence,
    detect_zero_claims,
)


class FakeLLM:
    def __init__(self, text: str = '{"reasoning":"ok","verdict":"accept","revise_sections":[],"feedback":{}}') -> None:
        self._text = text
        self.call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        return LLMResponse(text=self._text, model="fake")


# --- FM4: empty body ---


def test_empty_body_is_blocking() -> None:
    """body="" for a section → FM4 blocking."""
    sections = {"analysis": "", "conclusion": "Some content."}
    outline = [
        {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
        {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion"},
    ]
    detections = detect_empty_body(sections, outline)
    assert len(detections) == 1
    assert detections[0].mode == FailureMode.FM4_EMPTY_BODY
    assert detections[0].severity == "blocking"
    assert detections[0].section_key == "analysis"


def test_empty_body_not_triggered_for_nonempty() -> None:
    """Non-empty body → no FM4 detection."""
    sections = {"analysis": "Content here."}
    outline = [{"key": "analysis", "title": "Analysis"}]
    detections = detect_empty_body(sections, outline)
    assert len(detections) == 0


# --- FM5: zero claims ---


def test_zero_claims_is_blocking() -> None:
    """claims=[] for a section with evidence → FM5 blocking."""
    claim_plans = {"analysis": []}
    outline = [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}]
    extracted_facts = [{"source_id": "art-1", "claim": "Something"}]
    detections = detect_zero_claims(claim_plans, outline, extracted_facts)
    assert len(detections) == 1
    assert detections[0].mode == FailureMode.FM5_ZERO_CLAIMS
    assert detections[0].severity == "blocking"


def test_zero_claims_not_triggered_when_claims_exist() -> None:
    """claims exist → no FM5 detection."""
    claim_plans = {"analysis": [{"claim_id": "a-1", "claim": "Something"}]}
    outline = [{"key": "analysis", "title": "Analysis"}]
    detections = detect_zero_claims(claim_plans, outline, [])
    assert len(detections) == 0


def test_zero_claims_not_triggered_when_no_evidence() -> None:
    """claims=[] but no extracted_facts → not blocking (nothing to claim from)."""
    claim_plans = {"analysis": []}
    outline = [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}]
    detections = detect_zero_claims(claim_plans, outline, [])
    assert len(detections) == 0


# --- FM11: ES numeric absence ---


def test_es_numeric_absence_detected() -> None:
    """ES claims with no numeric_facts → FM11 warning."""
    claim_plans = {
        "executive_summary": [
            {"claim_id": "es-1", "claim": "Summary", "numeric_facts": [], "must_cite": True},
        ],
    }
    outline = [{"key": "executive_summary", "title": "ES", "section_role": "executive_summary"}]
    detections = detect_es_numeric_absence(claim_plans, outline)
    assert len(detections) == 1
    assert detections[0].mode == FailureMode.FM11_ES_NUMERIC_ABSENCE
    assert detections[0].severity == "warning"


def test_es_numeric_absence_not_triggered_with_numeric() -> None:
    """ES claims with numeric_facts → no FM11 detection."""
    claim_plans = {
        "executive_summary": [
            {"claim_id": "es-1", "claim": "Summary", "numeric_facts": ["42%"], "must_cite": True},
        ],
    }
    outline = [{"key": "executive_summary", "title": "ES", "section_role": "executive_summary"}]
    detections = detect_es_numeric_absence(claim_plans, outline)
    assert len(detections) == 0


# --- claim_feedbacks format ---


@pytest.mark.asyncio
async def test_claim_feedbacks_format() -> None:
    """Critic returns claim_feedbacks with claim_id-level entries."""
    llm = FakeLLM()
    node = CriticNode(llm)

    state = {
        "sections": {"analysis": ""},  # empty body → blocking
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "claim_plans": {"analysis": [{"claim_id": "analysis-1", "claim": "Something"}]},
        "section_paragraphs": {
            "analysis": [
                {"claim_id": "analysis-1", "body": "", "status": "rejected"},
            ],
        },
    }

    result = await node(state)
    claim_feedbacks = result.get("claim_feedbacks", {})
    # Should have paragraph-level feedback for rejected paragraphs
    assert "analysis" in claim_feedbacks
    assert len(claim_feedbacks["analysis"]) > 0
    fb = claim_feedbacks["analysis"][0]
    assert "claim_id" in fb
    assert "action" in fb
    assert "reason" in fb


# --- LLM critic behavior ---


@pytest.mark.asyncio
async def test_llm_critic_skipped_when_blocking_heuristic() -> None:
    """When heuristic finds blocking issues, LLM critic is not called."""
    llm = FakeLLM()
    node = CriticNode(llm)

    state = {
        "sections": {"analysis": ""},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
    }

    await node(state)
    # LLM should NOT be called because heuristic found blocking issue
    assert llm.call_count == 0


@pytest.mark.asyncio
async def test_llm_critic_failure_falls_back_to_heuristic() -> None:
    """When LLM returns unparseable JSON, heuristic results are used."""
    llm = FakeLLM(text="INVALID JSON {{{{")
    node = CriticNode(llm)

    state = {
        "sections": {"analysis": "Some reasonable content about AI trends that is long enough." * 5},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
    }

    result = await node(state)
    # Should not crash; verdict should be "revise" (parse failure → revise)
    assert result["critique"]["verdict"] == "revise"
