"""Unit tests for G-Eval (LLM-as-Judge) evaluation."""

from __future__ import annotations

import pytest
from unittest.mock import patch, MagicMock, AsyncMock

from recap_subworker.services.g_eval import (
    GEvalEvaluator,
    GEvalResult,
    EvaluationDimension,
)


class TestEvaluationDimension:
    """Tests for EvaluationDimension enum."""

    def test_all_dimensions_exist(self):
        """Should have all required evaluation dimensions."""
        assert EvaluationDimension.COHERENCE
        assert EvaluationDimension.CONSISTENCY
        assert EvaluationDimension.FLUENCY
        assert EvaluationDimension.RELEVANCE

    def test_dimension_values(self):
        """Should have correct string values."""
        assert EvaluationDimension.COHERENCE.value == "coherence"
        assert EvaluationDimension.CONSISTENCY.value == "consistency"


class TestGEvalResult:
    """Tests for GEvalResult dataclass."""

    def test_result_creation(self):
        """Should create result with required fields."""
        result = GEvalResult(
            coherence=4.5,
            consistency=4.0,
            fluency=5.0,
            relevance=3.5,
        )
        assert result.coherence == pytest.approx(4.5)
        assert result.consistency == pytest.approx(4.0)
        assert result.fluency == pytest.approx(5.0)
        assert result.relevance == pytest.approx(3.5)

    def test_result_average(self):
        """Should compute average score correctly."""
        result = GEvalResult(
            coherence=4.0,
            consistency=4.0,
            fluency=4.0,
            relevance=4.0,
        )
        assert result.average == pytest.approx(4.0)

    def test_result_to_dict(self):
        """Should convert to dictionary."""
        result = GEvalResult(
            coherence=4.5,
            consistency=4.0,
            fluency=5.0,
            relevance=3.5,
        )
        d = result.to_dict()
        assert d["coherence"] == pytest.approx(4.5)
        assert d["consistency"] == pytest.approx(4.0)
        assert "average" in d

    def test_result_with_explanations(self):
        """Should support optional explanations."""
        result = GEvalResult(
            coherence=4.5,
            consistency=4.0,
            fluency=5.0,
            relevance=3.5,
            explanations={
                "coherence": "Well-structured narrative.",
                "fluency": "Natural language use.",
            },
        )
        assert result.explanations is not None
        assert "coherence" in result.explanations


