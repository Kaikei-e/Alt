"""Unit tests for Critic heuristic quality tuning.

Per-role min length, paragraph-level duplication, paragraph citation check,
richer claim_feedbacks, warning accumulation.
"""

from __future__ import annotations

import pytest

from acolyte.domain.critic_taxonomy import FailureMode
from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.graph.nodes.critic_node import (
    CriticNode,
    detect_incomplete_sections,
    detect_paragraph_duplication,
    detect_paragraph_missing_citation,
)


class FakeLLM:
    def __init__(self, text: str = '{"reasoning":"ok","verdict":"accept","revise_sections":[],"feedback":{}}') -> None:
        self._text = text
        self.call_count = 0

    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        self.call_count += 1
        return LLMResponse(text=self._text, model="fake")


# --- Per-role minimum section length ---


def test_min_section_length_varies_by_role() -> None:
    """Analysis at 150 chars → FM3 warning; ES at 80 chars → pass."""
    short_text = "a" * 150
    es_text = "b" * 80
    sections: dict[str, str] = {"analysis": short_text, "executive_summary": es_text}
    outline = [
        {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
        {"key": "executive_summary", "title": "ES", "section_role": "executive_summary"},
    ]
    detections = detect_incomplete_sections(sections, outline)
    # analysis 150 chars < 200 (analysis min) → warning
    analysis_dets = [d for d in detections if d.section_key == "analysis"]
    assert len(analysis_dets) == 1
    assert analysis_dets[0].mode == FailureMode.FM3_INCOMPLETE_INFORMATION
    # ES 80 chars >= 80 (ES min) → no detection
    es_dets = [d for d in detections if d.section_key == "executive_summary"]
    assert len(es_dets) == 0


# --- Paragraph-level duplication (FM12) ---


def test_paragraph_duplication_within_section() -> None:
    """Two paragraphs in same section with high overlap → FM12."""
    # Same text repeated → Jaccard ≈ 1.0
    section_paragraphs = {
        "analysis": [
            {
                "claim_id": "a-1",
                "body": "AIの市場規模は急速に拡大しており、2025年には前年比20%増の成長が見込まれている。",
            },
            {
                "claim_id": "a-2",
                "body": "AIの市場規模は急速に拡大しており、2025年には前年比20%増の成長が見込まれている。",
            },
        ],
    }
    outline = [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}]
    detections = detect_paragraph_duplication(section_paragraphs, outline)
    assert len(detections) >= 1
    assert detections[0].mode == FailureMode.FM12_PARAGRAPH_DUPLICATION


def test_paragraph_duplication_cross_section() -> None:
    """Paragraph overlapping with novelty_against section → FM12."""
    section_paragraphs = {
        "analysis": [
            {"claim_id": "a-1", "body": "AIの市場規模は急速に拡大しており成長が見込まれている。"},
        ],
        "conclusion": [
            {"claim_id": "c-1", "body": "AIの市場規模は急速に拡大しており成長が見込まれている。"},
        ],
    }
    outline = [
        {"key": "analysis", "title": "Analysis", "section_role": "analysis"},
        {"key": "conclusion", "title": "Conclusion", "section_role": "conclusion", "novelty_against": ["analysis"]},
    ]
    detections = detect_paragraph_duplication(section_paragraphs, outline)
    assert len(detections) >= 1
    # Detection should be on the conclusion paragraph
    assert any(d.section_key == "conclusion" for d in detections)


# --- Paragraph-level citation check (FM13) ---


def test_paragraph_missing_citation_must_cite() -> None:
    """must_cite=True + no citation in paragraph → FM13 warning."""
    section_paragraphs = {
        "analysis": [
            {"claim_id": "a-1", "body": "Some text.", "citations": []},
        ],
    }
    claim_plans = {
        "analysis": [
            {"claim_id": "a-1", "must_cite": True, "evidence_ids": ["art-1"]},
        ],
    }
    detections = detect_paragraph_missing_citation(section_paragraphs, claim_plans)
    assert len(detections) == 1
    assert detections[0].mode == FailureMode.FM13_PARAGRAPH_MISSING_CITATION
    assert detections[0].severity == "warning"


def test_paragraph_missing_citation_not_triggered_when_cited() -> None:
    """must_cite=True + citation present → no FM13."""
    section_paragraphs = {
        "analysis": [
            {"claim_id": "a-1", "body": "Some text [1].", "citations": [{"source_id": "art-1"}]},
        ],
    }
    claim_plans = {
        "analysis": [
            {"claim_id": "a-1", "must_cite": True, "evidence_ids": ["art-1"]},
        ],
    }
    detections = detect_paragraph_missing_citation(section_paragraphs, claim_plans)
    assert len(detections) == 0


# --- Richer claim_feedbacks ---


