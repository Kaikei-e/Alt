"""Deep health runner — docs/runbooks/health-deep-contract.md."""

from __future__ import annotations

import asyncio
import time
from collections.abc import Awaitable, Callable, Sequence
from dataclasses import dataclass
from typing import Literal

from prometheus_client import REGISTRY, Gauge

Status = Literal["pass", "warn", "fail"]
Reason = Literal["timeout", "unavailable", "not_ready"]

STATUS_PASS: Status = "pass"  # noqa: S105 — status token, not a secret
STATUS_WARN: Status = "warn"
STATUS_FAIL: Status = "fail"

DEFAULT_CACHE_TTL_S = 3.0
DEFAULT_PER_CHECK_S = 0.25
DEFAULT_BUDGET_S = 0.8

_STATUS_VALUE: dict[Status, float] = {
    STATUS_FAIL: 0.0,
    STATUS_WARN: 1.0,
    STATUS_PASS: 2.0,
}

Probe = Callable[[], Awaitable[None]]


def _gauge(name: str, documentation: str, labelnames: tuple[str, ...]) -> Gauge:
    existing = REGISTRY._names_to_collectors.get(name)
    if isinstance(existing, Gauge):
        return existing
    return Gauge(name, documentation, labelnames)


HEALTH_DEEP_STATUS = _gauge(
    "health_deep_status",
    "Deep health overall status: 0=fail, 1=warn, 2=pass.",
    ("service",),
)
HEALTH_DEEP_LATENCY = _gauge(
    "health_deep_latency_seconds",
    "Wall time of the last deep-health run, including cache hits.",
    ("service",),
)


def observe(service: str, status: Status, latency_s: float) -> None:
    """Publish last-run gauges on the default registry (app /metrics scrape)."""
    HEALTH_DEEP_STATUS.labels(service=service).set(_STATUS_VALUE[status])
    HEALTH_DEEP_LATENCY.labels(service=service).set(latency_s)


@dataclass(frozen=True, slots=True)
class Check:
    name: str
    critical: bool
    probe: Probe


@dataclass(frozen=True, slots=True)
class CheckResult:
    name: str
    status: Status
    critical: bool
    latency_ms: int
    reason: Reason | None = None

    def as_dict(self) -> dict[str, object]:
        payload: dict[str, object] = {
            "name": self.name,
            "status": self.status,
            "critical": self.critical,
            "latency_ms": self.latency_ms,
        }
        if self.reason is not None:
            payload["reason"] = self.reason
        return payload


@dataclass(frozen=True, slots=True)
class Report:
    status: Status
    service: str
    checks: tuple[CheckResult, ...]
    latency_ms: int
    cached: bool

    def as_dict(self) -> dict[str, object]:
        return {
            "status": self.status,
            "service": self.service,
            "checks": [c.as_dict() for c in self.checks],
            "latency_ms": self.latency_ms,
            "cached": self.cached,
        }

    @property
    def http_status(self) -> int:
        if self.status == STATUS_FAIL:
            return 503
        return 200


class DeepHealthRunner:
    """Parallel checks, per-check timeout, global budget, cache + singleflight."""

    def __init__(
        self,
        service: str,
        checks: Sequence[Check],
        *,
        cache_ttl_s: float = DEFAULT_CACHE_TTL_S,
        per_check_s: float = DEFAULT_PER_CHECK_S,
        budget_s: float = DEFAULT_BUDGET_S,
    ) -> None:
        if not service:
            raise ValueError("deep health service name is required")
        self._service = service
        self._checks = tuple(checks)
        self._cache_ttl_s = min(max(cache_ttl_s, 2.0), 5.0)
        self._per_check_s = per_check_s
        self._budget_s = min(budget_s, 0.8)
        self._lock = asyncio.Lock()
        self._cached: tuple[float, Report] | None = None
        self._inflight: asyncio.Future[Report] | None = None
        self._task: asyncio.Task[None] | None = None

    @property
    def budget_s(self) -> float:
        return self._budget_s

    async def run(self) -> Report:
        loop = asyncio.get_running_loop()
        started = time.perf_counter()
        now = loop.time()
        async with self._lock:
            if self._cached is not None:
                until, report = self._cached
                if now < until:
                    cached = Report(
                        status=report.status,
                        service=report.service,
                        checks=report.checks,
                        latency_ms=report.latency_ms,
                        cached=True,
                    )
                    observe(self._service, cached.status, time.perf_counter() - started)
                    return cached
            if self._inflight is not None:
                fut = self._inflight
            else:
                fut = loop.create_future()
                self._inflight = fut
                self._task = loop.create_task(self._compute_and_settle(fut))

        report = await asyncio.shield(fut)
        observe(self._service, report.status, time.perf_counter() - started)
        return report

    async def _compute_and_settle(self, fut: asyncio.Future[Report]) -> None:
        try:
            report = await self._compute()
            async with self._lock:
                self._cached = (
                    asyncio.get_running_loop().time() + self._cache_ttl_s,
                    report,
                )
                self._inflight = None
            if not fut.done():
                fut.set_result(report)
        except asyncio.CancelledError:
            async with self._lock:
                self._inflight = None
            if not fut.done():
                fut.set_exception(asyncio.CancelledError())
            raise
        except RuntimeError as exc:
            async with self._lock:
                self._inflight = None
            if not fut.done():
                fut.set_exception(exc)

    async def _compute(self) -> Report:
        start = time.perf_counter()
        try:
            async with asyncio.timeout(self._budget_s):
                results = await asyncio.gather(
                    *(self._run_one(check) for check in self._checks),
                )
        except TimeoutError:
            results = tuple(
                CheckResult(
                    name=c.name,
                    status=STATUS_FAIL if c.critical else STATUS_WARN,
                    critical=c.critical,
                    latency_ms=int(self._budget_s * 1000),
                    reason="timeout",
                )
                for c in self._checks
            )
        overall: Status = STATUS_PASS
        for row in results:
            if row.status == STATUS_FAIL and row.critical:
                overall = STATUS_FAIL
                break
            if row.status != STATUS_PASS and overall != STATUS_FAIL:
                overall = STATUS_WARN
        return Report(
            status=overall,
            service=self._service,
            checks=tuple(results),
            latency_ms=int((time.perf_counter() - start) * 1000),
            cached=False,
        )

    async def _run_one(self, check: Check) -> CheckResult:
        start = time.perf_counter()
        reason: Reason | None = None
        status: Status = STATUS_PASS
        try:
            async with asyncio.timeout(self._per_check_s):
                await check.probe()
        except TimeoutError:
            status = STATUS_FAIL if check.critical else STATUS_WARN
            reason = "timeout"
        except asyncio.CancelledError:
            raise
        except (RuntimeError, OSError, ConnectionError, ValueError, LookupError):
            status = STATUS_FAIL if check.critical else STATUS_WARN
            reason = "unavailable"
        return CheckResult(
            name=check.name,
            status=status,
            critical=check.critical,
            latency_ms=int((time.perf_counter() - start) * 1000),
            reason=reason,
        )