class TestGEvalEvaluator:
    """Tests for GEvalEvaluator class."""

    def test_default_dimensions(self):
        """Should have default evaluation dimensions."""
        evaluator = GEvalEvaluator()
        assert EvaluationDimension.COHERENCE in evaluator.dimensions
        assert EvaluationDimension.CONSISTENCY in evaluator.dimensions
        assert EvaluationDimension.FLUENCY in evaluator.dimensions
        assert EvaluationDimension.RELEVANCE in evaluator.dimensions

    def test_custom_dimensions(self):
        """Should accept custom dimensions."""
        evaluator = GEvalEvaluator(
            dimensions=[EvaluationDimension.COHERENCE, EvaluationDimension.FLUENCY]
        )
        assert len(evaluator.dimensions) == 2
        assert EvaluationDimension.CONSISTENCY not in evaluator.dimensions

    def test_get_prompt_for_dimension(self):
        """Should generate evaluation prompt for each dimension."""
        evaluator = GEvalEvaluator()

        prompt = evaluator._get_prompt_for_dimension(
            EvaluationDimension.COHERENCE,
            summary="AI is evolving rapidly.",
            source="Artificial intelligence technology continues to advance.",
        )

        assert "coherence" in prompt.lower()
        assert "AI is evolving rapidly" in prompt
        assert "1" in prompt and "5" in prompt  # Rating scale

    @pytest.mark.asyncio
    async def test_evaluate_single_dimension(self):
        """Should evaluate a single dimension."""
        evaluator = GEvalEvaluator()

        with patch.object(evaluator, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = "Score: 4\nExplanation: Good coherence."

            score, explanation = await evaluator._evaluate_dimension(
                EvaluationDimension.COHERENCE,
                summary="Test summary.",
                source="Test source.",
            )

            assert score == pytest.approx(4.0)
            assert explanation is not None

    @pytest.mark.asyncio
    async def test_evaluate_returns_all_dimensions(self):
        """Should return scores for all dimensions."""
        evaluator = GEvalEvaluator()

        with patch.object(evaluator, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = "Score: 4\nExplanation: Good quality."

            result = await evaluator.evaluate(
                summary="AI technology advances.",
                source="Artificial intelligence is progressing.",
            )

            assert isinstance(result, GEvalResult)
            assert result.coherence is not None
            assert result.consistency is not None
            assert result.fluency is not None
            assert result.relevance is not None

    @pytest.mark.asyncio
    async def test_evaluate_with_explanations(self):
        """Should include explanations when requested."""
        evaluator = GEvalEvaluator()

        with patch.object(evaluator, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = "Score: 4\nExplanation: Well written."

            result = await evaluator.evaluate(
                summary="Test summary.",
                source="Test source.",
                include_explanations=True,
            )

            assert result.explanations is not None
            assert len(result.explanations) > 0

    @pytest.mark.asyncio
    async def test_evaluate_batch(self):
        """Should evaluate multiple summaries."""
        evaluator = GEvalEvaluator()

        with patch.object(evaluator, "_call_llm", new_callable=AsyncMock) as mock_llm:
            mock_llm.return_value = "Score: 4\nExplanation: Good."

            results = await evaluator.evaluate_batch(
                summaries=["Summary 1.", "Summary 2."],
                sources=["Source 1.", "Source 2."],
            )

            assert len(results) == 2
            assert all(isinstance(r, GEvalResult) for r in results)

    def test_parse_score_from_response(self):
        """Should parse score from LLM response."""
        evaluator = GEvalEvaluator()

        # Various response formats
        assert evaluator._parse_score("Score: 4") == 4.0
        assert evaluator._parse_score("4/5") == 4.0
        assert evaluator._parse_score("Rating: 3.5") == 3.5
        assert evaluator._parse_score("The score is 5.") == 5.0

    def test_parse_score_clamps_range(self):
        """Should clamp scores to valid range [1, 5]."""
        evaluator = GEvalEvaluator()

        assert evaluator._parse_score("Score: 0") == 1.0  # Clamped to min
        assert evaluator._parse_score("Score: 6") == 5.0  # Clamped to max

    def test_parse_score_default_on_failure(self):
        """Should return default score when parsing fails."""
        evaluator = GEvalEvaluator()

        assert evaluator._parse_score("No score here") == 3.0  # Default mid-range


class TestGEvalPrompts:
    """Tests for G-Eval prompt templates."""

    def test_coherence_prompt_contains_criteria(self):
        """Coherence prompt should mention logical flow."""
        evaluator = GEvalEvaluator()
        prompt = evaluator._get_prompt_for_dimension(
            EvaluationDimension.COHERENCE,
            summary="Test",
            source="Test",
        )
        # Should mention coherence-related terms
        assert any(
            term in prompt.lower()
            for term in ["coherence", "logical", "flow", "structure"]
        )

    def test_consistency_prompt_contains_criteria(self):
        """Consistency prompt should mention factual alignment."""
        evaluator = GEvalEvaluator()
        prompt = evaluator._get_prompt_for_dimension(
            EvaluationDimension.CONSISTENCY,
            summary="Test",
            source="Test",
        )
        assert any(
            term in prompt.lower()
            for term in ["consistency", "factual", "accurate", "source"]
        )

    def test_fluency_prompt_contains_criteria(self):
        """Fluency prompt should mention language quality."""
        evaluator = GEvalEvaluator()
        prompt = evaluator._get_prompt_for_dimension(
            EvaluationDimension.FLUENCY,
            summary="Test",
            source="Test",
        )
        assert any(
            term in prompt.lower()
            for term in ["fluency", "grammar", "readable", "natural"]
        )

    def test_relevance_prompt_contains_criteria(self):
        """Relevance prompt should mention information coverage."""
        evaluator = GEvalEvaluator()
        prompt = evaluator._get_prompt_for_dimension(
            EvaluationDimension.RELEVANCE,
            summary="Test",
            source="Test",
        )
        assert any(
            term in prompt.lower()
            for term in ["relevance", "important", "key", "coverage"]
        )