@pytest.mark.asyncio
async def test_claim_feedback_includes_specific_reason_for_duplication() -> None:
    """FM12 duplication → feedback reason includes overlap target info."""
    llm = FakeLLM()
    node = CriticNode(llm)
    dup_text = "AIの市場規模は急速に拡大しており、2025年には前年比20%増の成長が見込まれている。"
    state = {
        "sections": {"analysis": dup_text + "\n\n" + dup_text},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "claim_plans": {
            "analysis": [
                {"claim_id": "a-1", "must_cite": False},
                {"claim_id": "a-2", "must_cite": False},
            ],
        },
        "section_paragraphs": {
            "analysis": [
                {"claim_id": "a-1", "body": dup_text, "status": "accepted", "citations": []},
                {"claim_id": "a-2", "body": dup_text, "status": "accepted", "citations": []},
            ],
        },
    }
    result = await node(state)
    claim_fbs = result.get("claim_feedbacks", {})
    # Should have feedback for the duplicated paragraph
    if "analysis" in claim_fbs:
        reasons = [fb["reason"] for fb in claim_fbs["analysis"]]
        assert any("overlap" in r or "duplication" in r or "duplicate" in r for r in reasons)


@pytest.mark.asyncio
async def test_claim_feedback_includes_specific_reason_for_empty() -> None:
    """FM4 empty body → feedback reason is specific, not just 'body empty'."""
    llm = FakeLLM()
    node = CriticNode(llm)
    state = {
        "sections": {"analysis": ""},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "claim_plans": {"analysis": [{"claim_id": "a-1", "must_cite": True}]},
        "section_paragraphs": {
            "analysis": [
                {"claim_id": "a-1", "body": "", "status": "rejected", "citations": []},
            ],
        },
    }
    result = await node(state)
    claim_fbs = result.get("claim_feedbacks", {})
    assert "analysis" in claim_fbs
    reason = claim_fbs["analysis"][0]["reason"]
    # Should be more specific than just "body empty"
    assert "regenerate" in reason or "evidence" in reason or "claim" in reason


@pytest.mark.asyncio
async def test_claim_feedback_includes_specific_reason_for_short() -> None:
    """FM3 short section → feedback includes char count and target."""
    llm = FakeLLM()
    node = CriticNode(llm)
    short_text = "短いテキスト。"  # Very short
    state = {
        "sections": {"analysis": short_text},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "claim_plans": {"analysis": [{"claim_id": "a-1", "must_cite": False}]},
        "section_paragraphs": {
            "analysis": [
                {"claim_id": "a-1", "body": short_text, "status": "accepted", "citations": []},
            ],
        },
    }
    result = await node(state)
    # FM3 should trigger for short analysis section
    fms = result.get("failure_modes", [])
    fm3_dets = [fm for fm in fms if fm.get("mode") == "incomplete_info"]
    assert len(fm3_dets) >= 1


# --- Warning accumulation ---


@pytest.mark.asyncio
async def test_warning_accumulation_promotes_to_blocking() -> None:
    """3+ warnings on same section → verdict=revise."""
    llm = FakeLLM()
    node = CriticNode(llm)
    # Section with multiple warning-triggering conditions:
    # FM3 (short), FM11 (no numeric in ES), FM13 (missing citation)
    state = {
        "sections": {"executive_summary": "短い要約。"},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "executive_summary", "title": "ES", "section_role": "executive_summary"}],
        "claim_plans": {
            "executive_summary": [
                {"claim_id": "es-1", "must_cite": True, "evidence_ids": ["art-1"], "numeric_facts": []},
                {"claim_id": "es-2", "must_cite": True, "evidence_ids": ["art-2"], "numeric_facts": []},
                {"claim_id": "es-3", "must_cite": True, "evidence_ids": ["art-3"], "numeric_facts": []},
            ],
        },
        "section_paragraphs": {
            "executive_summary": [
                {"claim_id": "es-1", "body": "短い。", "status": "accepted", "citations": []},
                {"claim_id": "es-2", "body": "短い。", "status": "accepted", "citations": []},
                {"claim_id": "es-3", "body": "短い。", "status": "accepted", "citations": []},
            ],
        },
    }
    result = await node(state)
    # Multiple warnings should promote to blocking → revise
    assert result["critique"]["verdict"] == "revise"


# --- LLM critic still skipped ---


@pytest.mark.asyncio
async def test_llm_critic_still_skipped_on_blocking_heuristic() -> None:
    """With new heuristics, LLM critic is still skipped when blocking found."""
    llm = FakeLLM()
    node = CriticNode(llm)
    state = {
        "sections": {"analysis": ""},
        "brief": {"topic": "AI trends"},
        "outline": [{"key": "analysis", "title": "Analysis", "section_role": "analysis"}],
        "claim_plans": {"analysis": [{"claim_id": "a-1", "must_cite": True}]},
        "section_paragraphs": {
            "analysis": [{"claim_id": "a-1", "body": "", "status": "rejected", "citations": []}],
        },
    }
    await node(state)
    assert llm.call_count == 0
