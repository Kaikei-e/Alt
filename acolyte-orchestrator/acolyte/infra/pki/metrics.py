"""Lifecycle metrics for in-process enrollment."""

# ruff: noqa: TRY003

from __future__ import annotations

from datetime import UTC, datetime, timedelta
from typing import Protocol

from prometheus_client import REGISTRY as DEFAULT_REGISTRY
from prometheus_client import CollectorRegistry, Counter, Gauge

from acolyte.infra.pki.state import CertState


class Observer(Protocol):
    """Receives lifecycle events. Tests record; production exports metrics."""

    def on_classified(self, state: CertState, remaining: timedelta) -> None: ...
    def on_reissued(self, reason: str) -> None: ...
    def on_renewed(self, success: bool) -> None: ...
    def on_retry(self, attempt: int, err: BaseException) -> None: ...


class NopObserver:
    def on_classified(self, state: CertState, remaining: timedelta) -> None:
        return None

    def on_reissued(self, reason: str) -> None:
        return None

    def on_renewed(self, success: bool) -> None:
        return None

    def on_retry(self, attempt: int, err: BaseException) -> None:
        return None


class PromObserver:
    """Publishes pki_enrollment_* matching the Go cohort metric family."""

    def __init__(self, subject: str, registry: CollectorRegistry) -> None:
        if registry is DEFAULT_REGISTRY:
            raise TypeError("pki: PromObserver must use a private CollectorRegistry")
        reg = registry
        self._subject = subject
        self._remaining = Gauge(
            "cert_remaining_seconds",
            "Seconds until the current leaf expires. Negative if expired.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=reg,
        ).labels(subject=subject)
        self._not_after = Gauge(
            "cert_not_after_seconds",
            "Unix timestamp of the current leaf certificate's not_after.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=reg,
        ).labels(subject=subject)
        self._last_rotation = Gauge(
            "last_rotation_timestamp_seconds",
            "Unix timestamp of the last successful cert rotation.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=reg,
        ).labels(subject=subject)
        self._healthy = Gauge(
            "healthy",
            "1 if the cert on disk is currently valid (not expired).",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=reg,
        ).labels(subject=subject)
        self._renewal_total = Counter(
            "renewal_total",
            "Count of completed rotation attempts grouped by outcome.",
            labelnames=("result", "subject"),
            namespace="pki_enrollment",
            registry=reg,
        )
        self._reissue_total = Counter(
            "reissue_total",
            "Count of reissuances by reason (missing / expired / near_expiry / corrupt).",
            labelnames=("reason", "subject"),
            namespace="pki_enrollment",
            registry=reg,
        )
        self._state = CertState.MISSING

    def on_classified(self, state: CertState, remaining: timedelta) -> None:
        self._state = state
        self._remaining.set(remaining.total_seconds())
        self._not_after.set(datetime.now(tz=UTC).timestamp() + remaining.total_seconds())
        if state in {CertState.EXPIRED, CertState.CORRUPT, CertState.MISSING}:
            self._healthy.set(0)
            return
        self._healthy.set(1)

    def on_reissued(self, reason: str) -> None:
        self._reissue_total.labels(reason=reason, subject=self._subject).inc()

    def on_renewed(self, success: bool) -> None:
        result = "success" if success else "failure"
        if success:
            self._last_rotation.set(datetime.now(tz=UTC).timestamp())
        self._renewal_total.labels(result=result, subject=self._subject).inc()

    def on_retry(self, attempt: int, err: BaseException) -> None:
        return None
