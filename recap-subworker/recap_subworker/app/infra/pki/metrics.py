"""Lifecycle metrics for in-process enrollment on a private registry."""

from __future__ import annotations

import time

from prometheus_client import REGISTRY as DEFAULT_REGISTRY
from prometheus_client import CollectorRegistry, Counter, Gauge

from recap_subworker.app.infra.pki.state import State


class PromObserver:
    """Publishes pki_enrollment_* matching the Go cohort metric family."""

    def __init__(self, subject: str, registry: CollectorRegistry) -> None:
        if registry is DEFAULT_REGISTRY:
            raise TypeError("pki: PromObserver must use a private CollectorRegistry")
        self._subject = subject
        self._remaining = Gauge(
            "cert_remaining_seconds",
            "Seconds until the current leaf expires. Negative if expired.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=registry,
        ).labels(subject=subject)
        self._not_after = Gauge(
            "cert_not_after_seconds",
            "Unix timestamp of the current leaf certificate's not_after.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=registry,
        ).labels(subject=subject)
        self._last_rotation = Gauge(
            "last_rotation_timestamp_seconds",
            "Unix timestamp of the last successful cert rotation.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=registry,
        ).labels(subject=subject)
        self._healthy = Gauge(
            "healthy",
            "1 if the cert on disk is currently valid (not expired).",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=registry,
        ).labels(subject=subject)
        self._renewal_total = Counter(
            "renewal_total",
            "Count of completed rotation attempts grouped by outcome.",
            labelnames=("result", "subject"),
            namespace="pki_enrollment",
            registry=registry,
        )
        self._reissue_total = Counter(
            "reissue_total",
            "Count of reissuances by reason (missing / expired / near_expiry / corrupt).",
            labelnames=("reason", "subject"),
            namespace="pki_enrollment",
            registry=registry,
        )
        self._loop_error_total = Counter(
            "loop_error_total",
            "Unexpected renewal-loop exceptions that were logged and retried.",
            labelnames=("subject",),
            namespace="pki_enrollment",
            registry=registry,
        )
        self._state = State.MISSING

    def on_classified(self, state: State, remaining: float) -> None:
        self._state = state
        self._remaining.set(remaining)
        self._not_after.set(time.time() + remaining)
        if state in {State.EXPIRED, State.CORRUPT, State.MISSING}:
            self._healthy.set(0)
            return
        self._healthy.set(1)

    def on_reissued(self, reason: str) -> None:
        self._reissue_total.labels(reason=reason, subject=self._subject).inc()

    def on_renewed(self, success: bool) -> None:
        result = "success" if success else "failure"
        if success:
            self._last_rotation.set(time.time())
        self._renewal_total.labels(result=result, subject=self._subject).inc()

    def on_retry(self, attempt: int, err: BaseException) -> None:
        del attempt, err
        self._loop_error_total.labels(subject=self._subject).inc()
