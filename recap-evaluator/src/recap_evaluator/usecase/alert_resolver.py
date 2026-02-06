"""Centralized alert level resolution."""

from recap_evaluator.domain.models import AlertLevel


class AlertResolver:
    """Resolves overall alert level from multiple dimension alerts."""

    @staticmethod
    def resolve(alert_levels: list[AlertLevel]) -> AlertLevel:
        """Determine highest severity from a list of alert levels."""
        if not alert_levels:
            return AlertLevel.OK

        if AlertLevel.CRITICAL in alert_levels:
            return AlertLevel.CRITICAL
        if AlertLevel.WARN in alert_levels:
            return AlertLevel.WARN
        return AlertLevel.OK
