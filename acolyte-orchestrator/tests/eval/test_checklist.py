"""Unit tests for checklist evaluator (rule-based, no LLM)."""

from __future__ import annotations

import pytest

from acolyte.usecase.eval.checklist_evaluator import ChecklistEvaluator


@pytest.fixture
def evaluator() -> ChecklistEvaluator:
    return ChecklistEvaluator()


def test_task_fulfillment_passes_when_topic_in_sections(evaluator: ChecklistEvaluator) -> None:
    scope = {"topic": "AI semiconductor"}
    outline = [{"key": "summary", "title": "Summary"}]
    sections = {"summary": "The AI semiconductor market continues to grow rapidly."}
    result = evaluator.check_task_fulfillment(scope, outline, sections)
    assert any(c.name == "topic_in_content" and c.passed for c in result)


def test_task_fulfillment_fails_when_topic_absent(evaluator: ChecklistEvaluator) -> None:
    scope = {"topic": "AI semiconductor"}
    outline = [{"key": "summary", "title": "Summary"}]
    sections = {"summary": "This is a generic report about nothing specific."}
    result = evaluator.check_task_fulfillment(scope, outline, sections)
    assert any(c.name == "topic_in_content" and not c.passed for c in result)


def test_coverage_passes_when_all_sections_present(evaluator: ChecklistEvaluator) -> None:
    outline = [{"key": "intro", "title": "Intro"}, {"key": "analysis", "title": "Analysis"}]
    sections = {"intro": "x" * 250, "analysis": "y" * 250}
    result = evaluator.check_coverage(outline, sections)
    assert all(c.passed for c in result)


def test_coverage_fails_when_section_too_short(evaluator: ChecklistEvaluator) -> None:
    outline = [{"key": "intro", "title": "Intro"}]
    sections = {"intro": "Too short."}
    result = evaluator.check_coverage(outline, sections)
    assert any(not c.passed for c in result)


def test_coverage_fails_when_section_missing(evaluator: ChecklistEvaluator) -> None:
    outline = [{"key": "intro", "title": "Intro"}, {"key": "analysis", "title": "Analysis"}]
    sections = {"intro": "x" * 250}  # analysis missing
    result = evaluator.check_coverage(outline, sections)
    assert any(c.name == "section_present:analysis" and not c.passed for c in result)


def test_presentation_passes_clean_content(evaluator: ChecklistEvaluator) -> None:
    sections = {"summary": "The market grew by 20% in Q2 2026. Key players include NVIDIA and TSMC."}
    result = evaluator.check_presentation(sections)
    assert all(c.passed for c in result)


def test_presentation_fails_on_meta_statement(evaluator: ChecklistEvaluator) -> None:
    sections = {"summary": "情報が不足しているため、詳細な分析ができません。"}
    result = evaluator.check_presentation(sections)
    assert any(c.name == "no_meta_statements" and not c.passed for c in result)


def test_presentation_fails_on_english_meta(evaluator: ChecklistEvaluator) -> None:
    sections = {"summary": "As an AI language model, I don't have specific data on this topic."}
    result = evaluator.check_presentation(sections)
    assert any(c.name == "no_meta_statements" and not c.passed for c in result)


def test_full_evaluate_returns_eval_result(evaluator: ChecklistEvaluator) -> None:
    scope = {"topic": "AI trends"}
    outline = [{"key": "summary", "title": "Summary"}]
    sections = {"summary": "AI trends continue to accelerate. " * 20}
    result = evaluator.evaluate(scope, outline, sections)
    assert 0.0 <= result.score <= 1.0
    assert len(result.items) > 0
