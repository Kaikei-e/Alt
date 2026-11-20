"""Tests for AlertResolver."""

from recap_evaluator.domain.models import AlertLevel
from recap_evaluator.usecase.alert_resolver import AlertResolver


class TestAlertResolver:
    def test_empty_list_returns_ok(self) -> None:
        assert AlertResolver.resolve([]) == AlertLevel.OK

    def test_all_ok(self) -> None:
        levels = [AlertLevel.OK, AlertLevel.OK]
        assert AlertResolver.resolve(levels) == AlertLevel.OK

    def test_warn_propagates(self) -> None:
        levels = [AlertLevel.OK, AlertLevel.WARN]
        assert AlertResolver.resolve(levels) == AlertLevel.WARN

    def test_critical_propagates(self) -> None:
        levels = [AlertLevel.OK, AlertLevel.WARN, AlertLevel.CRITICAL]
        assert AlertResolver.resolve(levels) == AlertLevel.CRITICAL

    def test_single_critical(self) -> None:
        levels = [AlertLevel.CRITICAL]
        assert AlertResolver.resolve(levels) == AlertLevel.CRITICAL
