"""Tests for Faithfulness evaluator.

TDD RED Phase: These tests define the expected behavior for the Faithfulness evaluator
using NLI-based hallucination detection.
"""

import pytest
from unittest.mock import patch, MagicMock

from recap_evaluator.evaluators.faithfulness_eval import (
    FaithfulnessEvaluator,
    FaithfulnessResult,
    SentenceResult,
)


class TestSentenceResult:
    """Tests for SentenceResult dataclass."""

    def test_to_dict_returns_all_fields(self):
        """SentenceResult.to_dict() should return all fields."""
        result = SentenceResult(
            sentence="AI is evolving rapidly.",
            is_supported=True,
            entailment_score=0.85,
            contradiction_score=0.05,
            neutral_score=0.10,
        )

        d = result.to_dict()

        assert d["sentence"] == "AI is evolving rapidly."
        assert d["is_supported"] is True
        assert d["entailment_score"] == 0.85
        assert d["contradiction_score"] == 0.05
        assert d["neutral_score"] == 0.10


class TestFaithfulnessResult:
    """Tests for FaithfulnessResult dataclass."""

    def test_to_dict_returns_all_scores(self):
        """FaithfulnessResult.to_dict() should return all scores."""
        result = FaithfulnessResult(
            is_hallucinated=False,
            hallucination_score=0.15,
            faithfulness_score=0.85,
            entailment_score=0.85,
            contradiction_score=0.05,
            neutral_score=0.10,
        )

        d = result.to_dict()

        assert d["is_hallucinated"] is False
        assert d["hallucination_score"] == 0.15
        assert d["faithfulness_score"] == 0.85
        assert d["entailment_score"] == 0.85

    def test_to_dict_includes_sentence_results_when_present(self):
        """FaithfulnessResult.to_dict() should include sentence results if provided."""
        result = FaithfulnessResult(
            is_hallucinated=False,
            hallucination_score=0.15,
            faithfulness_score=0.85,
            entailment_score=0.85,
            contradiction_score=0.05,
            neutral_score=0.10,
            sentence_results=[
                SentenceResult(
                    sentence="Test sentence.",
                    is_supported=True,
                    entailment_score=0.9,
                    contradiction_score=0.05,
                    neutral_score=0.05,
                )
            ],
        )

        d = result.to_dict()

        assert "sentence_results" in d
        assert len(d["sentence_results"]) == 1


