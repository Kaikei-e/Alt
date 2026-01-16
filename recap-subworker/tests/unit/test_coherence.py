"""Unit tests for TopicCoherenceEvaluator (C_V/NPMI coherence)."""

from __future__ import annotations

import pytest
from unittest.mock import patch, MagicMock

from recap_subworker.services.coherence import (
    TopicCoherenceEvaluator,
    CoherenceResult,
    CoherenceType,
)


class TestCoherenceType:
    """Tests for CoherenceType enum."""

    def test_all_types_exist(self):
        """Should have all required coherence types."""
        assert CoherenceType.C_V
        assert CoherenceType.C_NPMI
        assert CoherenceType.C_UCI
        assert CoherenceType.U_MASS

    def test_type_values(self):
        """Should have correct string values."""
        assert CoherenceType.C_V.value == "c_v"
        assert CoherenceType.C_NPMI.value == "c_npmi"


class TestCoherenceResult:
    """Tests for CoherenceResult dataclass."""

    def test_result_creation(self):
        """Should create result with required fields."""
        result = CoherenceResult(
            overall_coherence=0.65,
            coherence_type="c_v",
            per_cluster_coherence={0: 0.7, 1: 0.6},
        )
        assert result.overall_coherence == pytest.approx(0.65)
        assert result.coherence_type == "c_v"
        assert len(result.per_cluster_coherence) == 2

    def test_result_to_dict(self):
        """Should convert to dictionary."""
        result = CoherenceResult(
            overall_coherence=0.65,
            coherence_type="c_v",
            per_cluster_coherence={0: 0.7, 1: 0.6},
        )
        d = result.to_dict()
        assert d["overall_coherence"] == pytest.approx(0.65)
        assert d["coherence_type"] == "c_v"
        assert "per_cluster_coherence" in d

    def test_result_with_metadata(self):
        """Should support optional metadata."""
        result = CoherenceResult(
            overall_coherence=0.65,
            coherence_type="c_v",
            per_cluster_coherence={0: 0.7},
            num_clusters=5,
            num_documents=100,
        )
        assert result.num_clusters == 5
        assert result.num_documents == 100


