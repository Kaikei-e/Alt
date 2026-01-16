"""Unit tests for HallucinationDetector (NLI-based detection)."""

from __future__ import annotations

import pytest
from unittest.mock import patch, MagicMock

from recap_subworker.services.hallucination_detector import (
    HallucinationDetector,
    HallucinationResult,
    SentenceResult,
)


class TestHallucinationResult:
    """Tests for HallucinationResult dataclass."""

    def test_result_creation(self):
        """Should create result with required fields."""
        result = HallucinationResult(
            is_hallucinated=False,
            hallucination_score=0.1,
            entailment_score=0.85,
            contradiction_score=0.05,
            neutral_score=0.10,
        )
        assert result.is_hallucinated is False
        assert result.hallucination_score == pytest.approx(0.1)
        assert result.entailment_score == pytest.approx(0.85)

    def test_result_with_sentence_results(self):
        """Should support per-sentence results."""
        sentence_results = [
            SentenceResult(
                sentence="AI is advancing rapidly.",
                is_supported=True,
                entailment_score=0.9,
                contradiction_score=0.05,
                neutral_score=0.05,
            ),
        ]
        result = HallucinationResult(
            is_hallucinated=False,
            hallucination_score=0.1,
            entailment_score=0.85,
            contradiction_score=0.05,
            neutral_score=0.10,
            sentence_results=sentence_results,
        )
        assert result.sentence_results is not None
        assert len(result.sentence_results) == 1

    def test_result_to_dict(self):
        """Should convert to dictionary."""
        result = HallucinationResult(
            is_hallucinated=True,
            hallucination_score=0.6,
            entailment_score=0.2,
            contradiction_score=0.6,
            neutral_score=0.2,
        )
        d = result.to_dict()
        assert d["is_hallucinated"] is True
        assert d["hallucination_score"] == pytest.approx(0.6)


class TestSentenceResult:
    """Tests for SentenceResult dataclass."""

    def test_sentence_result_creation(self):
        """Should create sentence result."""
        result = SentenceResult(
            sentence="Test sentence.",
            is_supported=True,
            entailment_score=0.9,
            contradiction_score=0.05,
            neutral_score=0.05,
        )
        assert result.sentence == "Test sentence."
        assert result.is_supported is True


