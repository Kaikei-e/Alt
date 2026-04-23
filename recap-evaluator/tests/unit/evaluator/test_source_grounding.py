"""Tests for SourceGroundingEvaluator — proportion of bullets with source_sentence_ids."""

import pytest

from recap_evaluator.evaluator.source_grounding import SourceGroundingEvaluator


class TestSourceGroundingEvaluator:
    def test_no_bullets_returns_zero(self):
        evaluator = SourceGroundingEvaluator()
        body_json = {"bullets": []}
        assert evaluator.compute(body_json) == 0.0

    def test_all_bullets_have_source_returns_one(self):
        evaluator = SourceGroundingEvaluator()
        body_json = {
            "bullets": [
                {"text": "bullet A", "source_sentence_ids": [101]},
                {"text": "bullet B", "source_sentence_ids": [102, 103]},
            ]
        }
        assert evaluator.compute(body_json) == pytest.approx(1.0)

    def test_no_bullets_have_source_returns_zero(self):
        evaluator = SourceGroundingEvaluator()
        body_json = {
            "bullets": [
                {"text": "bullet A", "source_sentence_ids": []},
                {"text": "bullet B"},
            ]
        }
        assert evaluator.compute(body_json) == 0.0

    def test_partial_coverage_returns_proportion(self):
        evaluator = SourceGroundingEvaluator()
        body_json = {
            "bullets": [
                {"text": "A", "source_sentence_ids": [1]},
                {"text": "B", "source_sentence_ids": []},
                {"text": "C", "source_sentence_ids": [2]},
                {"text": "D"},
            ]
        }
        assert evaluator.compute(body_json) == pytest.approx(0.5)

    def test_missing_bullets_key_returns_zero(self):
        evaluator = SourceGroundingEvaluator()
        assert evaluator.compute({}) == 0.0

    def test_batch_averages_over_outputs(self):
        evaluator = SourceGroundingEvaluator()
        outputs = [
            {"body_json": {"bullets": [
                {"text": "a", "source_sentence_ids": [1]},
                {"text": "b", "source_sentence_ids": [2]},
            ]}},
            {"body_json": {"bullets": [
                {"text": "c", "source_sentence_ids": []},
                {"text": "d", "source_sentence_ids": []},
            ]}},
        ]
        assert evaluator.compute_batch(outputs) == pytest.approx(0.5)
