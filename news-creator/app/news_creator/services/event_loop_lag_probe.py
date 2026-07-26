"""Event-loop scheduling-lag probe.

FastAPI/asyncio guidance: don't run scaling CPU work on the event loop, and
detect loop lag via a heartbeat probe (slow_callback_duration-style signal).
This is a small always-on heartbeat: it sleeps a fixed interval and measures
how much the wakeup overshot that interval. A synchronous CPU-bound
regression on the loop (e.g. an unthrottled O(N) sweep, a large payload
re-serialization) shows up here as scheduling lag, not just as slow
requests.
"""

import asyncio
import logging
import time

logger = logging.getLogger(__name__)

DEFAULT_PROBE_INTERVAL_SECONDS = 1.0


def _now() -> float:
    """Indirection point over time.monotonic() so tests can stub the clock
    without touching the real time module -- asyncio's own scheduler relies
    on time.monotonic() internally, so patching it globally would also
    corrupt the test's own event loop timing."""
    return time.monotonic()


async def _sleep(seconds: float) -> None:
    """Indirection point over asyncio.sleep() so tests can stub the delay
    without patching the global asyncio.sleep (which other concurrently
    scheduled coroutines, including the test runner's own, also rely on)."""
    await asyncio.sleep(seconds)


def _check_and_log_lag(
    *,
    scheduled_at: float,
    woke_at: float,
    interval_seconds: float,
    warn_threshold_ms: float,
) -> float:
    """Compute the observed scheduling lag and log a WARNING if it exceeds
    warn_threshold_ms.

    Args:
        scheduled_at: monotonic time when the sleep was requested
        woke_at: monotonic time when the sleep actually returned
        interval_seconds: the requested sleep duration
        warn_threshold_ms: overshoot threshold in milliseconds

    Returns:
        The measured lag in milliseconds (never negative).
    """
    lag_ms = max(0.0, (woke_at - scheduled_at - interval_seconds) * 1000.0)
    if lag_ms > warn_threshold_ms:
        logger.warning(
            "Event loop scheduling lag detected",
            extra={
                "lag_ms": round(lag_ms, 1),
                "warn_threshold_ms": warn_threshold_ms,
                "interval_seconds": interval_seconds,
            },
        )
    return lag_ms


async def run_event_loop_lag_probe(
    *,
    interval_seconds: float = DEFAULT_PROBE_INTERVAL_SECONDS,
    warn_threshold_ms: float,
) -> None:
    """Always-on heartbeat: sleeps `interval_seconds`, measures how much the
    wakeup overshot that delay, and logs a WARNING when the overshoot
    exceeds `warn_threshold_ms`. Runs until cancelled.

    Intended to be started once as a background task at app startup and
    cancelled at shutdown.
    """
    logger.info(
        "Event loop lag probe enabled",
        extra={
            "interval_seconds": interval_seconds,
            "warn_threshold_ms": warn_threshold_ms,
        },
    )
    try:
        while True:
            scheduled_at = _now()
            await _sleep(interval_seconds)
            _check_and_log_lag(
                scheduled_at=scheduled_at,
                woke_at=_now(),
                interval_seconds=interval_seconds,
                warn_threshold_ms=warn_threshold_ms,
            )
    except asyncio.CancelledError:
        logger.info("Event loop lag probe stopped")
        raise
