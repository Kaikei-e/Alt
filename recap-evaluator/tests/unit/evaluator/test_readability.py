"""Tests for ReadabilityEvaluator — LLM-based quick-catch readability score (1-5)."""

from unittest.mock import AsyncMock

import pytest

from recap_evaluator.evaluator.readability import ReadabilityEvaluator


class TestReadabilityEvaluator:
    @pytest.mark.asyncio
    async def test_score_returned_within_valid_range(self):
        mock_ollama = AsyncMock()
        mock_ollama.score_readability.return_value = 4.2

        evaluator = ReadabilityEvaluator(mock_ollama)
        score = await evaluator.evaluate("本日の主要な変化: AI エージェント導入で業務自動化が進む。")

        assert 1.0 <= score <= 5.0
        assert score == pytest.approx(4.2, abs=0.01)

    @pytest.mark.asyncio
    async def test_empty_summary_returns_zero(self):
        mock_ollama = AsyncMock()
        evaluator = ReadabilityEvaluator(mock_ollama)
        score = await evaluator.evaluate("")
        assert score == 0.0
        mock_ollama.score_readability.assert_not_called()

    @pytest.mark.asyncio
    async def test_batch_averages_per_item_scores(self):
        mock_ollama = AsyncMock()
        mock_ollama.score_readability.side_effect = [3.0, 5.0]

        evaluator = ReadabilityEvaluator(mock_ollama)
        avg = await evaluator.evaluate_batch(["短い要約.", "別の要約."])

        assert avg == pytest.approx(4.0, abs=0.01)

    @pytest.mark.asyncio
    async def test_llm_failure_propagates_zero(self):
        mock_ollama = AsyncMock()
        mock_ollama.score_readability.side_effect = RuntimeError("ollama down")

        evaluator = ReadabilityEvaluator(mock_ollama)
        score = await evaluator.evaluate("要約。")
        assert score == 0.0
