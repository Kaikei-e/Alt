"""Unit tests for SemanticEvaluator (BERTScore-based evaluation)."""

from __future__ import annotations

import pytest
from unittest.mock import patch, MagicMock

from recap_subworker.services.semantic_eval import SemanticEvaluator, BERTScoreResult


class TestSemanticEvaluator:
    """Tests for SemanticEvaluator class."""

    def test_model_map_contains_ja_and_en(self):
        """MODEL_MAP should contain both Japanese and English models."""
        assert "ja" in SemanticEvaluator.MODEL_MAP
        assert "en" in SemanticEvaluator.MODEL_MAP
        assert SemanticEvaluator.MODEL_MAP["ja"] == "cl-tohoku/bert-base-japanese-v3"
        assert SemanticEvaluator.MODEL_MAP["en"] == "microsoft/deberta-xlarge-mnli"

    @patch("recap_subworker.services.semantic_eval.bert_score")
    def test_compute_bert_score_returns_valid_structure(self, mock_bert_score):
        """compute_bert_score should return precision, recall, f1."""
        # Mock torch tensors
        mock_p = MagicMock()
        mock_p.mean.return_value.item.return_value = 0.85
        mock_r = MagicMock()
        mock_r.mean.return_value.item.return_value = 0.80
        mock_f1 = MagicMock()
        mock_f1.mean.return_value.item.return_value = 0.82

        mock_bert_score.return_value = (mock_p, mock_r, mock_f1)

        evaluator = SemanticEvaluator()
        result = evaluator.compute_bert_score(
            candidates=["This is a test summary."],
            references=["This is the reference text."],
            lang="en",
        )

        assert isinstance(result, BERTScoreResult)
        assert result.precision == pytest.approx(0.85)
        assert result.recall == pytest.approx(0.80)
        assert result.f1 == pytest.approx(0.82)

    @patch("recap_subworker.services.semantic_eval.bert_score")
    def test_compute_bert_score_uses_correct_model_for_ja(self, mock_bert_score):
        """Should use Japanese model for lang='ja'."""
        mock_tensor = MagicMock()
        mock_tensor.mean.return_value.item.return_value = 0.75
        mock_bert_score.return_value = (mock_tensor, mock_tensor, mock_tensor)

        evaluator = SemanticEvaluator()
        evaluator.compute_bert_score(
            candidates=["これはテストです。"],
            references=["これは参照テキストです。"],
            lang="ja",
        )

        mock_bert_score.assert_called_once()
        call_kwargs = mock_bert_score.call_args[1]
        assert call_kwargs["model_type"] == "cl-tohoku/bert-base-japanese-v3"
        assert call_kwargs["lang"] == "ja"

    @patch("recap_subworker.services.semantic_eval.bert_score")
    def test_compute_bert_score_uses_correct_model_for_en(self, mock_bert_score):
        """Should use English model for lang='en'."""
        mock_tensor = MagicMock()
        mock_tensor.mean.return_value.item.return_value = 0.75
        mock_bert_score.return_value = (mock_tensor, mock_tensor, mock_tensor)

        evaluator = SemanticEvaluator()
        evaluator.compute_bert_score(
            candidates=["This is a test."],
            references=["This is reference."],
            lang="en",
        )

        mock_bert_score.assert_called_once()
        call_kwargs = mock_bert_score.call_args[1]
        assert call_kwargs["model_type"] == "microsoft/deberta-xlarge-mnli"
        assert call_kwargs["lang"] == "en"

    @patch("recap_subworker.services.semantic_eval.bert_score")
    def test_compute_bert_score_with_multiple_samples(self, mock_bert_score):
        """Should handle multiple candidate-reference pairs."""
        mock_p = MagicMock()
        mock_p.mean.return_value.item.return_value = 0.90
        mock_r = MagicMock()
        mock_r.mean.return_value.item.return_value = 0.88
        mock_f1 = MagicMock()
        mock_f1.mean.return_value.item.return_value = 0.89

        mock_bert_score.return_value = (mock_p, mock_r, mock_f1)

        evaluator = SemanticEvaluator()
        result = evaluator.compute_bert_score(
            candidates=["Summary 1.", "Summary 2.", "Summary 3."],
            references=["Reference 1.", "Reference 2.", "Reference 3."],
            lang="en",
        )

        # Verify all 3 pairs were passed
        call_args = mock_bert_score.call_args[0]
        assert len(call_args[0]) == 3  # candidates
        assert len(call_args[1]) == 3  # references

    @patch("recap_subworker.services.semantic_eval.bert_score")
    def test_compute_bert_score_with_rescale_baseline(self, mock_bert_score):
        """Should use rescale_with_baseline=True by default."""
        mock_tensor = MagicMock()
        mock_tensor.mean.return_value.item.return_value = 0.75
        mock_bert_score.return_value = (mock_tensor, mock_tensor, mock_tensor)

        evaluator = SemanticEvaluator()
        evaluator.compute_bert_score(
            candidates=["Test"],
            references=["Reference"],
            lang="en",
        )

        call_kwargs = mock_bert_score.call_args[1]
        assert call_kwargs["rescale_with_baseline"] is True

    @patch("recap_subworker.services.semantic_eval.bert_score")
    def test_compute_bert_score_individual_scores(self, mock_bert_score):
        """Should return individual scores when requested."""
        # Create mock tensors with tolist method
        mock_p = MagicMock()
        mock_p.mean.return_value.item.return_value = 0.85
        mock_p.tolist.return_value = [0.80, 0.85, 0.90]
        mock_r = MagicMock()
        mock_r.mean.return_value.item.return_value = 0.80
        mock_r.tolist.return_value = [0.75, 0.80, 0.85]
        mock_f1 = MagicMock()
        mock_f1.mean.return_value.item.return_value = 0.82
        mock_f1.tolist.return_value = [0.77, 0.82, 0.87]

        mock_bert_score.return_value = (mock_p, mock_r, mock_f1)

        evaluator = SemanticEvaluator()
        result = evaluator.compute_bert_score(
            candidates=["S1", "S2", "S3"],
            references=["R1", "R2", "R3"],
            lang="en",
            return_individual=True,
        )

        assert result.individual_scores is not None
        assert len(result.individual_scores) == 3
        assert result.individual_scores[0]["precision"] == pytest.approx(0.80)
        assert result.individual_scores[1]["f1"] == pytest.approx(0.82)

    def test_evaluate_summary_quality(self):
        """evaluate_summary_quality should compute scores for a summary."""
        with patch.object(SemanticEvaluator, "compute_bert_score") as mock_compute:
            mock_compute.return_value = BERTScoreResult(
                precision=0.85, recall=0.80, f1=0.82
            )

            evaluator = SemanticEvaluator()
            result = evaluator.evaluate_summary_quality(
                summary="AI技術が急速に発展している。",
                source_text="人工知能の技術は近年急速に発展を遂げている。",
                lang="ja",
            )

            assert result.f1 == pytest.approx(0.82)
            mock_compute.assert_called_once()


class TestBERTScoreResult:
    """Tests for BERTScoreResult dataclass."""

    def test_bert_score_result_creation(self):
        """Should create BERTScoreResult with required fields."""
        result = BERTScoreResult(precision=0.85, recall=0.80, f1=0.82)
        assert result.precision == 0.85
        assert result.recall == 0.80
        assert result.f1 == 0.82
        assert result.individual_scores is None

    def test_bert_score_result_with_individual_scores(self):
        """Should support individual scores."""
        individual = [
            {"precision": 0.80, "recall": 0.75, "f1": 0.77},
            {"precision": 0.90, "recall": 0.85, "f1": 0.87},
        ]
        result = BERTScoreResult(
            precision=0.85, recall=0.80, f1=0.82, individual_scores=individual
        )
        assert result.individual_scores == individual

    def test_bert_score_result_to_dict(self):
        """Should convert to dictionary."""
        result = BERTScoreResult(precision=0.85, recall=0.80, f1=0.82)
        d = result.to_dict()
        assert d["precision"] == 0.85
        assert d["recall"] == 0.80
        assert d["f1"] == 0.82