class TestHallucinationDetector:
    """Tests for HallucinationDetector class."""

    def test_default_model_name(self):
        """Should use default NLI model."""
        with patch("recap_subworker.services.hallucination_detector.pipeline") as mock_pipeline:
            detector = HallucinationDetector()
            # The pipeline should be created with the default model
            assert detector.model_name == "tasksource/ModernBERT-base-nli"

    def test_custom_model_name(self):
        """Should accept custom model name."""
        with patch("recap_subworker.services.hallucination_detector.pipeline"):
            detector = HallucinationDetector(model_name="facebook/bart-large-mnli")
            assert detector.model_name == "facebook/bart-large-mnli"

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_detect_no_hallucination(self, mock_pipeline):
        """Should detect no hallucination when summary is supported."""
        # Mock NLI pipeline response for entailment
        mock_nli = MagicMock()
        mock_nli.return_value = [
            [
                {"label": "entailment", "score": 0.9},
                {"label": "neutral", "score": 0.08},
                {"label": "contradiction", "score": 0.02},
            ]
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()
        result = detector.detect(
            summary="AI technology is advancing.",
            source_sentences=["Artificial intelligence technology is making rapid progress."],
        )

        assert result.is_hallucinated is False
        assert result.entailment_score >= 0.5

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_detect_hallucination(self, mock_pipeline):
        """Should detect hallucination when summary contradicts source."""
        mock_nli = MagicMock()
        mock_nli.return_value = [
            [
                {"label": "contradiction", "score": 0.85},
                {"label": "neutral", "score": 0.10},
                {"label": "entailment", "score": 0.05},
            ]
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()
        result = detector.detect(
            summary="AI technology is declining.",
            source_sentences=["AI technology is advancing rapidly."],
        )

        assert result.is_hallucinated is True
        assert result.contradiction_score >= 0.5

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_detect_with_multiple_source_sentences(self, mock_pipeline):
        """Should aggregate scores across multiple source sentences."""
        mock_nli = MagicMock()
        # Return different scores for each source sentence
        mock_nli.side_effect = [
            [[{"label": "entailment", "score": 0.7}, {"label": "neutral", "score": 0.2}, {"label": "contradiction", "score": 0.1}]],
            [[{"label": "entailment", "score": 0.8}, {"label": "neutral", "score": 0.15}, {"label": "contradiction", "score": 0.05}]],
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()
        result = detector.detect(
            summary="AI advances in multiple areas.",
            source_sentences=[
                "AI is improving in natural language processing.",
                "Machine learning has made significant progress.",
            ],
        )

        # Should take max entailment across sources
        assert result.is_hallucinated is False

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_detect_with_threshold(self, mock_pipeline):
        """Should respect custom threshold."""
        mock_nli = MagicMock()
        mock_nli.return_value = [
            [
                {"label": "entailment", "score": 0.45},
                {"label": "neutral", "score": 0.40},
                {"label": "contradiction", "score": 0.15},
            ]
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()

        # With default threshold (0.5), this should be hallucinated (low entailment)
        result_default = detector.detect(
            summary="Some claim.",
            source_sentences=["Source text."],
            threshold=0.5,
        )
        assert result_default.is_hallucinated is True

        # With lower threshold (0.4), this should NOT be hallucinated
        result_low = detector.detect(
            summary="Some claim.",
            source_sentences=["Source text."],
            threshold=0.4,
        )
        assert result_low.is_hallucinated is False

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_detect_returns_sentence_results(self, mock_pipeline):
        """Should return per-sentence results when requested."""
        mock_nli = MagicMock()
        mock_nli.return_value = [
            [
                {"label": "entailment", "score": 0.8},
                {"label": "neutral", "score": 0.15},
                {"label": "contradiction", "score": 0.05},
            ]
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()
        result = detector.detect(
            summary="First sentence. Second sentence.",
            source_sentences=["Source content."],
            return_sentence_results=True,
        )

        assert result.sentence_results is not None
        assert len(result.sentence_results) >= 1

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_detect_batch(self, mock_pipeline):
        """Should detect hallucinations in batch."""
        mock_nli = MagicMock()
        mock_nli.return_value = [
            [{"label": "entailment", "score": 0.9}, {"label": "neutral", "score": 0.08}, {"label": "contradiction", "score": 0.02}]
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()
        results = detector.detect_batch(
            summaries=["Summary 1.", "Summary 2."],
            sources=[["Source 1."], ["Source 2."]],
        )

        assert len(results) == 2
        assert all(isinstance(r, HallucinationResult) for r in results)


class TestHallucinationDetectorIntegration:
    """Integration-style tests (with mocked pipeline)."""

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_japanese_text_handling(self, mock_pipeline):
        """Should handle Japanese text correctly."""
        mock_nli = MagicMock()
        mock_nli.return_value = [
            [
                {"label": "entailment", "score": 0.85},
                {"label": "neutral", "score": 0.10},
                {"label": "contradiction", "score": 0.05},
            ]
        ]
        mock_pipeline.return_value = mock_nli

        detector = HallucinationDetector()
        result = detector.detect(
            summary="AIは急速に発展している。",
            source_sentences=["人工知能の技術は近年急速に発展を遂げている。"],
        )

        assert result.is_hallucinated is False

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_empty_source_raises_error(self, mock_pipeline):
        """Should raise error for empty source sentences."""
        detector = HallucinationDetector()

        with pytest.raises(ValueError, match="source_sentences"):
            detector.detect(
                summary="Some summary.",
                source_sentences=[],
            )

    @patch("recap_subworker.services.hallucination_detector.pipeline")
    def test_empty_summary_raises_error(self, mock_pipeline):
        """Should raise error for empty summary."""
        detector = HallucinationDetector()

        with pytest.raises(ValueError, match="summary"):
            detector.detect(
                summary="",
                source_sentences=["Source text."],
            )
