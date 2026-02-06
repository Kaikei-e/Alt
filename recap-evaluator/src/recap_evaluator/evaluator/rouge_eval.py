"""ROUGE-based evaluation for text summarization.

This module provides ROUGE (Recall-Oriented Understudy for Gisting Evaluation)
metrics for evaluating summary quality through n-gram overlap comparison.

References:
- ROUGE Paper: https://aclanthology.org/W04-1013/
- rouge-score library: https://github.com/google-research/google-research/tree/master/rouge
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Literal

import structlog

try:
    from rouge_score import rouge_scorer
    from rouge_score.tokenizers import Tokenizer

    ROUGE_AVAILABLE = True
except ImportError:
    rouge_scorer = None
    Tokenizer = None
    ROUGE_AVAILABLE = False

logger = structlog.get_logger(__name__)


class JapaneseTokenizer:
    """Simple character-based tokenizer for Japanese text.

    ROUGE-score doesn't natively support Japanese well, so we use
    character-level tokenization for Japanese text.
    """

    def tokenize(self, text: str) -> list[str]:
        """Tokenize text into characters (for Japanese)."""
        # Remove spaces and split into characters
        return list(text.replace(" ", "").replace("　", ""))


@dataclass
class ROUGEResult:
    """Result container for ROUGE computation.

    Attributes:
        rouge_1_precision: ROUGE-1 precision (unigram).
        rouge_1_recall: ROUGE-1 recall.
        rouge_1_f1: ROUGE-1 F1 score.
        rouge_2_precision: ROUGE-2 precision (bigram).
        rouge_2_recall: ROUGE-2 recall.
        rouge_2_f1: ROUGE-2 F1 score.
        rouge_l_precision: ROUGE-L precision (longest common subsequence).
        rouge_l_recall: ROUGE-L recall.
        rouge_l_f1: ROUGE-L F1 score.
    """

    rouge_1_precision: float = 0.0
    rouge_1_recall: float = 0.0
    rouge_1_f1: float = 0.0
    rouge_2_precision: float = 0.0
    rouge_2_recall: float = 0.0
    rouge_2_f1: float = 0.0
    rouge_l_precision: float = 0.0
    rouge_l_recall: float = 0.0
    rouge_l_f1: float = 0.0

    def to_dict(self) -> dict[str, float]:
        """Convert to dictionary representation."""
        return {
            "rouge_1_precision": self.rouge_1_precision,
            "rouge_1_recall": self.rouge_1_recall,
            "rouge_1_f1": self.rouge_1_f1,
            "rouge_2_precision": self.rouge_2_precision,
            "rouge_2_recall": self.rouge_2_recall,
            "rouge_2_f1": self.rouge_2_f1,
            "rouge_l_precision": self.rouge_l_precision,
            "rouge_l_recall": self.rouge_l_recall,
            "rouge_l_f1": self.rouge_l_f1,
        }


class ROUGEEvaluator:
    """ROUGE-based evaluator for text summarization quality.

    Computes ROUGE-1, ROUGE-2, and ROUGE-L scores between candidate
    summaries and reference texts.

    Example:
        >>> evaluator = ROUGEEvaluator()
        >>> result = evaluator.compute_rouge(
        ...     candidate="AI技術が発展している。",
        ...     reference="人工知能の技術は発展を遂げている。",
        ...     lang="ja"
        ... )
        >>> print(f"ROUGE-L F1: {result.rouge_l_f1:.3f}")
    """

    def __init__(self, use_stemmer: bool = False):
        """Initialize the ROUGE evaluator.

        Args:
            use_stemmer: Whether to use stemming (only for English).
        """
        if not ROUGE_AVAILABLE:
            logger.warning(
                "rouge-score not available",
                hint="Install with: pip install rouge-score",
            )

        self.use_stemmer = use_stemmer
        self._english_scorer: rouge_scorer.RougeScorer | None = None
        self._japanese_scorer: rouge_scorer.RougeScorer | None = None

    def _get_scorer(self, lang: Literal["en", "ja"]) -> rouge_scorer.RougeScorer:
        """Get or create a ROUGE scorer for the specified language."""
        if not ROUGE_AVAILABLE:
            raise RuntimeError(
                "rouge-score is not installed. Install with: pip install rouge-score"
            )

        if lang == "ja":
            if self._japanese_scorer is None:
                self._japanese_scorer = rouge_scorer.RougeScorer(
                    ["rouge1", "rouge2", "rougeL"],
                    use_stemmer=False,  # No stemming for Japanese
                    tokenizer=JapaneseTokenizer(),
                )
            return self._japanese_scorer
        else:
            if self._english_scorer is None:
                self._english_scorer = rouge_scorer.RougeScorer(
                    ["rouge1", "rouge2", "rougeL"],
                    use_stemmer=self.use_stemmer,
                )
            return self._english_scorer

    def compute_rouge(
        self,
        candidate: str,
        reference: str,
        lang: Literal["en", "ja"] = "en",
    ) -> ROUGEResult:
        """Compute ROUGE scores between candidate and reference text.

        Args:
            candidate: The generated/candidate text (summary).
            reference: The reference text (source or gold summary).
            lang: Language code ('en' or 'ja').

        Returns:
            ROUGEResult containing precision, recall, and F1 for ROUGE-1/2/L.
        """
        # Handle empty candidate
        if not candidate or not candidate.strip():
            logger.debug("Empty candidate text, returning zero scores")
            return ROUGEResult()

        # Handle empty reference
        if not reference or not reference.strip():
            logger.debug("Empty reference text, returning zero scores")
            return ROUGEResult()

        scorer = self._get_scorer(lang)
        scores = scorer.score(reference, candidate)

        result = ROUGEResult(
            rouge_1_precision=scores["rouge1"].precision,
            rouge_1_recall=scores["rouge1"].recall,
            rouge_1_f1=scores["rouge1"].fmeasure,
            rouge_2_precision=scores["rouge2"].precision,
            rouge_2_recall=scores["rouge2"].recall,
            rouge_2_f1=scores["rouge2"].fmeasure,
            rouge_l_precision=scores["rougeL"].precision,
            rouge_l_recall=scores["rougeL"].recall,
            rouge_l_f1=scores["rougeL"].fmeasure,
        )

        logger.debug(
            "ROUGE computed",
            rouge_1_f1=f"{result.rouge_1_f1:.4f}",
            rouge_2_f1=f"{result.rouge_2_f1:.4f}",
            rouge_l_f1=f"{result.rouge_l_f1:.4f}",
            lang=lang,
        )

        return result

    def compute_batch(
        self,
        candidates: list[str],
        references: list[str],
        lang: Literal["en", "ja"] = "en",
        return_individual: bool = False,
    ) -> dict:
        """Compute ROUGE scores for a batch of candidate-reference pairs.

        Args:
            candidates: List of generated/candidate texts.
            references: List of reference texts (same length as candidates).
            lang: Language code ('en' or 'ja').
            return_individual: Whether to return per-pair scores.

        Returns:
            Dictionary with average scores and optionally individual scores.

        Raises:
            ValueError: If candidates and references have different lengths.
        """
        if len(candidates) != len(references):
            raise ValueError(
                f"Mismatched lengths: {len(candidates)} candidates vs {len(references)} references"
            )

        if not candidates:
            return {
                "rouge_1_f1": 0.0,
                "rouge_2_f1": 0.0,
                "rouge_l_f1": 0.0,
                "num_samples": 0,
            }

        individual_scores: list[dict[str, float]] = []
        totals = {
            "rouge_1_precision": 0.0,
            "rouge_1_recall": 0.0,
            "rouge_1_f1": 0.0,
            "rouge_2_precision": 0.0,
            "rouge_2_recall": 0.0,
            "rouge_2_f1": 0.0,
            "rouge_l_precision": 0.0,
            "rouge_l_recall": 0.0,
            "rouge_l_f1": 0.0,
        }

        for candidate, reference in zip(candidates, references):
            result = self.compute_rouge(candidate, reference, lang)
            result_dict = result.to_dict()

            for key in totals:
                totals[key] += result_dict[key]

            if return_individual:
                individual_scores.append(result_dict)

        n = len(candidates)
        avg_result = {key: val / n for key, val in totals.items()}
        avg_result["num_samples"] = n

        if return_individual:
            avg_result["individual_scores"] = individual_scores

        logger.info(
            "Batch ROUGE computed",
            num_samples=n,
            avg_rouge_1_f1=f"{avg_result['rouge_1_f1']:.4f}",
            avg_rouge_l_f1=f"{avg_result['rouge_l_f1']:.4f}",
            lang=lang,
        )

        return avg_result


# Singleton instance
rouge_evaluator = ROUGEEvaluator()
