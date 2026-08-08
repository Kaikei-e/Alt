"""Prometheus exposition for the notification outbox relay.

Hand-rolled rather than pulled from a metrics client: this service ships no
metrics dependency, and two gauges do not justify one. The names are the
contract — the other producers expose the same two series, and a single alert
rule reads all of them.
"""

from __future__ import annotations

OLDEST_PENDING_AGE = "notification_outbox_oldest_pending_age_seconds"
LAST_TICK_TIMESTAMP = "notification_outbox_last_tick_timestamp_seconds"


class RelayMetrics:
    """Holds the two gauge values and renders them in the text format."""

    def __init__(self) -> None:
        self._oldest_pending_age = 0.0
        self._last_tick_timestamp = 0.0

    def set_oldest_pending_age_seconds(self, value: float) -> None:
        self._oldest_pending_age = value

    def set_last_tick_timestamp_seconds(self, value: float) -> None:
        self._last_tick_timestamp = value

    def render(self) -> str:
        return (
            f"# HELP {OLDEST_PENDING_AGE} Age of the oldest un-forwarded notification, "
            "measured from when the thing happened.\n"
            f"# TYPE {OLDEST_PENDING_AGE} gauge\n"
            f"{OLDEST_PENDING_AGE} {self._oldest_pending_age!r}\n"
            f"# HELP {LAST_TICK_TIMESTAMP} Unix time of the last completed relay tick.\n"
            f"# TYPE {LAST_TICK_TIMESTAMP} gauge\n"
            f"{LAST_TICK_TIMESTAMP} {self._last_tick_timestamp!r}\n"
        )
