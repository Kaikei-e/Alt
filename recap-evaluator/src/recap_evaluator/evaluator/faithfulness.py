"""NLI-based faithfulness evaluation for summarization.

Re-exported from evaluators/faithfulness_eval.py for Clean Architecture layout.
These are CPU-bound, stateless evaluators — no DI needed.
"""

from recap_evaluator.evaluator.faithfulness_eval import (
    FaithfulnessEvaluator,
    FaithfulnessResult,
    SentenceResult,
)

__all__ = ["FaithfulnessEvaluator", "FaithfulnessResult", "SentenceResult"]