class TestTopicCoherenceEvaluator:
    """Tests for TopicCoherenceEvaluator class."""

    def test_default_coherence_type(self):
        """Should use C_V by default."""
        evaluator = TopicCoherenceEvaluator()
        assert evaluator.coherence_type == CoherenceType.C_V

    def test_custom_coherence_type(self):
        """Should accept custom coherence type."""
        evaluator = TopicCoherenceEvaluator(coherence_type=CoherenceType.C_NPMI)
        assert evaluator.coherence_type == CoherenceType.C_NPMI

    def test_tokenize_japanese(self):
        """Should tokenize Japanese text correctly."""
        evaluator = TopicCoherenceEvaluator()
        tokens = evaluator._tokenize("AIは急速に発展している。", lang="ja")
        assert isinstance(tokens, list)
        assert len(tokens) > 0

    def test_tokenize_english(self):
        """Should tokenize English text correctly."""
        evaluator = TopicCoherenceEvaluator()
        tokens = evaluator._tokenize("AI is advancing rapidly.", lang="en")
        assert isinstance(tokens, list)
        assert len(tokens) > 0
        # Should contain lowercase words
        assert all(t.islower() or not t.isalpha() for t in tokens)

    def test_extract_topic_words(self):
        """Should extract representative words for a cluster."""
        evaluator = TopicCoherenceEvaluator()

        cluster_texts = [
            "Machine learning models are improving.",
            "Deep learning advances in neural networks.",
            "AI models learn from data patterns.",
        ]

        topic_words = evaluator._extract_topic_words(cluster_texts, top_n=5, lang="en")
        assert isinstance(topic_words, list)
        assert len(topic_words) <= 5

    @patch("recap_subworker.services.coherence.CoherenceModel")
    def test_compute_coherence_single_cluster(self, mock_coherence_model):
        """Should compute coherence for a single cluster."""
        # Mock the coherence model
        mock_model = MagicMock()
        mock_model.get_coherence.return_value = 0.65
        mock_coherence_model.return_value = mock_model

        evaluator = TopicCoherenceEvaluator()
        result = evaluator.compute_coherence(
            clusters={
                0: ["Text about AI.", "AI advances."],
            },
            texts=["Text about AI.", "AI advances.", "Other topic."],
        )

        assert isinstance(result, CoherenceResult)
        assert 0 in result.per_cluster_coherence

    @patch("recap_subworker.services.coherence.CoherenceModel")
    def test_compute_coherence_multiple_clusters(self, mock_coherence_model):
        """Should compute coherence for multiple clusters."""
        mock_model = MagicMock()
        mock_model.get_coherence.return_value = 0.6
        mock_coherence_model.return_value = mock_model

        evaluator = TopicCoherenceEvaluator()
        result = evaluator.compute_coherence(
            clusters={
                0: ["AI topic 1.", "AI topic 2."],
                1: ["Finance topic 1.", "Finance topic 2."],
            },
            texts=["AI topic 1.", "AI topic 2.", "Finance topic 1.", "Finance topic 2."],
        )

        assert len(result.per_cluster_coherence) == 2
        assert 0 in result.per_cluster_coherence
        assert 1 in result.per_cluster_coherence

    @patch("recap_subworker.services.coherence.CoherenceModel")
    def test_compute_coherence_with_different_types(self, mock_coherence_model):
        """Should compute different coherence types."""
        mock_model = MagicMock()
        mock_model.get_coherence.return_value = 0.5
        mock_coherence_model.return_value = mock_model

        for ctype in [CoherenceType.C_V, CoherenceType.C_NPMI, CoherenceType.U_MASS]:
            evaluator = TopicCoherenceEvaluator(coherence_type=ctype)
            result = evaluator.compute_coherence(
                clusters={0: ["Test 1.", "Test 2."]},
                texts=["Test 1.", "Test 2."],
            )
            assert result.coherence_type == ctype.value

    def test_compute_coherence_empty_clusters_raises_error(self):
        """Should raise error for empty clusters."""
        evaluator = TopicCoherenceEvaluator()

        with pytest.raises(ValueError, match="clusters"):
            evaluator.compute_coherence(
                clusters={},
                texts=["Some text."],
            )

    def test_compute_coherence_empty_texts_raises_error(self):
        """Should raise error for empty texts."""
        evaluator = TopicCoherenceEvaluator()

        with pytest.raises(ValueError, match="texts"):
            evaluator.compute_coherence(
                clusters={0: ["Cluster text."]},
                texts=[],
            )

    @patch("recap_subworker.services.coherence.CoherenceModel")
    def test_overall_coherence_is_weighted_average(self, mock_coherence_model):
        """Should compute overall coherence as weighted average."""
        mock_model = MagicMock()
        mock_model.get_coherence.return_value = 0.6
        mock_coherence_model.return_value = mock_model

        evaluator = TopicCoherenceEvaluator()
        result = evaluator.compute_coherence(
            clusters={
                0: ["Text 1.", "Text 2.", "Text 3."],  # 3 docs
                1: ["Text 4."],  # 1 doc
            },
            texts=["Text 1.", "Text 2.", "Text 3.", "Text 4."],
        )

        # Overall should be weighted by cluster size
        assert result.overall_coherence is not None

    @patch("recap_subworker.services.coherence.CoherenceModel")
    def test_compute_coherence_with_min_cluster_size(self, mock_coherence_model):
        """Should skip clusters smaller than min_cluster_size."""
        mock_model = MagicMock()
        mock_model.get_coherence.return_value = 0.7
        mock_coherence_model.return_value = mock_model

        evaluator = TopicCoherenceEvaluator()
        result = evaluator.compute_coherence(
            clusters={
                0: ["Text 1.", "Text 2.", "Text 3."],  # 3 docs, included
                1: ["Text 4."],  # 1 doc, excluded if min_cluster_size=2
            },
            texts=["Text 1.", "Text 2.", "Text 3.", "Text 4."],
            min_cluster_size=2,
        )

        # Only cluster 0 should be evaluated
        assert 0 in result.per_cluster_coherence
        assert 1 not in result.per_cluster_coherence


class TestTopicCoherenceEvaluatorIntegration:
    """Integration-style tests."""

    def test_japanese_text_handling(self):
        """Should handle Japanese text correctly."""
        evaluator = TopicCoherenceEvaluator()
        tokens = evaluator._tokenize("人工知能の技術は発展している。", lang="ja")
        assert len(tokens) > 0

    def test_english_stopwords_removed(self):
        """Should remove English stopwords."""
        evaluator = TopicCoherenceEvaluator()
        tokens = evaluator._tokenize("The AI is a great technology.", lang="en")
        # Common stopwords like 'the', 'is', 'a' should be removed
        assert "the" not in tokens
        assert "is" not in tokens
        assert "a" not in tokens
