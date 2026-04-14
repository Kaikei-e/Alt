"""OpenTelemetry instruments for distributed BE dispatch observability.

Emits per-remote dispatch counters, in-flight gauges, request duration
histograms, and fallback/cooldown counters. Metric names and attribute sets
are bounded and cardinality-safe (remote URLs and model names only).
"""

from __future__ import annotations

import time
from contextlib import asynccontextmanager
from typing import AsyncIterator, Optional

from opentelemetry import metrics


_DISPATCH_COUNTER_NAME = "newscreator.distributed_be.dispatches"
_INFLIGHT_NAME = "newscreator.distributed_be.inflight"
_DURATION_NAME = "newscreator.distributed_be.request.duration"
_FALLBACK_COUNTER_NAME = "newscreator.distributed_be.fallbacks"
_COOLDOWN_COUNTER_NAME = "newscreator.distributed_be.cooldowns"


class _Metrics:
    def __init__(self, meter: metrics.Meter) -> None:
        self.dispatches = meter.create_counter(
            _DISPATCH_COUNTER_NAME,
            unit="1",
            description="Distributed BE dispatch attempts grouped by remote, model, outcome.",
        )
        self.inflight = meter.create_up_down_counter(
            _INFLIGHT_NAME,
            unit="1",
            description="In-flight distributed BE dispatches currently held by a remote.",
        )
        self.duration = meter.create_histogram(
            _DURATION_NAME,
            unit="s",
            description="Wall-clock duration of a distributed BE dispatch to a remote.",
        )
        self.fallbacks = meter.create_counter(
            _FALLBACK_COUNTER_NAME,
            unit="1",
            description="Fallback events when a remote failed and another path was taken.",
        )
        self.cooldowns = meter.create_counter(
            _COOLDOWN_COUNTER_NAME,
            unit="1",
            description="Times a remote entered the cooldown/unhealthy state.",
        )


_metrics: Optional[_Metrics] = None


def _get() -> _Metrics:
    global _metrics
    if _metrics is None:
        meter = metrics.get_meter("news_creator.distributed_be")
        _metrics = _Metrics(meter)
    return _metrics


def reset_metrics_for_tests(meter: Optional[metrics.Meter]) -> None:
    """Re-bind instruments to a test meter (or clear to defer re-init)."""
    global _metrics
    _metrics = _Metrics(meter) if meter is not None else None


class DispatchObservation:
    """Mutable outcome holder yielded from dispatch_context()."""

    __slots__ = ("_outcome",)

    def __init__(self) -> None:
        self._outcome: Optional[str] = None

    def set_outcome(self, outcome: str) -> None:
        self._outcome = outcome

    @property
    def outcome(self) -> Optional[str]:
        return self._outcome


@asynccontextmanager
async def dispatch_context(
    *, remote_url: str, model: str
) -> AsyncIterator[DispatchObservation]:
    """Track one dispatched BE request: inflight +/-, duration, outcome counter.

    Outcome defaults to ``"failure"`` if the body neither calls ``set_outcome``
    nor raises — this keeps ambiguous exits visible instead of silently counting
    as success. Exceptions propagate after the inflight gauge is decremented.
    """
    m = _get()
    inflight_attrs = {"remote_url": remote_url}
    m.inflight.add(1, inflight_attrs)

    obs = DispatchObservation()
    started = time.monotonic()
    try:
        yield obs
    except BaseException:
        if obs.outcome is None:
            obs.set_outcome("failure")
        raise
    finally:
        elapsed = time.monotonic() - started
        outcome = obs.outcome or "failure"
        m.inflight.add(-1, inflight_attrs)
        m.dispatches.add(
            1,
            {"remote_url": remote_url, "model": model, "outcome": outcome},
        )
        m.duration.record(
            elapsed,
            {"remote_url": remote_url, "outcome": outcome},
        )


def record_fallback(*, from_remote_url: str, to: str, reason: str) -> None:
    """Record that a dispatch fell back to another remote or to local."""
    _get().fallbacks.add(
        1,
        {"from_remote_url": from_remote_url, "to": to, "reason": reason},
    )


def record_cooldown(*, remote_url: str, reason: str) -> None:
    """Record that a remote was moved into the cooldown/unhealthy state."""
    _get().cooldowns.add(1, {"remote_url": remote_url, "reason": reason})
