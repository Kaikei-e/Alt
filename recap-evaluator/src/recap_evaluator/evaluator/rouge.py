"""ROUGE-based evaluation for text summarization.

Re-exported from evaluators/rouge_eval.py for Clean Architecture layout.
These are CPU-bound, stateless evaluators — no DI needed.
"""

from recap_evaluator.evaluator.rouge_eval import (
    JapaneseTokenizer,
    ROUGEEvaluator,
    ROUGEResult,
)

__all__ = ["JapaneseTokenizer", "ROUGEEvaluator", "ROUGEResult"]
