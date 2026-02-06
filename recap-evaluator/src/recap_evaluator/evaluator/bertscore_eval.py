"""BERTScore-based semantic evaluation for summarization quality.

This module provides semantic similarity evaluation using BERTScore,
which leverages contextual embeddings from BERT models to compute
similarity between generated summaries and reference texts.

References:
- BERTScore Paper: https://arxiv.org/abs/1904.09675
- tohoku-nlp/bert-base-japanese-v3: https://github.com/cl-tohoku/bert-japanese
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal

import structlog

try:
    from bert_score import score as bert_score

    BERT_SCORE_AVAILABLE = True
except ImportError:
    bert_score = None
    BERT_SCORE_AVAILABLE = False

logger = structlog.get_logger(__name__)


@dataclass
class BERTScoreResult:
    """Result container for BERTScore computation.

    Attributes:
        precision: Average precision score across all pairs.
        recall: Average recall score across all pairs.
        f1: Average F1 score across all pairs.
        individual_scores: Per-pair scores if requested.
    """

    precision: float
    recall: float
    f1: float
    individual_scores: list[dict[str, float]] | None = field(default=None)

    def to_dict(self) -> dict:
        """Convert to dictionary representation."""
        result = {
            "precision": self.precision,
            "recall": self.recall,
            "f1": self.f1,
        }
        if self.individual_scores is not None:
            result["individual_scores"] = self.individual_scores
        return result


class BERTScoreEvaluator:
    """BERTScore-based semantic evaluator for text summarization.

    Uses language-specific BERT models for accurate semantic similarity:
    - Japanese: cl-tohoku/bert-base-japanese-v3
    - English: microsoft/deberta-xlarge-mnli

    Example:
        >>> evaluator = BERTScoreEvaluator()
        >>> result = evaluator.compute_bert_score(
        ...     candidates=["AI技術が発展している。"],
        ...     references=["人工知能の技術は発展を遂げている。"],
        ...     lang="ja"
        ... )
        >>> print(f"F1: {result.f1:.3f}")
    """

    MODEL_MAP: dict[str, str] = {
        "ja": "cl-tohoku/bert-base-japanese-v3",
        "en": "microsoft/deberta-xlarge-mnli",
    }

    def __init__(
        self,
        batch_size: int = 32,
        device: str | None = None,
        use_fast_tokenizer: bool = True,
    ):
        """Initialize the BERTScore evaluator.

        Args:
            batch_size: Batch size for BERTScore computation.
            device: Device to use ('cuda', 'cpu', or None for auto).
            use_fast_tokenizer: Whether to use fast tokenizer.
        """
        if not BERT_SCORE_AVAILABLE:
            logger.warning(
                "bert-score not available",
                hint="Install with: pip install bert-score",
            )

        self.batch_size = batch_size
        self.device = device
        self.use_fast_tokenizer = use_fast_tokenizer

    def compute_bert_score(
        self,
        candidates: list[str],
        references: list[str],
        lang: Literal["ja", "en"] = "ja",
        rescale_with_baseline: bool = True,
        return_individual: bool = False,
    ) -> BERTScoreResult:
        """Compute BERTScore between candidate and reference texts.

        Args:
            candidates: List of generated/candidate texts.
            references: List of reference texts (same length as candidates).
            lang: Language code ('ja' or 'en').
            rescale_with_baseline: Whether to rescale scores with baseline.
            return_individual: Whether to return per-pair scores.

        Returns:
            BERTScoreResult containing precision, recall, F1 scores.

        Raises:
            ValueError: If candidates and references have different lengths.
            ValueError: If language is not supported.
            RuntimeError: If bert-score is not available.
        """
        if not BERT_SCORE_AVAILABLE:
            raise RuntimeError(
                "bert-score is not installed. Install with: pip install bert-score"
            )

        if len(candidates) != len(references):
            raise ValueError(
                f"Mismatched lengths: {len(candidates)} candidates vs {len(references)} references"
            )

        model_type = self.MODEL_MAP.get(lang)
        if model_type is None:
            raise ValueError(
                f"Unsupported language: {lang}. Supported: {list(self.MODEL_MAP.keys())}"
            )

        logger.debug(
            "Computing BERTScore",
            model=model_type,
            lang=lang,
            num_pairs=len(candidates),
            batch_size=self.batch_size,
        )

        P, R, F1 = bert_score(
            candidates,
            references,
            model_type=model_type,
            lang=lang,
            rescale_with_baseline=rescale_with_baseline,
            batch_size=self.batch_size,
            device=self.device,
            use_fast_tokenizer=self.use_fast_tokenizer,
            verbose=False,
        )

        result = BERTScoreResult(
            precision=P.mean().item(),
            recall=R.mean().item(),
            f1=F1.mean().item(),
        )

        if return_individual:
            p_list = P.tolist()
            r_list = R.tolist()
            f1_list = F1.tolist()
            result.individual_scores = [
                {"precision": p, "recall": r, "f1": f}
                for p, r, f in zip(p_list, r_list, f1_list)
            ]

        logger.info(
            "BERTScore computed",
            precision=f"{result.precision:.4f}",
            recall=f"{result.recall:.4f}",
            f1=f"{result.f1:.4f}",
            lang=lang,
        )

        return result

    def evaluate_summary_quality(
        self,
        summary: str,
        source_text: str,
        lang: Literal["ja", "en"] = "ja",
    ) -> BERTScoreResult:
        """Evaluate the quality of a single summary against source text.

        This is a convenience method for evaluating a single summary.

        Args:
            summary: The generated summary text.
            source_text: The original source text.
            lang: Language code ('ja' or 'en').

        Returns:
            BERTScoreResult for the summary-source pair.
        """
        return self.compute_bert_score(
            candidates=[summary],
            references=[source_text],
            lang=lang,
        )

    def evaluate_batch(
        self,
        summaries: list[str],
        sources: list[str],
        lang: Literal["ja", "en"] = "ja",
    ) -> dict[str, float]:
        """Evaluate a batch of summaries and return aggregate metrics.

        Args:
            summaries: List of generated summaries.
            sources: List of source texts.
            lang: Language code ('ja' or 'en').

        Returns:
            Dictionary with aggregate metrics.
        """
        result = self.compute_bert_score(
            candidates=summaries,
            references=sources,
            lang=lang,
            return_individual=True,
        )

        return {
            "mean_precision": result.precision,
            "mean_recall": result.recall,
            "mean_f1": result.f1,
            "num_samples": len(summaries),
            "individual_scores": result.individual_scores,
        }


# Singleton instance
bertscore_evaluator = BERTScoreEvaluator()
