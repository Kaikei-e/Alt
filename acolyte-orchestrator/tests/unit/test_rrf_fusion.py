"""Unit tests for RRF (Reciprocal Rank Fusion) implementation.

TDD RED phase: These tests define the expected behavior of FusionStrategy/RRFFusion
before the implementation exists.
"""

from __future__ import annotations

import pytest

from acolyte.domain.fusion import RRFFusion, ScoredHit


def _hit(article_id: str, score: float = 0.0, source: str = "lexical") -> ScoredHit:
    return ScoredHit(
        article_id=article_id,
        title=f"Article {article_id}",
        tags=None,
        score=score,
        source=source,
    )


class TestRRFFusion:
    def test_single_list_passthrough(self) -> None:
        """Single ranked list should pass through with RRF scores."""
        fusion = RRFFusion(k=60)
        hits = [_hit("a", 0.9), _hit("b", 0.7), _hit("c", 0.5)]
        result = fusion.fuse([hits])
        assert len(result) == 3
        assert result[0].article_id == "a"
        assert result[1].article_id == "b"
        assert result[2].article_id == "c"

    def test_two_lists_interleave(self) -> None:
        """Two ranked lists with different orderings should be fused by RRF score."""
        fusion = RRFFusion(k=60)
        list1 = [_hit("a", 0.9, "lexical"), _hit("b", 0.7, "lexical")]
        list2 = [_hit("b", 0.8, "broad"), _hit("c", 0.6, "broad")]

        result = fusion.fuse([list1, list2])

        # "b" appears rank 2 in list1 (1/(60+2)=0.0161) and rank 1 in list2 (1/(60+1)=0.0164)
        # RRF(b) = 0.0161 + 0.0164 = 0.0325
        # "a" appears rank 1 in list1 only: RRF(a) = 0.0164
        # "c" appears rank 2 in list2 only: RRF(c) = 0.0161
        # Order: b > a > c
        assert result[0].article_id == "b"
        assert result[1].article_id == "a"
        assert result[2].article_id == "c"

    def test_dedup_by_article_id(self) -> None:
        """Duplicate article IDs across lists should be merged, not duplicated."""
        fusion = RRFFusion(k=60)
        list1 = [_hit("a", 0.9, "lexical")]
        list2 = [_hit("a", 0.8, "broad")]

        result = fusion.fuse([list1, list2])
        assert len(result) == 1
        assert result[0].article_id == "a"

    def test_empty_lists_handled(self) -> None:
        """Empty ranked lists should produce empty result."""
        fusion = RRFFusion(k=60)
        assert fusion.fuse([]) == []
        assert fusion.fuse([[]]) == []
        assert fusion.fuse([[], []]) == []

    def test_tie_breaking_by_max_score(self) -> None:
        """When RRF scores are equal, break ties by max individual score."""
        fusion = RRFFusion(k=60)
        # Both appear at rank 1 in their respective single lists → same RRF score
        list1 = [_hit("a", 0.9, "lexical")]
        list2 = [_hit("b", 0.5, "broad")]

        result = fusion.fuse([list1, list2])
        assert len(result) == 2
        # Same RRF score (1/(60+1) each), so "a" wins by higher max_score (0.9 > 0.5)
        assert result[0].article_id == "a"
        assert result[1].article_id == "b"

    def test_k_parameter_effect(self) -> None:
        """Different k values should change relative ordering when list depths differ."""
        list1 = [_hit("a", 0.9, "lexical"), _hit("b", 0.7, "lexical"), _hit("c", 0.5, "lexical")]
        list2 = [_hit("c", 0.8, "broad")]

        # With k=1: rank matters a lot. c gets 1/(1+1)=0.5 from list2, 1/(1+3)=0.25 from list1 = 0.75
        # a gets 1/(1+1)=0.5. So c > a
        result_small_k = RRFFusion(k=1).fuse([list1, list2])
        assert result_small_k[0].article_id == "c"

        # With k=1000: all ranks compress. Appearing in 2 lists still wins.
        result_large_k = RRFFusion(k=1000).fuse([list1, list2])
        assert result_large_k[0].article_id == "c"  # 2 lists > 1 list regardless of k

    def test_fused_hit_preserves_best_metadata(self) -> None:
        """Fused hit should preserve the metadata from the highest-scored source."""
        fusion = RRFFusion(k=60)
        hit1 = ScoredHit(article_id="a", title="Title v1", tags=["tag1"], score=0.5, source="lexical")
        hit2 = ScoredHit(article_id="a", title="Title v2", tags=["tag2"], score=0.9, source="broad")

        result = fusion.fuse([[hit1], [hit2]])
        assert len(result) == 1
        # Should use metadata from higher-scored hit
        assert result[0].title == "Title v2"
        assert result[0].score > 0  # RRF score, not original score

    def test_three_lists_fusion(self) -> None:
        """Three lists should be fused correctly."""
        fusion = RRFFusion(k=60)
        list1 = [_hit("a", 0.9), _hit("b", 0.8)]
        list2 = [_hit("b", 0.7), _hit("c", 0.6)]
        list3 = [_hit("c", 0.5), _hit("a", 0.4)]

        result = fusion.fuse([list1, list2, list3])
        # a: appears in list1 rank 1 + list3 rank 2
        # b: appears in list1 rank 2 + list2 rank 1
        # c: appears in list2 rank 2 + list3 rank 1
        # All have same total RRF (1/(k+1) + 1/(k+2))
        # Tie-break by max_score: a(0.9) > b(0.8) > c(0.6)
        assert result[0].article_id == "a"
        assert result[1].article_id == "b"
        assert result[2].article_id == "c"
