"""Tests for ReadabilityEvaluator — LLM-based quick-catch readability score (1-5)."""

from unittest.mock import AsyncMock

import httpx
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
    async def test_llm_network_failure_returns_zero(self):
        """A recoverable LLM-call failure (network/HTTP) defaults to 0.0."""
        mock_ollama = AsyncMock()
        mock_ollama.score_readability.side_effect = httpx.ConnectError(
            "ollama unreachable"
        )

        evaluator = ReadabilityEvaluator(mock_ollama)
        score = await evaluator.evaluate("要約。")
        assert score == 0.0

    @pytest.mark.asyncio
    async def test_unparseable_llm_response_returns_zero(self):
        """A malformed LLM response (ValueError/KeyError) also defaults to 0.0."""
        mock_ollama = AsyncMock()
        mock_ollama.score_readability.side_effect = ValueError("no JSON found")

        evaluator = ReadabilityEvaluator(mock_ollama)
        score = await evaluator.evaluate("要約。")
        assert score == 0.0

    @pytest.mark.asyncio
    async def test_unexpected_error_propagates(self):
        """A bug (e.g. AttributeError from a missing method) must NOT be
        silently swallowed into a fake 0.0 score — that would hide a wiring
        defect as "always low readability" (CLAUDE.md rule 8)."""
        mock_ollama = AsyncMock()
        mock_ollama.score_readability.side_effect = AttributeError(
            "'OllamaGateway' object has no attribute 'score_readability'"
        )

        evaluator = ReadabilityEvaluator(mock_ollama)
        with pytest.raises(AttributeError):
            await evaluator.evaluate("要約。")