class TestFaithfulnessEvaluator:
    """Tests for FaithfulnessEvaluator."""

    @pytest.fixture
    def mock_nli_pipeline(self):
        """Mock the transformers NLI pipeline."""
        with patch(
            "recap_evaluator.evaluators.faithfulness_eval.pipeline"
        ) as mock_pipeline:
            # Create mock NLI that returns high entailment scores
            mock_nli = MagicMock()
            mock_nli.return_value = [[
                {"label": "entailment", "score": 0.85},
                {"label": "neutral", "score": 0.10},
                {"label": "contradiction", "score": 0.05},
            ]]
            mock_pipeline.return_value = mock_nli
            yield mock_pipeline

    def test_default_model_is_modern_bert_nli(self):
        """Evaluator should use ModernBERT-base-nli as default."""
        evaluator = FaithfulnessEvaluator.__new__(FaithfulnessEvaluator)
        evaluator._nli = None

        assert FaithfulnessEvaluator.DEFAULT_MODEL == "tasksource/ModernBERT-base-nli"

    def test_detect_returns_faithfulness_result(self, mock_nli_pipeline):
        """detect() should return FaithfulnessResult."""
        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI technology is advancing.",
            source_sentences=["AI technology is advancing rapidly."],
        )

        assert isinstance(result, FaithfulnessResult)

    def test_detect_high_entailment_is_not_hallucinated(self, mock_nli_pipeline):
        """High entailment should result in is_hallucinated=False."""
        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI technology is advancing.",
            source_sentences=["AI technology is advancing rapidly."],
            threshold=0.5,
        )

        assert result.is_hallucinated is False
        assert result.faithfulness_score > 0.5

    def test_detect_low_entailment_is_hallucinated(self, mock_nli_pipeline):
        """Low entailment should result in is_hallucinated=True."""
        # Reconfigure mock for low entailment
        mock_nli = mock_nli_pipeline.return_value
        mock_nli.return_value = [[
            {"label": "entailment", "score": 0.2},
            {"label": "neutral", "score": 0.3},
            {"label": "contradiction", "score": 0.5},
        ]]

        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI is declining.",
            source_sentences=["AI technology is advancing."],
            threshold=0.5,
        )

        assert result.is_hallucinated is True
        assert result.hallucination_score > 0.5

    def test_detect_with_sentence_results(self, mock_nli_pipeline):
        """detect() should optionally return per-sentence results."""
        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI is advancing. Technology improves.",
            source_sentences=["AI technology is advancing rapidly."],
            return_sentence_results=True,
        )

        assert result.sentence_results is not None
        assert len(result.sentence_results) >= 1

    def test_detect_empty_summary_raises(self, mock_nli_pipeline):
        """detect() should raise ValueError for empty summary."""
        evaluator = FaithfulnessEvaluator()

        with pytest.raises(ValueError, match="summary cannot be empty"):
            evaluator.detect(
                summary="",
                source_sentences=["Some source text."],
            )

    def test_detect_empty_sources_raises(self, mock_nli_pipeline):
        """detect() should raise ValueError for empty sources."""
        evaluator = FaithfulnessEvaluator()

        with pytest.raises(ValueError, match="source_sentences cannot be empty"):
            evaluator.detect(
                summary="Some summary.",
                source_sentences=[],
            )

    def test_detect_batch_processes_multiple_summaries(self, mock_nli_pipeline):
        """detect_batch() should process multiple summaries."""
        evaluator = FaithfulnessEvaluator()

        results = evaluator.detect_batch(
            summaries=["Summary 1.", "Summary 2."],
            sources=[
                ["Source for summary 1."],
                ["Source for summary 2."],
            ],
        )

        assert len(results) == 2
        assert all(isinstance(r, FaithfulnessResult) for r in results)

    def test_detect_batch_mismatched_lengths_raises(self, mock_nli_pipeline):
        """detect_batch() should raise ValueError for mismatched lengths."""
        evaluator = FaithfulnessEvaluator()

        with pytest.raises(ValueError, match="Mismatched lengths"):
            evaluator.detect_batch(
                summaries=["Summary 1.", "Summary 2."],
                sources=[["Source 1."]],  # Only one source list
            )

    def test_split_sentences_handles_english(self, mock_nli_pipeline):
        """_split_sentences should handle English text."""
        evaluator = FaithfulnessEvaluator()

        sentences = evaluator._split_sentences("First sentence. Second sentence! Third?")

        assert len(sentences) == 3
        assert "First sentence." in sentences[0]

    def test_split_sentences_handles_japanese(self, mock_nli_pipeline):
        """_split_sentences should handle Japanese text."""
        evaluator = FaithfulnessEvaluator()

        sentences = evaluator._split_sentences("最初の文です。二番目の文！三番目？")

        assert len(sentences) == 3

    def test_max_pooling_across_sources(self, mock_nli_pipeline):
        """detect() should use max-pooling for entailment across sources."""
        # Configure mock to return different scores on different calls
        mock_nli = mock_nli_pipeline.return_value
        mock_nli.side_effect = [
            [[{"label": "entailment", "score": 0.3}, {"label": "neutral", "score": 0.4}, {"label": "contradiction", "score": 0.3}]],
            [[{"label": "entailment", "score": 0.9}, {"label": "neutral", "score": 0.05}, {"label": "contradiction", "score": 0.05}]],
        ]

        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI is advancing.",  # Single sentence
            source_sentences=["Unrelated text.", "AI technology is advancing rapidly."],
            threshold=0.5,
        )

        # Should use the higher entailment score (0.9) due to max-pooling
        assert result.entailment_score >= 0.5
        assert result.is_hallucinated is False


class TestFaithfulnessEvaluatorIntegration:
    """Integration tests for FaithfulnessEvaluator (requires transformers)."""

    @pytest.mark.skip(reason="Integration test - requires model download")
    def test_real_nli_detection(self):
        """Test actual NLI-based detection (skipped by default)."""
        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI technology is declining in 2025.",
            source_sentences=["AI technology is advancing rapidly in 2025."],
        )

        # Contradictory statement should be detected as hallucination
        assert result.is_hallucinated is True

    @pytest.mark.skip(reason="Integration test - requires model download")
    def test_real_nli_supported_text(self):
        """Test NLI with supported text (skipped by default)."""
        evaluator = FaithfulnessEvaluator()

        result = evaluator.detect(
            summary="AI is developing quickly.",
            source_sentences=["Artificial intelligence technology is advancing rapidly."],
        )

        assert result.is_hallucinated is False
