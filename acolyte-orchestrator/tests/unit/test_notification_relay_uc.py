"""Unit tests for the notification outbox relay loop.

The relay is at-least-once by construction: it claims, commits the claim, then
calls out. Every property here exists because the opposite failure is silent —
a row forwarded but never marked, a wedged loop whose last emitted liveness
value still reads healthy, a permanent failure retried forever.

The relay publishes no metrics endpoint, so its two signals travel as fields on
the per-tick log line. The field names are asserted verbatim below because they
are what an operator greps for, and because two other producers (pre-processor,
recap-worker) publish the same two names as Prometheus gauges.
"""

from __future__ import annotations

import asyncio
from datetime import UTC, datetime
from typing import TYPE_CHECKING, Any
from uuid import UUID, uuid4

import pytest
from structlog.testing import capture_logs

from acolyte.domain.notification import MAX_ATTEMPTS, PendingNotification
from acolyte.usecase.relay_notifications_uc import RelayNotificationsUsecase

_AGE_FIELD = "notification_outbox_oldest_pending_age_seconds"
_TICK_FIELD = "notification_outbox_last_tick_timestamp_seconds"

if TYPE_CHECKING:
    from collections.abc import MutableMapping, Sequence

_USER = UUID("44444444-4444-4444-4444-444444444444")


def _pending(*, attempts: int = 1) -> PendingNotification:
    return PendingNotification(
        notification_id=uuid4(),
        dedupe_key=f"acolyte:{uuid4()}",
        user_id=_USER,
        kind="acolyte_report_ready",
        payload={"kind": "acolyte_report_ready", "url": "/acolyte/reports/x"},
        occurred_at=datetime(2026, 8, 8, 9, 0, tzinfo=UTC),
        attempts=attempts,
    )


class _FakeOutbox:
    def __init__(self, batches: Sequence[Sequence[PendingNotification]], *, oldest_age: float = 0.0) -> None:
        self._batches = [list(b) for b in batches]
        self._oldest_age = oldest_age
        self.events: list[tuple[str, object]] = []

    async def claim_batch(self, worker_id: str, limit: int) -> list[PendingNotification]:
        self.events.append(("claim", (worker_id, limit)))
        return self._batches.pop(0) if self._batches else []

    async def mark_forwarded(self, notification_id: UUID) -> None:
        self.events.append(("forwarded", notification_id))

    async def mark_failed(self, notification_id: UUID, attempts: int, error: str) -> None:
        self.events.append(("failed", (notification_id, attempts, error)))

    async def oldest_pending_age_seconds(self) -> float:
        self.events.append(("age", None))
        return self._oldest_age


class _FakeForwarder:
    def __init__(self, *, fail_with: Exception | None = None) -> None:
        self._fail_with = fail_with
        self.forwarded: list[PendingNotification] = []

    async def forward(self, notification: PendingNotification) -> None:
        self.forwarded.append(notification)
        if self._fail_with is not None:
            raise self._fail_with


def _relay(outbox: _FakeOutbox, forwarder: _FakeForwarder) -> RelayNotificationsUsecase:
    return RelayNotificationsUsecase(
        outbox,  # type: ignore[arg-type]
        forwarder,  # type: ignore[arg-type]
        worker_id="acolyte-1",
        batch_size=5,
    )


def _tick_events(logs: Sequence[MutableMapping[str, Any]]) -> list[MutableMapping[str, Any]]:
    return [entry for entry in logs if entry.get("event") == "notification_relay_tick"]


@pytest.mark.asyncio
async def test_forwards_each_claimed_row_then_marks_it() -> None:
    row = _pending()
    outbox = _FakeOutbox([[row]])
    forwarder = _FakeForwarder()

    forwarded = await _relay(outbox, forwarder).tick()

    assert forwarded == 1
    assert forwarder.forwarded == [row]
    kinds = [name for name, _ in outbox.events]
    assert kinds.index("claim") < kinds.index("forwarded")
    assert ("forwarded", row.notification_id) in outbox.events


@pytest.mark.asyncio
async def test_claim_uses_the_configured_worker_id_and_batch_size() -> None:
    outbox = _FakeOutbox([[]])
    await _relay(outbox, _FakeForwarder()).tick()
    assert ("claim", ("acolyte-1", 5)) in outbox.events


