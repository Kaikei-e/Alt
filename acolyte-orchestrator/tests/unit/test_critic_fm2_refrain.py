"""Unit tests for FM2 meta-statement heuristic detection."""

from __future__ import annotations

from acolyte.usecase.graph.nodes.critic_node import detect_meta_statements


def test_detects_japanese_meta_statement() -> None:
    sections = {"summary": "情報が不足しているため、詳細な分析ができません。"}
    detections = detect_meta_statements(sections)
    assert len(detections) >= 1
    assert detections[0].section_key == "summary"
    assert detections[0].mode.value == "failure_to_refrain"


def test_detects_english_meta_statement() -> None:
    sections = {"summary": "As an AI language model, I don't have access to real-time data."}
    detections = detect_meta_statements(sections)
    assert len(detections) >= 1


def test_detects_topic_not_specified() -> None:
    sections = {"intro": "トピックが明示されていませんでした。一般的な知識に基づいて記述します。"}
    detections = detect_meta_statements(sections)
    assert len(detections) >= 1


def test_clean_content_passes() -> None:
    sections = {"summary": "AI半導体市場は2026年Q2に20%成長した。NVIDIAが市場を牽引している。"}
    detections = detect_meta_statements(sections)
    assert len(detections) == 0


def test_multiple_sections_checked() -> None:
    sections = {
        "intro": "AI market continues to grow.",
        "analysis": "I cannot provide specific data on this topic.",
    }
    detections = detect_meta_statements(sections)
    assert len(detections) == 1
    assert detections[0].section_key == "analysis"
