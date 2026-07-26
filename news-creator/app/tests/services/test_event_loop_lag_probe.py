"""Tests for the event-loop scheduling-lag probe.

FastAPI/asyncio guidance: don't run scaling CPU work on the loop, and detect
loop lag via a heartbeat probe. This is the code-level observability signal
for regressions like an unthrottled O(N) sweep or a synchronous
re-serialization hogging the loop during a large hierarchical job.
"""

import asyncio
import contextlib
import logging

import pytest


@pytest.mark.asyncio
async def test_check_and_log_lag_warns_when_over_threshold(caplog):
    """A wakeup that overshoots the requested sleep interval by more than
    the threshold must log a WARNING with the measured lag."""
    from news_creator.services.event_loop_lag_probe import _check_and_log_lag

    caplog.set_level(logging.WARNING)

    # scheduled_at=0.0, woke_at=1.6s, interval=1.0s -> 600ms overshoot
    lag_ms = _check_and_log_lag(
        scheduled_at=0.0, woke_at=1.6, interval_seconds=1.0, warn_threshold_ms=500.0
    )

    assert lag_ms == pytest.approx(600.0, abs=1.0)

    warnings = [r for r in caplog.records if r.levelno == logging.WARNING]
    lag_warnings = [r for r in warnings if "lag" in r.message.lower()]
    assert len(lag_warnings) == 1
    assert lag_warnings[0].lag_ms == pytest.approx(600.0, abs=1.0)
    assert lag_warnings[0].warn_threshold_ms == 500.0


@pytest.mark.asyncio
async def test_check_and_log_lag_silent_when_under_threshold(caplog):
    """A wakeup within the threshold must not log anything."""
    from news_creator.services.event_loop_lag_probe import _check_and_log_lag

    caplog.set_level(logging.WARNING)

    # scheduled_at=0.0, woke_at=1.05s, interval=1.0s -> 50ms overshoot, well
    # under the 500ms threshold.
    lag_ms = _check_and_log_lag(
        scheduled_at=0.0, woke_at=1.05, interval_seconds=1.0, warn_threshold_ms=500.0
    )

    assert lag_ms == pytest.approx(50.0, abs=1.0)
    assert len(caplog.records) == 0


@pytest.mark.asyncio
async def test_check_and_log_lag_clamps_negative_lag_to_zero():
    """A wakeup that fires early (e.g. clock jitter) must not report negative lag."""
    from news_creator.services.event_loop_lag_probe import _check_and_log_lag

    lag_ms = _check_and_log_lag(
        scheduled_at=0.0, woke_at=0.9, interval_seconds=1.0, warn_threshold_ms=500.0
    )

    assert lag_ms == 0.0


@pytest.mark.asyncio
async def test_run_event_loop_lag_probe_logs_startup_enabled_line(caplog):
    """The probe must log a loud startup line stating it is enabled and its
    threshold, before the first sleep."""
    from news_creator.services.event_loop_lag_probe import run_event_loop_lag_probe

    caplog.set_level(logging.INFO)

    task = asyncio.create_task(
        run_event_loop_lag_probe(interval_seconds=1.0, warn_threshold_ms=500.0)
    )
    await asyncio.sleep(0)  # let the coroutine run up to its first await
    task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await task

    enabled_logs = [
        r for r in caplog.records if "Event loop lag probe enabled" in r.message
    ]
    assert len(enabled_logs) == 1
    assert enabled_logs[0].warn_threshold_ms == 500.0
    assert enabled_logs[0].interval_seconds == 1.0


@pytest.mark.asyncio
async def test_run_event_loop_lag_probe_warns_when_loop_blocked(monkeypatch, caplog):
    """Simulate a blocked event loop: the wakeup from asyncio.sleep(interval)
    is delayed far beyond the requested interval (as if CPU-bound work
    hogged the loop) -- the probe must log a WARNING with the measured lag.
    """
    from news_creator.services import event_loop_lag_probe as mod

    caplog.set_level(logging.WARNING)

    monotonic_values = iter([0.0, 2.0])  # scheduled_at=0.0, woke_at=2.0s

    def fake_now():
        return next(monotonic_values, 2.0)

    async def fake_sleep(_seconds):
        await asyncio.sleep(0)  # yield control without a real delay

    # Patch the module's own indirection points, not the global time/asyncio
    # modules -- asyncio's scheduler relies on the real time.monotonic() and
    # asyncio.sleep() internally, so patching those globally would corrupt
    # this test's own event loop timing.
    monkeypatch.setattr(mod, "_now", fake_now)
    monkeypatch.setattr(mod, "_sleep", fake_sleep)

    task = asyncio.create_task(
        mod.run_event_loop_lag_probe(interval_seconds=1.0, warn_threshold_ms=500.0)
    )
    await asyncio.sleep(0.02)  # let at least one full iteration complete
    task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await task

    lag_warnings = [
        r
        for r in caplog.records
        if r.levelno == logging.WARNING and "lag" in r.message.lower()
    ]
    assert len(lag_warnings) >= 1
    assert lag_warnings[0].lag_ms >= 500.0
