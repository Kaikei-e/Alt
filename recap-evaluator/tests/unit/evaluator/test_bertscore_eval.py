"""Tests for BERTScore evaluator.

TDD RED Phase: These tests define the expected behavior for the BERTScore evaluator.
"""

from collections.abc import Iterator
from unittest.mock import MagicMock, patch

import pytest
import torch

from recap_evaluator.evaluator.bertscore_eval import BERTScoreEvaluator, BERTScoreResult

EXPECTED_PRECISION = 0.85
EXPECTED_RECALL = 0.80
EXPECTED_F1 = 0.82
EXPECTED_PAIR_COUNT = 2
MIN_EN_INTEGRATION_F1 = 0.5
MIN_JA_INTEGRATION_F1 = 0.3


class TestBERTScoreResult:
    """Tests for BERTScoreResult dataclass."""

    def test_to_dict_returns_all_scores(self) -> None:
        """BERTScoreResult.to_dict() should return all scores."""
        result = BERTScoreResult(
            precision=EXPECTED_PRECISION,
            recall=EXPECTED_RECALL,
            f1=EXPECTED_F1,
        )

        d = result.to_dict()

        assert d["precision"] == EXPECTED_PRECISION
        assert d["recall"] == EXPECTED_RECALL
        assert d["f1"] == EXPECTED_F1

    def test_to_dict_includes_individual_scores_when_present(self) -> None:
        """BERTScoreResult.to_dict() should include individual scores if provided."""
        result = BERTScoreResult(
            precision=EXPECTED_PRECISION,
            recall=EXPECTED_RECALL,
            f1=EXPECTED_F1,
            individual_scores=[
                {"precision": 0.9, "recall": 0.85, "f1": 0.87},
                {"precision": 0.8, "recall": 0.75, "f1": 0.77},
            ],
        )

        d = result.to_dict()

        assert "individual_scores" in d
        assert len(d["individual_scores"]) == EXPECTED_PAIR_COUNT


class TestBERTScoreEvaluator:
    """Tests for BERTScoreEvaluator."""

    @pytest.fixture
    def mock_bert_score(self) -> Iterator[MagicMock]:
        """Mock the bert_score function."""
        with patch("recap_evaluator.evaluator.bertscore_eval.bert_score") as mock:
            # Return tensors like the real bert_score does
            mock.return_value = (
                torch.tensor([0.85, 0.80]),  # Precision
                torch.tensor([0.80, 0.75]),  # Recall
                torch.tensor([0.82, 0.77]),  # F1
            )
            yield mock

    def test_model_map_contains_japanese(self) -> None:
        """Evaluator should have Japanese model mapping."""
        evaluator = BERTScoreEvaluator()

        assert "ja" in evaluator.MODEL_MAP
        assert "cl-tohoku" in evaluator.MODEL_MAP["ja"]

    def test_model_map_contains_english(self) -> None:
        """Evaluator should have English model mapping."""
        evaluator = BERTScoreEvaluator()

        assert "en" in evaluator.MODEL_MAP

    def test_compute_bert_score_returns_result(self, mock_bert_score: MagicMock) -> None:
        """compute_bert_score should return BERTScoreResult with the mean of
        the mocked per-pair tensors (precision=[0.85, 0.80], f1=[0.82, 0.77])
        — an isinstance/range-only check would pass even if the evaluator
        picked the wrong tensor or forgot to average across pairs."""
        evaluator = BERTScoreEvaluator()

        result = evaluator.compute_bert_score(
            candidates=["Generated summary."],
            references=["Reference text."],
            lang="en",
        )

        assert isinstance(result, BERTScoreResult)
        assert result.precision == pytest.approx(0.825, abs=0.001)
        assert result.recall == pytest.approx(0.775, abs=0.001)
        assert result.f1 == pytest.approx(0.795, abs=0.001)

    def test_compute_bert_score_mismatched_lengths_raises(self, mock_bert_score: MagicMock) -> None:
        """compute_bert_score should raise ValueError for mismatched lengths."""
        evaluator = BERTScoreEvaluator()

        with pytest.raises(ValueError, match="Mismatched lengths"):
            evaluator.compute_bert_score(
                candidates=["Text 1", "Text 2"],
                references=["Ref 1"],
                lang="en",
            )

    def test_compute_bert_score_unsupported_language_raises(
        self, mock_bert_score: MagicMock
    ) -> None:
        """compute_bert_score should raise ValueError for unsupported language."""
        evaluator = BERTScoreEvaluator()

        with pytest.raises(ValueError, match="Unsupported language"):
            evaluator.compute_bert_score(
                candidates=["Text"],
                references=["Ref"],
                lang="xx",  # pyrefly: ignore[bad-argument-type]  # Invalid language
            )

    def test_compute_bert_score_with_individual_scores(self, mock_bert_score: MagicMock) -> None:
        """compute_bert_score should optionally return per-pair scores."""
        evaluator = BERTScoreEvaluator()

        result = evaluator.compute_bert_score(
            candidates=["Summary 1.", "Summary 2."],
            references=["Reference 1.", "Reference 2."],
            lang="en",
            return_individual=True,
        )

        assert result.individual_scores is not None
        assert len(result.individual_scores) == EXPECTED_PAIR_COUNT

    def test_evaluate_summary_quality_single_pair(self, mock_bert_score: MagicMock) -> None:
        """evaluate_summary_quality is a thin wrapper delegating to
        compute_bert_score([summary], [source_text]) — assert it actually
        forwards the mocked score rather than just returning some result."""
        evaluator = BERTScoreEvaluator()

        result = evaluator.evaluate_summary_quality(
            summary="This is a summary.",
            source_text="This is the source text.",
            lang="en",
        )

        assert isinstance(result, BERTScoreResult)
        assert result.f1 == pytest.approx(0.795, abs=0.001)

    def test_evaluate_batch_returns_aggregate_metrics(self, mock_bert_score: MagicMock) -> None:
        """evaluate_batch should return aggregated metrics."""
        evaluator = BERTScoreEvaluator()

        result = evaluator.evaluate_batch(
            summaries=["Summary 1.", "Summary 2."],
            sources=["Source 1.", "Source 2."],
            lang="en",
        )

        assert "mean_precision" in result
        assert "mean_recall" in result
        assert "mean_f1" in result
        assert result["num_samples"] == EXPECTED_PAIR_COUNT


class TestBERTScoreEvaluatorIntegration:
    """Integration tests for BERTScoreEvaluator (requires bert-score package)."""

    @pytest.mark.skip(reason="Integration test - requires GPU/model download")
    def test_real_bert_score_computation(self) -> None:
        """Test actual BERTScore computation (skipped by default)."""
        evaluator = BERTScoreEvaluator()

        result = evaluator.compute_bert_score(
            candidates=["The cat sat on the mat."],
            references=["A cat is sitting on a mat."],
            lang="en",
        )

        # Semantically similar sentences should have high score
        assert result.f1 > MIN_EN_INTEGRATION_F1

    @pytest.mark.skip(reason="Integration test - requires Japanese model")
    def test_real_bert_score_japanese(self) -> None:
        """Test BERTScore with Japanese text (skipped by default)."""
        evaluator = BERTScoreEvaluator()

        result = evaluator.compute_bert_score(
            candidates=["AIは急速に発展している。"],
            references=["人工知能の技術は発展を遂げている。"],
            lang="ja",
        )

        assert result.f1 > MIN_JA_INTEGRATION_F1