@pytest.mark.asyncio
async def test_a_failed_forward_is_rescheduled_with_its_attempt_count() -> None:
    row = _pending(attempts=3)
    outbox = _FakeOutbox([[row]])
    forwarder = _FakeForwarder(fail_with=RuntimeError("data-hub unavailable"))

    forwarded = await _relay(outbox, forwarder).tick()

    assert forwarded == 0
    failures = [payload for name, payload in outbox.events if name == "failed"]
    assert len(failures) == 1
    notification_id, attempts, error = failures[0]  # type: ignore[misc]
    assert notification_id == row.notification_id
    assert attempts == 3
    assert "data-hub unavailable" in error
    assert not any(name == "forwarded" for name, _ in outbox.events)


@pytest.mark.asyncio
async def test_one_bad_row_does_not_strand_the_rest_of_the_batch() -> None:
    class _FlakyForwarder:
        def __init__(self) -> None:
            self.seen: list[PendingNotification] = []

        async def forward(self, notification: PendingNotification) -> None:
            self.seen.append(notification)
            if len(self.seen) == 1:
                raise RuntimeError("boom")

    first, second = _pending(), _pending()
    outbox = _FakeOutbox([[first, second]])
    forwarder = _FlakyForwarder()
    relay = RelayNotificationsUsecase(
        outbox,  # type: ignore[arg-type]
        forwarder,  # type: ignore[arg-type]
        worker_id="acolyte-1",
        batch_size=5,
    )

    forwarded = await relay.tick()

    assert forwarded == 1
    assert ("forwarded", second.notification_id) in outbox.events
    assert any(name == "failed" for name, _ in outbox.events)


@pytest.mark.asyncio
async def test_both_signals_are_emitted_every_tick_including_the_empty_ones() -> None:
    outbox = _FakeOutbox([[], []], oldest_age=0.0)
    relay = _relay(outbox, _FakeForwarder())

    with capture_logs() as logs:
        await relay.tick()
        await relay.tick()

    # A signal you stop emitting leaves the last value standing as the freshest
    # thing an operator can see, which makes a wedged relay look healthy — so a
    # tick that moved nothing still has to say so.
    ticks = _tick_events(logs)
    assert len(ticks) == 2
    assert [entry[_AGE_FIELD] for entry in ticks] == [0.0, 0.0]
    assert all(isinstance(entry[_TICK_FIELD], float) for entry in ticks)
    assert ticks[0][_TICK_FIELD] > 0


@pytest.mark.asyncio
async def test_the_backlog_signal_reports_the_age_after_the_batch_drained() -> None:
    outbox = _FakeOutbox([[_pending()]], oldest_age=42.5)

    with capture_logs() as logs:
        await _relay(outbox, _FakeForwarder()).tick()

    assert [entry[_AGE_FIELD] for entry in _tick_events(logs)] == [42.5]
    kinds = [name for name, _ in outbox.events]
    assert kinds.index("forwarded") < kinds.index("age")


@pytest.mark.asyncio
async def test_a_failing_tick_does_not_emit_a_liveness_timestamp() -> None:
    class _BrokenOutbox(_FakeOutbox):
        async def claim_batch(self, worker_id: str, limit: int) -> list[PendingNotification]:
            msg = "db down"
            raise RuntimeError(msg)

    relay = _relay(_BrokenOutbox([]), _FakeForwarder())

    with capture_logs() as logs, pytest.raises(RuntimeError):
        await relay.tick()

    assert _tick_events(logs) == []


@pytest.mark.asyncio
async def test_the_loop_survives_a_failing_tick_and_keeps_going() -> None:
    class _FlakyOutbox(_FakeOutbox):
        def __init__(self) -> None:
            super().__init__([])
            self.calls = 0

        async def claim_batch(self, worker_id: str, limit: int) -> list[PendingNotification]:
            self.calls += 1
            if self.calls == 1:
                msg = "db down"
                raise RuntimeError(msg)
            return []

    outbox = _FlakyOutbox()
    relay = _relay(outbox, _FakeForwarder())

    task = asyncio.create_task(relay.run_forever(interval_seconds=0.0))
    for _ in range(200):
        await asyncio.sleep(0)
        if outbox.calls >= 3:
            break
    task.cancel()
    with pytest.raises(asyncio.CancelledError):
        await task

    assert outbox.calls >= 3


def test_max_attempts_is_the_domain_constant_not_a_local_number() -> None:
    # The relay hands the row's attempt count back to the outbox and lets the
    # gateway apply the death rule, so there is exactly one place to change it.
    assert MAX_ATTEMPTS == 8
