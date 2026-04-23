"""Tests for RedundancyEvaluator — pairwise bullet similarity (higher = more redundant)."""

import pytest

from recap_evaluator.evaluator.redundancy import RedundancyEvaluator


class TestRedundancyEvaluator:
    def test_single_bullet_has_zero_redundancy(self):
        evaluator = RedundancyEvaluator()
        score = evaluator.compute(["The cat sat on the mat."])
        assert score == 0.0

    def test_empty_bullets_returns_zero(self):
        evaluator = RedundancyEvaluator()
        assert evaluator.compute([]) == 0.0

    def test_identical_bullets_have_high_redundancy(self):
        evaluator = RedundancyEvaluator()
        bullets = [
            "The quick brown fox jumps over the lazy dog.",
            "The quick brown fox jumps over the lazy dog.",
        ]
        score = evaluator.compute(bullets)
        assert score == pytest.approx(1.0, abs=0.05)

    def test_different_bullets_have_low_redundancy(self):
        evaluator = RedundancyEvaluator()
        bullets = [
            "The cat sat on the mat.",
            "A dog ran across the park.",
            "Birds fly above the trees.",
        ]
        score = evaluator.compute(bullets)
        assert score < 0.3

    def test_two_bullets_produces_single_pair_score(self):
        evaluator = RedundancyEvaluator()
        bullets = [
            "We saw clouds above mountains.",
            "Clouds drifted above tall mountains.",
        ]
        score = evaluator.compute(bullets)
        assert 0.0 < score < 1.0
