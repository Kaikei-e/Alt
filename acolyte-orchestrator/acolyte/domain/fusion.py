"""Fusion strategies for hybrid retrieval — RRF and future CC.

Design (Issue 7 + resolve): Pure Python, no LLM involvement.
FusionStrategy is a Protocol for swappable fusion algorithms.
RRFFusion is the default (k=60, Cormack et al. 2009).
ConvexCombinationFusion is a placeholder for Phase B (needs relevance data).
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol


@dataclass(frozen=True)
class ScoredHit:
    """A search hit with score and source label for multi-source fusion."""

    article_id: str
    title: str
    tags: list[str] | None
    score: float
    source: str  # "primary", "broad", "narrow", etc.


class FusionStrategy(Protocol):
    """Protocol for ranked list fusion algorithms."""

    def fuse(self, ranked_lists: list[list[ScoredHit]]) -> list[ScoredHit]: ...


class RRFFusion:
    """Reciprocal Rank Fusion (Cormack, Clarke & Buettcher, 2009).

    RRF score for document d = sum over lists L of 1/(k + rank_in_L)
    where rank is 1-based. Tie-break by max individual score.

    k=60 is the standard default from the original paper.
    """

    def __init__(self, k: int = 60) -> None:
        self._k = k

    def fuse(self, ranked_lists: list[list[ScoredHit]]) -> list[ScoredHit]:
        if not ranked_lists:
            return []

        # Track per-article: rrf_score, max_score, best_hit
        scores: dict[str, float] = {}
        max_scores: dict[str, float] = {}
        best_hits: dict[str, ScoredHit] = {}

        for ranked_list in ranked_lists:
            for rank_0, hit in enumerate(ranked_list):
                aid = hit.article_id
                rrf = 1.0 / (self._k + rank_0 + 1)  # 1-based rank
                scores[aid] = scores.get(aid, 0.0) + rrf

                if aid not in max_scores or hit.score > max_scores[aid]:
                    max_scores[aid] = hit.score
                    best_hits[aid] = hit

        if not scores:
            return []

        # Sort by RRF score desc, then by max individual score desc for tie-breaking
        sorted_ids = sorted(
            scores,
            key=lambda aid: (scores[aid], max_scores.get(aid, 0.0)),
            reverse=True,
        )

        return [
            ScoredHit(
                article_id=aid,
                title=best_hits[aid].title,
                tags=best_hits[aid].tags,
                score=scores[aid],
                source=best_hits[aid].source,
            )
            for aid in sorted_ids
        ]
