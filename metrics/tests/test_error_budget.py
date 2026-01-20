"""エラーバジェット計算のテスト"""

from __future__ import annotations

import pytest
from pytest import approx

from alt_metrics.analysis import calculate_error_budget
from alt_metrics.models import ErrorBudgetResult


class TestCalculateErrorBudget:
    """calculate_error_budget関数のテスト"""

    def test_returns_error_budget_result(self) -> None:
        """ErrorBudgetResultを返す"""
        result = calculate_error_budget(
            error_rate=0.5,
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert isinstance(result, ErrorBudgetResult)

    def test_budget_total_is_100_minus_slo(self) -> None:
        """バジェット合計は100 - SLO"""
        result = calculate_error_budget(
            error_rate=0.5,
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.budget_total == approx(0.1)  # 100 - 99.9

    def test_budget_consumed_is_error_rate(self) -> None:
        """消費バジェットはエラー率"""
        result = calculate_error_budget(
            error_rate=0.05,
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.budget_consumed == 0.05

    def test_budget_remaining_calculated_correctly(self) -> None:
        """残りバジェットは正しく計算される"""
        result = calculate_error_budget(
            error_rate=0.05,
            slo_target=99.9,
            hours_analyzed=24,
        )
        # budget_remaining = budget_total - budget_consumed = 0.1 - 0.05 = 0.05
        assert result.budget_remaining == approx(0.05)

    def test_consumption_percentage_calculated_correctly(self) -> None:
        """消費率は正しく計算される"""
        result = calculate_error_budget(
            error_rate=0.05,
            slo_target=99.9,
            hours_analyzed=24,
        )
        # consumption_pct = (budget_consumed / budget_total) * 100 = (0.05 / 0.1) * 100 = 50%
        assert result.consumption_pct == 50.0

    def test_is_exceeded_false_when_under_budget(self) -> None:
        """バジェット内ならis_exceededはFalse"""
        result = calculate_error_budget(
            error_rate=0.05,
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.is_exceeded is False

    def test_is_exceeded_true_when_over_budget(self) -> None:
        """バジェット超過ならis_exceededはTrue"""
        result = calculate_error_budget(
            error_rate=0.15,  # > 0.1 budget
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.is_exceeded is True

    def test_consumption_over_100_when_exceeded(self) -> None:
        """超過時は消費率が100%を超える"""
        result = calculate_error_budget(
            error_rate=0.2,  # 0.2% error rate
            slo_target=99.9,  # 0.1% budget
            hours_analyzed=24,
        )
        assert result.consumption_pct == 200.0  # 200% consumed

    def test_zero_error_rate_full_budget_remaining(self) -> None:
        """エラー率0なら全バジェット残り"""
        result = calculate_error_budget(
            error_rate=0.0,
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.budget_remaining == approx(0.1)
        assert result.consumption_pct == 0.0

    def test_different_slo_targets(self) -> None:
        """異なるSLO目標で正しく計算"""
        # 99% SLO = 1% budget
        result = calculate_error_budget(
            error_rate=0.5,
            slo_target=99.0,
            hours_analyzed=24,
        )
        assert result.budget_total == 1.0
        assert result.consumption_pct == 50.0  # 0.5 / 1.0 * 100

    def test_status_healthy_when_under_50_percent(self) -> None:
        """消費率50%未満はhealthy"""
        result = calculate_error_budget(
            error_rate=0.04,
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.status == "healthy"

    def test_status_warning_when_50_to_80_percent(self) -> None:
        """消費率50-80%はwarning"""
        result = calculate_error_budget(
            error_rate=0.06,  # 60% of 0.1 budget
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.status == "warning"

    def test_status_critical_when_80_to_100_percent(self) -> None:
        """消費率80-100%はcritical"""
        result = calculate_error_budget(
            error_rate=0.09,  # 90% of 0.1 budget
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.status == "critical"

    def test_status_exceeded_when_over_100_percent(self) -> None:
        """消費率100%超はexceeded"""
        result = calculate_error_budget(
            error_rate=0.15,  # 150% of 0.1 budget
            slo_target=99.9,
            hours_analyzed=24,
        )
        assert result.status == "exceeded"
