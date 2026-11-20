"""Tests for ROUGE evaluator.

TDD RED Phase: These tests define the expected behavior for the ROUGE evaluator.
"""

import pytest

from recap_evaluator.evaluator.rouge_eval import ROUGEEvaluator, ROUGEResult

ROUGE_1_PRECISION = 0.8
ROUGE_1_RECALL = 0.7
ROUGE_1_F1 = 0.75
ROUGE_2_PRECISION = 0.6
ROUGE_2_RECALL = 0.5
ROUGE_2_F1 = 0.55
ROUGE_L_PRECISION = 0.7
ROUGE_L_RECALL = 0.6
ROUGE_L_F1 = 0.65
MAX_UNRELATED_ROUGE_1 = 0.5
MAX_UNRELATED_ROUGE_2 = 0.3
MIN_PARTIAL_PRECISION = 0.8
EXPECTED_BATCH_SAMPLES = 2


class TestROUGEResult:
    """Tests for ROUGEResult dataclass."""

    def test_to_dict_returns_all_scores(self) -> None:
        """ROUGEResult.to_dict() should return all ROUGE scores."""
        result = ROUGEResult(
            rouge_1_precision=ROUGE_1_PRECISION,
            rouge_1_recall=ROUGE_1_RECALL,
            rouge_1_f1=ROUGE_1_F1,
            rouge_2_precision=ROUGE_2_PRECISION,
            rouge_2_recall=ROUGE_2_RECALL,
            rouge_2_f1=ROUGE_2_F1,
            rouge_l_precision=ROUGE_L_PRECISION,
            rouge_l_recall=ROUGE_L_RECALL,
            rouge_l_f1=ROUGE_L_F1,
        )

        d = result.to_dict()

        assert d["rouge_1_precision"] == ROUGE_1_PRECISION
        assert d["rouge_1_recall"] == ROUGE_1_RECALL
        assert d["rouge_1_f1"] == ROUGE_1_F1
        assert d["rouge_2_precision"] == ROUGE_2_PRECISION
        assert d["rouge_2_recall"] == ROUGE_2_RECALL
        assert d["rouge_2_f1"] == ROUGE_2_F1
        assert d["rouge_l_precision"] == ROUGE_L_PRECISION
        assert d["rouge_l_recall"] == ROUGE_L_RECALL
        assert d["rouge_l_f1"] == ROUGE_L_F1


class TestROUGEEvaluator:
    """Tests for ROUGEEvaluator."""

    def test_compute_rouge_identical_texts(self) -> None:
        """Identical texts should produce perfect ROUGE scores."""
        evaluator = ROUGEEvaluator()
        candidate = "The quick brown fox jumps over the lazy dog."
        reference = "The quick brown fox jumps over the lazy dog."

        result = evaluator.compute_rouge(candidate, reference)

        assert result.rouge_1_f1 == pytest.approx(1.0, abs=0.01)
        assert result.rouge_2_f1 == pytest.approx(1.0, abs=0.01)
        assert result.rouge_l_f1 == pytest.approx(1.0, abs=0.01)

    def test_compute_rouge_different_texts(self) -> None:
        """Different texts should produce lower ROUGE scores."""
        evaluator = ROUGEEvaluator()
        candidate = "The cat sat on the mat."
        reference = "A dog ran in the park."

        result = evaluator.compute_rouge(candidate, reference)

        assert result.rouge_1_f1 < MAX_UNRELATED_ROUGE_1
        assert result.rouge_2_f1 < MAX_UNRELATED_ROUGE_2

    def test_compute_rouge_partial_overlap(self) -> None:
        """Partial overlap should produce moderate ROUGE scores."""
        evaluator = ROUGEEvaluator()
        candidate = "The quick brown fox."
        reference = "The quick brown fox jumps over the lazy dog."

        result = evaluator.compute_rouge(candidate, reference)

        # Should have high precision (candidate words are in reference)
        assert result.rouge_1_precision > MIN_PARTIAL_PRECISION
        # Should have lower recall (reference has more words)
        assert result.rouge_1_recall < result.rouge_1_precision

    def test_compute_rouge_japanese_text(self) -> None:
        """ROUGE should handle Japanese text properly."""
        evaluator = ROUGEEvaluator()
        candidate = "人工知能は急速に発展している。"
        reference = "人工知能の技術は急速に発展を遂げている。"

        result = evaluator.compute_rouge(candidate, reference, lang="ja")

        # Should produce reasonable scores for similar Japanese sentences
        assert 0.0 < result.rouge_1_f1 < 1.0
        assert 0.0 < result.rouge_l_f1 < 1.0

    def test_compute_batch_aggregates_scores(self) -> None:
        """Batch evaluation should aggregate scores across pairs."""
        evaluator = ROUGEEvaluator()
        candidates = [
            "The quick brown fox.",
            "Hello world.",
        ]
        references = [
            "The quick brown fox jumps over the lazy dog.",
            "Hello world!",
        ]

        result = evaluator.compute_batch(candidates, references)

        # Should return average scores
        assert "rouge_1_f1" in result
        assert "rouge_2_f1" in result
        assert "rouge_l_f1" in result
        assert result["num_samples"] == EXPECTED_BATCH_SAMPLES

    def test_compute_batch_mismatched_lengths_raises(self) -> None:
        """Batch evaluation should raise ValueError for mismatched lengths."""
        evaluator = ROUGEEvaluator()
        candidates = ["Text 1", "Text 2"]
        references = ["Ref 1"]

        with pytest.raises(ValueError, match="Mismatched lengths"):
            evaluator.compute_batch(candidates, references)

    def test_compute_rouge_empty_text_handles_gracefully(self) -> None:
        """Empty text should be handled gracefully."""
        evaluator = ROUGEEvaluator()

        result = evaluator.compute_rouge("", "Some reference text")

        # Should return zeros for empty candidate
        assert result.rouge_1_f1 == 0.0
        assert result.rouge_2_f1 == 0.0
        assert result.rouge_l_f1 == 0.0

    def test_compute_batch_with_individual_scores(self) -> None:
        """Batch evaluation should optionally return individual scores."""
        evaluator = ROUGEEvaluator()
        candidates = ["The quick brown fox.", "Hello world."]
        references = ["The quick brown fox jumps.", "Hello world!"]

        result = evaluator.compute_batch(candidates, references, return_individual=True)

        assert "individual_scores" in result
        assert len(result["individual_scores"]) == EXPECTED_BATCH_SAMPLES
