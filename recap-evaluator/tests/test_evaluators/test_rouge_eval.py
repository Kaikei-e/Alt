"""Tests for ROUGE evaluator.

TDD RED Phase: These tests define the expected behavior for the ROUGE evaluator.
"""

import pytest

from recap_evaluator.evaluators.rouge_eval import ROUGEEvaluator, ROUGEResult


class TestROUGEResult:
    """Tests for ROUGEResult dataclass."""

    def test_to_dict_returns_all_scores(self):
        """ROUGEResult.to_dict() should return all ROUGE scores."""
        result = ROUGEResult(
            rouge_1_precision=0.8,
            rouge_1_recall=0.7,
            rouge_1_f1=0.75,
            rouge_2_precision=0.6,
            rouge_2_recall=0.5,
            rouge_2_f1=0.55,
            rouge_l_precision=0.7,
            rouge_l_recall=0.6,
            rouge_l_f1=0.65,
        )

        d = result.to_dict()

        assert d["rouge_1_precision"] == 0.8
        assert d["rouge_1_recall"] == 0.7
        assert d["rouge_1_f1"] == 0.75
        assert d["rouge_2_precision"] == 0.6
        assert d["rouge_2_recall"] == 0.5
        assert d["rouge_2_f1"] == 0.55
        assert d["rouge_l_precision"] == 0.7
        assert d["rouge_l_recall"] == 0.6
        assert d["rouge_l_f1"] == 0.65


class TestROUGEEvaluator:
    """Tests for ROUGEEvaluator."""

    def test_compute_rouge_identical_texts(self):
        """Identical texts should produce perfect ROUGE scores."""
        evaluator = ROUGEEvaluator()
        candidate = "The quick brown fox jumps over the lazy dog."
        reference = "The quick brown fox jumps over the lazy dog."

        result = evaluator.compute_rouge(candidate, reference)

        assert result.rouge_1_f1 == pytest.approx(1.0, abs=0.01)
        assert result.rouge_2_f1 == pytest.approx(1.0, abs=0.01)
        assert result.rouge_l_f1 == pytest.approx(1.0, abs=0.01)

    def test_compute_rouge_different_texts(self):
        """Different texts should produce lower ROUGE scores."""
        evaluator = ROUGEEvaluator()
        candidate = "The cat sat on the mat."
        reference = "A dog ran in the park."

        result = evaluator.compute_rouge(candidate, reference)

        assert result.rouge_1_f1 < 0.5
        assert result.rouge_2_f1 < 0.3

    def test_compute_rouge_partial_overlap(self):
        """Partial overlap should produce moderate ROUGE scores."""
        evaluator = ROUGEEvaluator()
        candidate = "The quick brown fox."
        reference = "The quick brown fox jumps over the lazy dog."

        result = evaluator.compute_rouge(candidate, reference)

        # Should have high precision (candidate words are in reference)
        assert result.rouge_1_precision > 0.8
        # Should have lower recall (reference has more words)
        assert result.rouge_1_recall < result.rouge_1_precision

    def test_compute_rouge_japanese_text(self):
        """ROUGE should handle Japanese text properly."""
        evaluator = ROUGEEvaluator()
        candidate = "人工知能は急速に発展している。"
        reference = "人工知能の技術は急速に発展を遂げている。"

        result = evaluator.compute_rouge(candidate, reference, lang="ja")

        # Should produce reasonable scores for similar Japanese sentences
        assert 0.0 < result.rouge_1_f1 < 1.0
        assert 0.0 < result.rouge_l_f1 < 1.0

    def test_compute_batch_aggregates_scores(self):
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
        assert result["num_samples"] == 2

    def test_compute_batch_mismatched_lengths_raises(self):
        """Batch evaluation should raise ValueError for mismatched lengths."""
        evaluator = ROUGEEvaluator()
        candidates = ["Text 1", "Text 2"]
        references = ["Ref 1"]

        with pytest.raises(ValueError, match="Mismatched lengths"):
            evaluator.compute_batch(candidates, references)

    def test_compute_rouge_empty_text_handles_gracefully(self):
        """Empty text should be handled gracefully."""
        evaluator = ROUGEEvaluator()

        result = evaluator.compute_rouge("", "Some reference text")

        # Should return zeros for empty candidate
        assert result.rouge_1_f1 == 0.0
        assert result.rouge_2_f1 == 0.0
        assert result.rouge_l_f1 == 0.0

    def test_compute_batch_with_individual_scores(self):
        """Batch evaluation should optionally return individual scores."""
        evaluator = ROUGEEvaluator()
        candidates = ["The quick brown fox.", "Hello world."]
        references = ["The quick brown fox jumps.", "Hello world!"]

        result = evaluator.compute_batch(
            candidates, references, return_individual=True
        )

        assert "individual_scores" in result
        assert len(result["individual_scores"]) == 2
