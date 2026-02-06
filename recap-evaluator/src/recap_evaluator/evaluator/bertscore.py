"""BERTScore-based semantic evaluation for summarization quality.

Re-exported from evaluators/bertscore_eval.py for Clean Architecture layout.
These are CPU-bound, stateless evaluators — no DI needed.
"""

from recap_evaluator.evaluator.bertscore_eval import (
    BERTScoreEvaluator,
    BERTScoreResult,
)

__all__ = ["BERTScoreEvaluator", "BERTScoreResult"]
