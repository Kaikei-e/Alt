"""Redundancy — mean pairwise ROUGE-L F1 across summary bullets (higher = more redundant)."""

from itertools import combinations

from recap_evaluator.evaluator.rouge_eval import ROUGEEvaluator


class RedundancyEvaluator:
    def __init__(self, rouge: ROUGEEvaluator | None = None) -> None:
        self._rouge = rouge or ROUGEEvaluator()

    def compute(self, bullets: list[str]) -> float:
        non_empty = [b for b in bullets if b and b.strip()]
        if len(non_empty) < 2:
            return 0.0

        scores: list[float] = []
        for a, b in combinations(non_empty, 2):
            result = self._rouge.compute_rouge(a, b)
            scores.append(result.rouge_l_f1)

        return sum(scores) / len(scores) if scores else 0.0
