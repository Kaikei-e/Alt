"""Tests for JsonRepairRateEvaluator — rate of outputs whose LLM JSON needed repair."""

import pytest

from recap_evaluator.evaluator.json_repair_rate import JsonRepairRateEvaluator


class TestJsonRepairRateEvaluator:
    def test_zero_outputs_returns_zero(self):
        evaluator = JsonRepairRateEvaluator()
        assert evaluator.compute([]) == 0.0

    def test_no_repair_metadata_returns_zero(self):
        evaluator = JsonRepairRateEvaluator()
        outputs = [
            {"body_json": {"metadata": {"json_repair_attempted": False}}},
            {"body_json": {"metadata": {}}},
        ]
        assert evaluator.compute(outputs) == 0.0

    def test_some_repairs_returns_proportion(self):
        evaluator = JsonRepairRateEvaluator()
        outputs = [
            {"body_json": {"metadata": {"json_repair_attempted": True}}},
            {"body_json": {"metadata": {"json_repair_attempted": False}}},
            {"body_json": {"metadata": {"json_repair_attempted": True}}},
            {"body_json": {"metadata": {"json_repair_attempted": False}}},
        ]
        assert evaluator.compute(outputs) == pytest.approx(0.5)

    def test_all_repairs_returns_one(self):
        evaluator = JsonRepairRateEvaluator()
        outputs = [
            {"body_json": {"metadata": {"json_repair_attempted": True}}},
            {"body_json": {"metadata": {"json_repair_attempted": True}}},
        ]
        assert evaluator.compute(outputs) == pytest.approx(1.0)
