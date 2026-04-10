"""Unit tests for rubric evaluator (LLM-based, uses FakeLLM)."""

from __future__ import annotations

import json

import pytest

from acolyte.port.llm_provider import LLMResponse
from acolyte.usecase.eval.rubric_evaluator import RubricEvaluator


class FakeLLM:
    async def generate(self, prompt: str, **kwargs: object) -> LLMResponse:
        # Return structured rubric scores
        return LLMResponse(
            text=json.dumps(
                {
                    "claims": [
                        {"claim": "AI market grew 20%", "supported": True, "source_id": "art-1"},
                        {"claim": "NVIDIA leads the market", "supported": True, "source_id": "art-2"},
                        {"claim": "Market will reach $1T", "supported": False, "source_id": ""},
                    ]
                }
            ),
            model="fake",
        )


@pytest.fixture
def evaluator() -> RubricEvaluator:
    return RubricEvaluator(FakeLLM())


@pytest.mark.asyncio
async def test_factual_consistency_scores_supported_claims(evaluator: RubricEvaluator) -> None:
    sections = {"summary": "AI market grew 20%. NVIDIA leads. Market will reach $1T."}
    evidence = [{"id": "art-1", "title": "AI Market"}, {"id": "art-2", "title": "NVIDIA"}]
    result = await evaluator.evaluate_factual_consistency(sections, evidence)
    # 2/3 claims supported → ~0.67
    assert 0.5 < result.score < 0.8


@pytest.mark.asyncio
async def test_citation_association_measures_sourced_claims(evaluator: RubricEvaluator) -> None:
    sections = {"summary": "AI market grew 20%. NVIDIA leads. Market will reach $1T."}
    evidence = [{"id": "art-1"}, {"id": "art-2"}]
    result = await evaluator.evaluate_citation_association(sections, evidence)
    # 2/3 claims have source_id → ~0.67
    assert 0.5 < result.score < 0.8


@pytest.mark.asyncio
async def test_full_evaluate_returns_dimensions(evaluator: RubricEvaluator) -> None:
    sections = {"summary": "AI market analysis content."}
    evidence = [{"id": "art-1", "title": "AI"}]
    result = await evaluator.evaluate(sections, evidence)
    assert len(result) == 2
    assert all(d.protocol == "rubric" for d in result)
