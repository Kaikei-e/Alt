"""Unit tests for FM8 conclusion-analysis duplication detection in CriticNode."""

from __future__ import annotations

from acolyte.domain.critic_taxonomy import FailureMode
from acolyte.usecase.graph.nodes.critic_node import (
    _claim_synthesis_ratio,
    detect_conclusion_analysis_duplication,
)


def test_detect_conclusion_analysis_duplication_high_overlap() -> None:
    """High bigram overlap between analysis and conclusion → FM8 blocking detection."""
    analysis_text = (
        "AI市場は急速に成長している。半導体需要が増加し、GPUの供給が不足している。企業はAI投資を拡大している。"
    )
    # Conclusion that mostly repeats analysis
    conclusion_text = (
        "AI市場は急速に成長している。半導体需要が増加している。GPUの供給が不足しており、企業はAI投資を拡大している。"
    )

    sections = {"analysis": analysis_text, "conclusion": conclusion_text}
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion"},
    ]

    detections = detect_conclusion_analysis_duplication(sections, outline)
    assert len(detections) >= 1
    assert detections[0].mode == FailureMode.FM8_CONCLUSION_ANALYSIS_DUPLICATION
    assert detections[0].severity == "blocking"
    assert detections[0].section_key == "conclusion"


def test_detect_conclusion_analysis_duplication_no_overlap() -> None:
    """Distinct content between analysis and conclusion → no detection."""
    analysis_text = "AI市場は急速に成長している。半導体需要が増加し、GPUの供給が不足している。"
    conclusion_text = "以上の分析を踏まえ、リスク管理と投資優先順位の再評価が必要である。短期的にはサプライチェーンの多角化を推奨する。"

    sections = {"analysis": analysis_text, "conclusion": conclusion_text}
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion"},
    ]

    detections = detect_conclusion_analysis_duplication(sections, outline)
    assert len(detections) == 0


def test_detect_conclusion_analysis_duplication_only_when_both_exist() -> None:
    """No crash when analysis or conclusion is missing."""
    # Only analysis, no conclusion
    sections = {"analysis": "Some analysis text."}
    outline = [
        {"key": "analysis", "section_role": "analysis"},
    ]
    detections = detect_conclusion_analysis_duplication(sections, outline)
    assert len(detections) == 0

    # Only conclusion, no analysis
    sections2 = {"conclusion": "Some conclusion text."}
    outline2 = [
        {"key": "conclusion", "section_role": "conclusion"},
    ]
    detections2 = detect_conclusion_analysis_duplication(sections2, outline2)
    assert len(detections2) == 0

    # Both in outline but conclusion body is empty
    sections3 = {"analysis": "Analysis text.", "conclusion": ""}
    outline3 = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion"},
    ]
    detections3 = detect_conclusion_analysis_duplication(sections3, outline3)
    assert len(detections3) == 0


def test_fm8_high_overlap_low_synthesis_is_blocking() -> None:
    """High overlap + low synthesis ratio → blocking."""
    analysis_text = (
        "AI市場は急速に成長している。半導体需要が増加し、GPUの供給が不足している。企業はAI投資を拡大している。"
    )
    conclusion_text = (
        "AI市場は急速に成長している。半導体需要が増加している。GPUの供給が不足しており、企業はAI投資を拡大している。"
    )

    sections = {"analysis": analysis_text, "conclusion": conclusion_text}
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion"},
    ]
    # Paragraphs that reference only single claims (no synthesis)
    section_paragraphs = {
        "analysis": [{"claim_id": "a-1", "body": "text", "evidence_ids": ["src1"]}],
        "conclusion": [{"claim_id": "c-1", "body": "text", "evidence_ids": ["src1"]}],
    }

    detections = detect_conclusion_analysis_duplication(sections, outline, section_paragraphs=section_paragraphs)
    assert len(detections) >= 1
    assert detections[0].severity == "blocking"


def test_fm8_high_overlap_high_synthesis_is_warning() -> None:
    """High overlap + high synthesis ratio → warning (overlap exists but synthesis happening)."""
    analysis_text = (
        "AI市場は急速に成長している。半導体需要が増加し、GPUの供給が不足している。企業はAI投資を拡大している。"
    )
    conclusion_text = (
        "AI市場は急速に成長している。半導体需要が増加している。GPUの供給が不足しており、企業はAI投資を拡大している。"
    )

    sections = {"analysis": analysis_text, "conclusion": conclusion_text}
    outline = [
        {"key": "analysis", "section_role": "analysis"},
        {"key": "conclusion", "section_role": "conclusion"},
    ]
    # Paragraphs that reference multiple analysis claims (cross-claim synthesis)
    section_paragraphs = {
        "analysis": [
            {"claim_id": "a-1", "body": "text", "evidence_ids": ["src1"]},
            {"claim_id": "a-2", "body": "text", "evidence_ids": ["src2"]},
        ],
        "conclusion": [
            {"claim_id": "c-1", "body": "text", "evidence_ids": ["a-1", "a-2"]},  # references 2 analysis claims
        ],
    }

    detections = detect_conclusion_analysis_duplication(sections, outline, section_paragraphs=section_paragraphs)
    assert len(detections) >= 1
    assert detections[0].severity == "warning"


def test_claim_synthesis_ratio_single_claim_is_zero() -> None:
    """Paragraphs referencing single claims → synthesis ratio 0."""
    section_paragraphs = {
        "conclusion": [
            {"claim_id": "c-1", "body": "text", "evidence_ids": ["a-1"]},
            {"claim_id": "c-2", "body": "text", "evidence_ids": ["a-2"]},
        ],
    }
    analysis_claim_ids = {"a-1", "a-2", "a-3"}
    ratio = _claim_synthesis_ratio(section_paragraphs, {"conclusion"}, analysis_claim_ids)
    assert ratio == 0.0


def test_claim_synthesis_ratio_cross_claim_is_high() -> None:
    """Paragraphs referencing multiple claims → high synthesis ratio."""
    section_paragraphs = {
        "conclusion": [
            {"claim_id": "c-1", "body": "text", "evidence_ids": ["a-1", "a-2"]},
            {"claim_id": "c-2", "body": "text", "evidence_ids": ["a-2", "a-3"]},
        ],
    }
    analysis_claim_ids = {"a-1", "a-2", "a-3"}
    ratio = _claim_synthesis_ratio(section_paragraphs, {"conclusion"}, analysis_claim_ids)
    assert ratio == 1.0
