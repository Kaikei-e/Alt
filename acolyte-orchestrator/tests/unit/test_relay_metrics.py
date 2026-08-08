"""Unit tests for the relay's Prometheus exposition.

The metric names are a cross-service contract — the other two producers
(pre-processor, recap-worker) expose the same two series, and one dashboard and
one alert rule read all three.
"""

from __future__ import annotations

from acolyte.infra.metrics import RelayMetrics


def test_gauges_are_present_from_boot_with_zero_values() -> None:
    text = RelayMetrics().render()
    assert "# TYPE notification_outbox_oldest_pending_age_seconds gauge" in text
    assert "# TYPE notification_outbox_last_tick_timestamp_seconds gauge" in text
    assert "notification_outbox_oldest_pending_age_seconds 0.0" in text
    assert "notification_outbox_last_tick_timestamp_seconds 0.0" in text


def test_every_series_carries_a_help_line() -> None:
    lines = RelayMetrics().render().splitlines()
    help_lines = [line for line in lines if line.startswith("# HELP")]
    assert len(help_lines) == 2


def test_set_values_are_rendered() -> None:
    metrics = RelayMetrics()
    metrics.set_oldest_pending_age_seconds(12.25)
    metrics.set_last_tick_timestamp_seconds(1786000000.5)

    text = metrics.render()
    assert "notification_outbox_oldest_pending_age_seconds 12.25" in text
    assert "notification_outbox_last_tick_timestamp_seconds 1786000000.5" in text


def test_exposition_ends_with_a_newline() -> None:
    # Prometheus rejects a body whose last line is unterminated.
    assert RelayMetrics().render().endswith("\n")
