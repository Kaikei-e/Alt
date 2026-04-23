"""Tests for FallbackRateEvaluator — measures rate of degraded / low-evidence outputs."""

import pytest

from recap_evaluator.evaluator.fallback_rate import FallbackRateEvaluator


class TestFallbackRateEvaluator:
    def test_zero_outputs_returns_zero(self):
        evaluator = FallbackRateEvaluator()
        assert evaluator.compute([]) == 0.0

    def test_all_healthy_outputs_returns_zero(self):
        evaluator = FallbackRateEvaluator()
        outputs = [
            {"body_json": {"metadata": {"is_degraded": False, "model": "gemma3-4b"}}},
            {"body_json": {"metadata": {"is_degraded": False, "model": "gemma3-4b"}}},
        ]
        assert evaluator.compute(outputs) == 0.0

    def test_all_degraded_returns_one(self):
        evaluator = FallbackRateEvaluator()
        outputs = [
            {"body_json": {"metadata": {"is_degraded": True, "model": "gemma3-4b"}}},
            {"body_json": {"metadata": {"is_degraded": True}}},
        ]
        assert evaluator.compute(outputs) == pytest.approx(1.0)

    def test_low_evidence_extractive_counted_as_fallback(self):
        evaluator = FallbackRateEvaluator()
        outputs = [
            {"body_json": {"metadata": {"model": "low-evidence-extractive"}}},
            {"body_json": {"metadata": {"is_degraded": False, "model": "gemma3-4b"}}},
        ]
        assert evaluator.compute(outputs) == pytest.approx(0.5)

    def test_missing_metadata_counts_as_healthy(self):
        evaluator = FallbackRateEvaluator()
        outputs = [
            {"body_json": {}},
            {"body_json": {"metadata": {"is_degraded": True}}},
        ]
        assert evaluator.compute(outputs) == pytest.approx(0.5)
