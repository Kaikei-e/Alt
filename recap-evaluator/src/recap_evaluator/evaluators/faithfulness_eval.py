"""NLI-based faithfulness evaluation for summarization.

This module provides faithfulness evaluation using Natural Language Inference (NLI)
models to verify if generated summaries are supported by source texts (hallucination detection).

References:
- HaluGate: https://blog.vllm.ai/2025/12/14/halugate.html
- ModernBERT-base-nli: https://huggingface.co/tasksource/ModernBERT-base-nli
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import Optional

import structlog

try:
    from transformers import pipeline

    TRANSFORMERS_AVAILABLE = True
except ImportError:
    pipeline = None
    TRANSFORMERS_AVAILABLE = False

logger = structlog.get_logger(__name__)


@dataclass
class SentenceResult:
    """Result for a single sentence's faithfulness check.

    Attributes:
        sentence: The sentence being evaluated.
        is_supported: Whether the sentence is supported by source.
        entailment_score: NLI entailment probability.
        contradiction_score: NLI contradiction probability.
        neutral_score: NLI neutral probability.
    """

    sentence: str
    is_supported: bool
    entailment_score: float
    contradiction_score: float
    neutral_score: float

    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "sentence": self.sentence,
            "is_supported": self.is_supported,
            "entailment_score": self.entailment_score,
            "contradiction_score": self.contradiction_score,
            "neutral_score": self.neutral_score,
        }


@dataclass
class FaithfulnessResult:
    """Result container for faithfulness evaluation.

    Attributes:
        is_hallucinated: Whether the summary contains hallucinations.
        hallucination_score: Overall hallucination score (0-1, higher = more hallucinated).
        faithfulness_score: Overall faithfulness score (0-1, higher = more faithful).
        entailment_score: Average entailment score.
        contradiction_score: Average contradiction score.
        neutral_score: Average neutral score.
        sentence_results: Per-sentence results if requested.
    """

    is_hallucinated: bool
    hallucination_score: float
    faithfulness_score: float
    entailment_score: float
    contradiction_score: float
    neutral_score: float
    sentence_results: Optional[list[SentenceResult]] = field(default=None)

    def to_dict(self) -> dict:
        """Convert to dictionary representation."""
        result = {
            "is_hallucinated": self.is_hallucinated,
            "hallucination_score": self.hallucination_score,
            "faithfulness_score": self.faithfulness_score,
            "entailment_score": self.entailment_score,
            "contradiction_score": self.contradiction_score,
            "neutral_score": self.neutral_score,
        }
        if self.sentence_results is not None:
            result["sentence_results"] = [s.to_dict() for s in self.sentence_results]
        return result


class FaithfulnessEvaluator:
    """NLI-based faithfulness evaluator for summarization.

    Uses Natural Language Inference to check if summary sentences
    are entailed by (supported by) the source text.

    Example:
        >>> evaluator = FaithfulnessEvaluator()
        >>> result = evaluator.detect(
        ...     summary="AI technology is declining in 2025.",
        ...     source_sentences=["AI technology is advancing rapidly in 2025."],
        ... )
        >>> print(f"Hallucinated: {result.is_hallucinated}")
    """

    DEFAULT_MODEL = "tasksource/ModernBERT-base-nli"

    # Label mapping for different NLI models
    LABEL_MAPS = {
        "default": {
            "entailment": "entailment",
            "contradiction": "contradiction",
            "neutral": "neutral",
        },
        "bart-mnli": {
            "entailment": "ENTAILMENT",
            "contradiction": "CONTRADICTION",
            "neutral": "NEUTRAL",
        },
    }

    def __init__(
        self,
        model_name: Optional[str] = None,
        device: Optional[str] = None,
    ):
        """Initialize the faithfulness evaluator.

        Args:
            model_name: HuggingFace model name for NLI (default: ModernBERT-base-nli).
            device: Device to use ('cuda', 'cpu', or None for auto).
        """
        self._model_name = model_name or self.DEFAULT_MODEL
        self.device = device
        self._nli = None
        self._initialized = False

        if not TRANSFORMERS_AVAILABLE:
            logger.warning(
                "transformers not available",
                hint="Install with: pip install transformers",
            )

        # Detect label format based on model
        self._label_map = self.LABEL_MAPS["default"]
        if model_name and "bart" in model_name.lower():
            self._label_map = self.LABEL_MAPS["bart-mnli"]

    def _ensure_initialized(self):
        """Lazily initialize the NLI pipeline when first needed."""
        if self._initialized:
            return

        if not TRANSFORMERS_AVAILABLE:
            raise RuntimeError("transformers not available")

        logger.info("Initializing NLI pipeline", model=self._model_name)
        self._nli = pipeline(
            "text-classification",
            model=self._model_name,
            top_k=None,  # Return all labels with scores
            device=self.device,
        )
        self._initialized = True

    @property
    def model_name(self) -> str:
        """Get the model name."""
        return self._model_name

    def _split_sentences(self, text: str) -> list[str]:
        """Split text into sentences.

        Handles both English and Japanese sentence boundaries.
        """
        # Pattern for sentence boundaries (. ! ? and Japanese 。！？)
        pattern = r"(?<=[.!?。！？])\s*"
        sentences = re.split(pattern, text.strip())
        return [s.strip() for s in sentences if s.strip()]

    def _get_nli_scores(self, premise: str, hypothesis: str) -> dict[str, float]:
        """Get NLI scores for a premise-hypothesis pair.

        Args:
            premise: The source/premise text.
            hypothesis: The claim/hypothesis to verify.

        Returns:
            Dictionary with entailment, contradiction, neutral scores.
        """
        self._ensure_initialized()
        if self._nli is None:
            raise RuntimeError("NLI pipeline not available")

        # Format input for NLI
        nli_input = f"{premise}</s></s>{hypothesis}"
        results = self._nli(nli_input)

        # Extract scores by label
        scores = {"entailment": 0.0, "contradiction": 0.0, "neutral": 0.0}
        result_list = results[0] if isinstance(results[0], list) else results
        for item in result_list:
            label = item["label"].lower()
            for key, mapped_label in self._label_map.items():
                if label == mapped_label.lower():
                    scores[key] = item["score"]
                    break

        return scores

    def detect(
        self,
        summary: str,
        source_sentences: list[str],
        threshold: float = 0.5,
        return_sentence_results: bool = False,
    ) -> FaithfulnessResult:
        """Detect hallucinations in a summary.

        Uses NLI to check if each summary sentence is entailed by
        at least one source sentence (max-pooling strategy).

        Args:
            summary: The generated summary to check.
            source_sentences: List of source sentences for verification.
            threshold: Entailment threshold (0-1). Below this is considered hallucinated.
            return_sentence_results: Whether to include per-sentence results.

        Returns:
            FaithfulnessResult with detection results.

        Raises:
            ValueError: If summary or source_sentences is empty.
        """
        if not summary or not summary.strip():
            raise ValueError("summary cannot be empty")
        if not source_sentences:
            raise ValueError("source_sentences cannot be empty")

        summary_sentences = self._split_sentences(summary)
        sentence_results: list[SentenceResult] = []

        total_entailment = 0.0
        total_contradiction = 0.0
        total_neutral = 0.0
        num_hallucinated = 0

        for sent in summary_sentences:
            # Get max entailment score across all source sentences
            best_scores = {"entailment": 0.0, "contradiction": 1.0, "neutral": 0.0}

            for source in source_sentences:
                scores = self._get_nli_scores(premise=source, hypothesis=sent)
                # Max-pooling: take highest entailment across sources
                if scores["entailment"] > best_scores["entailment"]:
                    best_scores = scores

            is_supported = best_scores["entailment"] >= threshold
            if not is_supported:
                num_hallucinated += 1

            total_entailment += best_scores["entailment"]
            total_contradiction += best_scores["contradiction"]
            total_neutral += best_scores["neutral"]

            if return_sentence_results:
                sentence_results.append(
                    SentenceResult(
                        sentence=sent,
                        is_supported=is_supported,
                        entailment_score=best_scores["entailment"],
                        contradiction_score=best_scores["contradiction"],
                        neutral_score=best_scores["neutral"],
                    )
                )

        num_sentences = len(summary_sentences)
        avg_entailment = total_entailment / num_sentences if num_sentences > 0 else 0.0
        avg_contradiction = (
            total_contradiction / num_sentences if num_sentences > 0 else 0.0
        )
        avg_neutral = total_neutral / num_sentences if num_sentences > 0 else 0.0

        # Hallucination score: 1 - entailment (higher means more hallucinated)
        hallucination_score = 1.0 - avg_entailment
        faithfulness_score = avg_entailment
        is_hallucinated = avg_entailment < threshold

        logger.debug(
            "Faithfulness detection complete",
            num_sentences=num_sentences,
            num_hallucinated=num_hallucinated,
            avg_entailment=f"{avg_entailment:.4f}",
            is_hallucinated=is_hallucinated,
        )

        return FaithfulnessResult(
            is_hallucinated=is_hallucinated,
            hallucination_score=hallucination_score,
            faithfulness_score=faithfulness_score,
            entailment_score=avg_entailment,
            contradiction_score=avg_contradiction,
            neutral_score=avg_neutral,
            sentence_results=sentence_results if return_sentence_results else None,
        )

    def detect_batch(
        self,
        summaries: list[str],
        sources: list[list[str]],
        threshold: float = 0.5,
    ) -> list[FaithfulnessResult]:
        """Detect hallucinations in a batch of summaries.

        Args:
            summaries: List of summaries to check.
            sources: List of source sentence lists (one per summary).
            threshold: Entailment threshold.

        Returns:
            List of FaithfulnessResult for each summary.

        Raises:
            ValueError: If summaries and sources have different lengths.
        """
        if len(summaries) != len(sources):
            raise ValueError(
                f"Mismatched lengths: {len(summaries)} summaries vs {len(sources)} sources"
            )

        results = []
        for summary, source_sentences in zip(summaries, sources):
            result = self.detect(
                summary=summary,
                source_sentences=source_sentences,
                threshold=threshold,
            )
            results.append(result)

        return results


# Singleton instance
faithfulness_evaluator = FaithfulnessEvaluator()
