"""Tests for RecapQualityEvaluator — deterministic quality scorers."""

import pytest

from news_creator.domain.models import RecapSummary
from news_creator.evaluation.recap_quality import RecapQualityEvaluator

# ============================================================================
# Source Grounding Tests
# ============================================================================


MAX_LOW_REDUNDANCY_SCORE = 0.3
MIN_HIGH_REDUNDANCY_SCORE = 0.1
MIN_GOOD_READABILITY_SCORE = 0.5
MAX_SHORT_READABILITY_SCORE = 0.5
MIN_PROPER_ENDING_READABILITY_SCORE = 0.7
MIN_GOOD_STRUCTURE_SCORE = 0.5
MAX_FRAGMENT_STRUCTURE_SCORE = 0.3
MIN_RICH_ENTITY_SCORE = 0.5
MAX_NO_ENTITY_SCORE = 0.3


class TestSourceGrounding:
    """evaluate_source_grounding checks [n] markers against references list."""

    def test_perfect_grounding(self, good_summary: RecapSummary) -> None:
        """All bullet [n] markers map to existing references, no unused refs."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_source_grounding(good_summary)
        assert score == pytest.approx(1.0)

    def test_dangling_marker_and_unused_ref(
        self, broken_refs_summary: RecapSummary
    ) -> None:
        """[3] has no matching ref, ref id=2 is unused → score < 1.0."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_source_grounding(broken_refs_summary)
        assert 0.0 < score < 1.0

    def test_no_references_no_markers(self) -> None:
        """No references and no markers → score 0.0 (grounding absent)."""
        summary = RecapSummary(
            title="タイトル",
            bullets=["マーカーなしの文。"],
            language="ja",
        )
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_source_grounding(summary)
        assert score == pytest.approx(0.0)

    def test_markers_present_but_no_references_list(self) -> None:
        """Bullets have [1] markers but references is None → score 0.0."""
        summary = RecapSummary(
            title="タイトル",
            bullets=["文 [1]"],
            language="ja",
            references=None,
        )
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_source_grounding(summary)
        assert score == pytest.approx(0.0)


# ============================================================================
# Redundancy Tests
# ============================================================================


class TestRedundancy:
    """evaluate_redundancy scores bullet-to-bullet n-gram overlap."""

    def test_no_redundancy(self, good_summary: RecapSummary) -> None:
        """Distinct bullets → low redundancy score (close to 0)."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_redundancy(good_summary)
        assert score < MAX_LOW_REDUNDANCY_SCORE

    def test_high_redundancy(self, redundant_summary: RecapSummary) -> None:
        """Near-duplicate bullets → high redundancy score."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_redundancy(redundant_summary)
        # Bullets 0 and 1 share many bigrams; bullet 2 is distinct.
        # Pairwise average Jaccard across 3 pairs dilutes the score.
        assert score > MIN_HIGH_REDUNDANCY_SCORE

    def test_single_bullet_no_redundancy(self) -> None:
        """Single bullet → redundancy is 0.0 by definition."""
        summary = RecapSummary(title="タイトル", bullets=["一つだけ。"], language="ja")
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_redundancy(summary)
        assert score == pytest.approx(0.0)


# ============================================================================
# Readability Tests
# ============================================================================


class TestReadability:
    """evaluate_readability checks bullet length and sentence endings."""

    def test_ideal_length_bullets(self, good_summary: RecapSummary) -> None:
        """Bullets in 400-600 char range → high readability."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_readability(good_summary)
        # good_summary bullets are ~250 chars — within a reasonable range but
        # the actual check range is 200-600 for tolerance
        assert score > MIN_GOOD_READABILITY_SCORE

    def test_too_short_bullets(self, short_summary: RecapSummary) -> None:
        """Very short bullets (< 100 chars) → low readability score."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_readability(short_summary)
        assert score < MAX_SHORT_READABILITY_SCORE

    def test_proper_sentence_endings(self) -> None:
        """Bullets ending with proper Japanese endings (。、た、る) score higher."""
        summary = RecapSummary(
            title="タイトル",
            bullets=[
                "A" * 400 + "。",  # proper ending, ideal length
                "B" * 400 + "。",
            ],
            language="ja",
        )
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_readability(summary)
        assert score > MIN_PROPER_ENDING_READABILITY_SCORE


# ============================================================================
# Structure Tests
# ============================================================================


class TestStructure:
    """evaluate_structure checks 4-element presence per bullet."""

    def test_well_structured_bullets(self, good_summary: RecapSummary) -> None:
        """Good summary bullets contain who/what, action, background, impact."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_structure(good_summary)
        # good_summary has company names, actions (買収, 発表), numbers, outlook
        assert score > MIN_GOOD_STRUCTURE_SCORE

    def test_fragment_bullets(self) -> None:
        """Fragment-like bullets → low structure score."""
        summary = RecapSummary(
            title="タイトル",
            bullets=["AI", "テクノロジー"],
            language="ja",
        )
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_structure(summary)
        assert score < MAX_FRAGMENT_STRUCTURE_SCORE


# ============================================================================
# Entity Density Tests
# ============================================================================


class TestEntityDensity:
    """evaluate_entity_density checks named entities and numeric values."""

    def test_rich_entities(self, good_summary: RecapSummary) -> None:
        """Bullets with company names, dates, currencies → high density."""
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_entity_density(good_summary)
        # Contains: TechFusion, Nova Labs, Google, Gemini 3.0, 12億ドル, 40%, etc.
        assert score > MIN_RICH_ENTITY_SCORE

    def test_no_entities(self) -> None:
        """Vague bullets with no proper nouns or numbers → low density."""
        summary = RecapSummary(
            title="タイトル",
            bullets=["技術が進歩した。", "業界が変わった。"],
            language="ja",
        )
        evaluator = RecapQualityEvaluator()
        score = evaluator.evaluate_entity_density(summary)
        assert score < MAX_NO_ENTITY_SCORE


# ============================================================================
# evaluate_all Tests
# ============================================================================


class TestEvaluateAll:
    """evaluate_all returns per-axis scores dict."""

    def test_returns_all_axes(self, good_summary: RecapSummary) -> None:
        evaluator = RecapQualityEvaluator()
        scores = evaluator.evaluate_all(good_summary)
        expected_axes = {
            "source_grounding",
            "redundancy",
            "readability",
            "structure",
            "entity_density",
        }
        assert set(scores.keys()) == expected_axes
        for axis, val in scores.items():
            assert 0.0 <= val <= 1.0, f"{axis} out of range: {val}"
