"""Unit tests for eval runner — multi-protocol integration."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.eval.checklist_evaluator import ChecklistEvaluator
from acolyte.usecase.eval.eval_runner import EvalRunner
from acolyte.usecase.eval.rubric_evaluator import RubricEvaluator


class FakeLLM:
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        return LLMResponse(
            text=json.dumps(
                {
                    "claims": [
                        {"claim": "AI market grew", "supported": True, "source_id": "art-1"},
                        {"claim": "Unsupported claim", "supported": False, "source_id": ""},
                    ]
                }
            ),
            model="fake",
        )


@pytest.mark.asyncio
async def test_eval_runner_checklist_only() -> None:
    runner = EvalRunner(checklist=ChecklistEvaluator())
    result = await runner.evaluate(
        report_id="r-1",
        run_id="run-1",
        sections={"summary": "AI trends continue to grow rapidly. " * 15},
        evidence=[],
        scope={"topic": "AI trends"},
        outline=[{"key": "summary", "title": "Summary"}],
    )
    assert result.report_id == "r-1"
    assert len(result.dimensions) == 3  # task_fulfillment, coverage, presentation
    assert all(d.protocol == "checklist" for d in result.dimensions)
    assert 0.0 <= result.overall_score <= 1.0


@pytest.mark.asyncio
async def test_eval_runner_with_rubric() -> None:
    runner = EvalRunner(
        checklist=ChecklistEvaluator(),
        rubric=RubricEvaluator(FakeLLM()),
    )
    result = await runner.evaluate(
        report_id="r-2",
        run_id="run-2",
        sections={"summary": "AI market grew significantly. " * 15},
        evidence=[{"id": "art-1", "title": "AI Market"}],
        scope={"topic": "AI market"},
        outline=[{"key": "summary", "title": "Summary"}],
    )
    assert len(result.dimensions) == 5  # 3 checklist + 2 rubric
    assert result.evaluated_at is not None


@pytest.mark.asyncio
async def test_eval_runner_dimension_map() -> None:
    runner = EvalRunner(checklist=ChecklistEvaluator())
    result = await runner.evaluate(
        report_id="r-3",
        run_id="run-3",
        sections={"summary": "AI trends are important for semiconductor markets. " * 15},
        evidence=[],
        scope={"topic": "AI trends"},
        outline=[{"key": "summary", "title": "Summary"}],
    )
    dim_map = result.dimension_map
    assert "task_fulfillment" in dim_map
    assert "coverage" in dim_map
    assert "presentation" in dim_map


@pytest.mark.asyncio
async def test_coverage_excludes_section_generated_items() -> None:
    """Coverage must not double-count task_fulfillment's section_generated:* items."""
    runner = EvalRunner(checklist=ChecklistEvaluator())
    # Missing section body → section_generated fails AND section_present fails.
    # If coverage wrongly includes section_generated, both fail and score stays 0;
    # with a partial pass we can detect dilution. Use a present-but-short body so
    # section_present passes and section_length fails → coverage = 0.5 when
    # section_generated is excluded; would be 0.33 if section_generated were included.
    short_body = "AI short"  # < MIN_SECTION_LENGTH
    result = await runner.evaluate(
        report_id="r-4",
        run_id="run-4",
        sections={"summary": short_body},
        evidence=[],
        scope={"topic": "AI"},
        outline=[{"key": "summary", "title": "Summary"}],
    )
    assert result.dimension_map["coverage"] == 0.5
    tf = next(d for d in result.dimensions if d.name == "task_fulfillment")
    names = [i["name"] for i in tf.details["items"]]
    assert any(n.startswith("section_generated:") for n in names)
