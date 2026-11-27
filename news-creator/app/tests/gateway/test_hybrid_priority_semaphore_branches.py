"""Branch characterization tests for HybridPrioritySemaphore."""

from __future__ import annotations

import asyncio
import contextlib
import heapq
import logging

import pytest

from news_creator.domain.errors import QueueFullError
from news_creator.gateway.hybrid_priority_semaphore import HybridPrioritySemaphore

_SINGLE_SLOT = 1
_TWO_SLOTS = 2
_UNKNOWN_SLOT_ID = 99999
_INITIAL_MONOTONIC_TIME = 100.0
_LONG_WAIT_MONOTONIC_TIME = 115.0


@pytest.mark.asyncio
async def test_release_unknown_slot_id_is_rejected_without_corrupting_pools(
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Releasing an unknown slot_id is strictly rejected as a no-op with ERROR log."""
    caplog.set_level(logging.ERROR)
    sem = HybridPrioritySemaphore(
        total_slots=_TWO_SLOTS,
        rt_reserved_slots=_SINGLE_SLOT,
        guaranteed_be_ratio=0,
    )
    _, rt_sid = await sem.acquire(high_priority=True)
    assert sem._rt_available == 0

    # Release the real slot first, then call release with an unknown slot_id.
    sem.release(was_high_priority=True, slot_id=rt_sid)
    assert sem._rt_available == _SINGLE_SLOT

    sem.release(was_high_priority=False, slot_id=_UNKNOWN_SLOT_ID)
    assert sem._be_available == _SINGLE_SLOT
    assert sem._rt_available == _SINGLE_SLOT
    assert any("unknown or already-released" in rec.message for rec in caplog.records)
    assert not any("SLOT INVARIANT VIOLATION" in rec.message for rec in caplog.records)


@pytest.mark.asyncio
async def test_release_without_slot_id_raises_type_error() -> None:
    """Legacy release(slot_id=None) fallback is removed; slot_id is required."""
    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
        guaranteed_be_ratio=0,
    )
    # BE falls back to the RT pool because be_slots == 0.
    _, be_sid = await sem.acquire(high_priority=False)
    assert sem._rt_available == 0
    assert len(sem._acquired_slots) == _SINGLE_SLOT

    with pytest.raises(TypeError):
        sem.release(was_high_priority=False)  # type: ignore[call-arg]

    # Explicit slot_id releases cleanly back to RT pool.
    sem.release(was_high_priority=False, slot_id=be_sid)
    assert sem._rt_available == _SINGLE_SLOT
    assert sem._be_available == 0
    assert len(sem._acquired_slots) == 0


@pytest.mark.asyncio
async def test_release_force_be_skips_cancelled_waiter_and_falls_through_to_rt() -> (
    None
):
    """When guaranteed bandwidth fires and the BE waiter was cancelled, release falls through to RT."""
    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
        guaranteed_be_ratio=_SINGLE_SLOT,
    )
    _, holder_sid = await sem.acquire(high_priority=True)

    be_task = asyncio.create_task(sem.acquire(high_priority=False))
    rt_task = asyncio.create_task(sem.acquire(high_priority=True))
    await asyncio.sleep(0)

    assert len(sem._be_queue) == _SINGLE_SLOT
    assert len(sem._rt_queue) == _SINGLE_SLOT

    # Cancelling be_task synchronously marks its awaited future cancelled before purge runs.
    be_task.cancel()

    # Guaranteed bandwidth triggers (consecutive_rt_releases reaches 1 == ratio),
    # pops the cancelled BE waiter, fails to wake it, exits force_be loop, and wakes RT.
    sem.release(was_high_priority=True, slot_id=holder_sid)

    with contextlib.suppress(asyncio.CancelledError):
        await be_task
    _, rt_sid = await asyncio.wait_for(rt_task, timeout=1.0)
    sem.release(was_high_priority=True, slot_id=rt_sid)
    assert sem._rt_available == _SINGLE_SLOT


@pytest.mark.asyncio
async def test_release_skips_cancelled_rt_waiter_and_wakes_next_rt_waiter() -> None:
    """Standard RT wake loop skips a cancelled RT waiter and wakes the next live RT waiter."""
    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
        guaranteed_be_ratio=0,
    )
    _, holder_sid = await sem.acquire(high_priority=True)

    rt_first = asyncio.create_task(sem.acquire(high_priority=True))
    rt_second = asyncio.create_task(sem.acquire(high_priority=True))
    await asyncio.sleep(0)

    assert len(sem._rt_queue) == _TWO_SLOTS
    rt_first.cancel()

    sem.release(was_high_priority=True, slot_id=holder_sid)

    with contextlib.suppress(asyncio.CancelledError):
        await rt_first
    _, rt_second_sid = await asyncio.wait_for(rt_second, timeout=1.0)
    sem.release(was_high_priority=True, slot_id=rt_second_sid)
    assert sem._rt_available == _SINGLE_SLOT


@pytest.mark.asyncio
async def test_release_skips_cancelled_be_waiter_and_wakes_next_be_waiter() -> None:
    """Standard BE wake loop skips a cancelled BE waiter and wakes the next live BE waiter."""
    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
        guaranteed_be_ratio=0,
    )
    _, holder_sid = await sem.acquire(high_priority=False)

    be_first = asyncio.create_task(sem.acquire(high_priority=False))
    be_second = asyncio.create_task(sem.acquire(high_priority=False))
    await asyncio.sleep(0)

    assert len(sem._be_queue) == _TWO_SLOTS
    be_first.cancel()

    sem.release(was_high_priority=False, slot_id=holder_sid)

    with contextlib.suppress(asyncio.CancelledError):
        await be_first
    _, be_second_sid = await asyncio.wait_for(be_second, timeout=1.0)
    sem.release(was_high_priority=False, slot_id=be_second_sid)
    assert sem._rt_available == _SINGLE_SLOT


@pytest.mark.asyncio
async def test_acquire_allows_immediate_rt_slot_when_be_queue_at_max_depth() -> None:
    """When queue depth hits max_queue_depth, a priority with an available slot still acquires immediately."""
    sem = HybridPrioritySemaphore(
        total_slots=_TWO_SLOTS,
        rt_reserved_slots=_SINGLE_SLOT,
        max_queue_depth=_SINGLE_SLOT,
        preemption_enabled=False,
    )
    _, be_holder_sid = await sem.acquire(high_priority=False)
    be_waiter = asyncio.create_task(sem.acquire(high_priority=False))
    await asyncio.sleep(0)

    assert len(sem._be_queue) == _SINGLE_SLOT
    assert sem._rt_available == _SINGLE_SLOT

    with pytest.raises(QueueFullError):
        await sem.acquire(high_priority=False)

    wait_time, rt_sid = await sem.acquire(high_priority=True)
    assert wait_time == 0.0

    sem.release(was_high_priority=True, slot_id=rt_sid)
    sem.release(was_high_priority=False, slot_id=be_holder_sid)
    _, be_waiter_sid = await asyncio.wait_for(be_waiter, timeout=1.0)
    sem.release(was_high_priority=False, slot_id=be_waiter_sid)


@pytest.mark.asyncio
async def test_acquire_non_string_future_result_falls_back_to_priority_pool() -> None:
    """If a waiter future receives a non-string truthy result, home_pool falls back to caller priority."""
    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
    )
    _, holder_sid = await sem.acquire(high_priority=True)
    be_task = asyncio.create_task(sem.acquire(high_priority=False))
    await asyncio.sleep(0)

    req = heapq.heappop(sem._be_queue)
    req.future.set_result(True)

    _, be_sid = await asyncio.wait_for(be_task, timeout=1.0)
    assert sem._acquired_slots[be_sid].home_pool == "be"

    # Clean up tracked slots cleanly.
    sem.release(was_high_priority=False, slot_id=be_sid)
    sem.release(was_high_priority=True, slot_id=holder_sid)


@pytest.mark.asyncio
async def test_acquire_logs_warning_on_long_queue_wait(
    monkeypatch: pytest.MonkeyPatch,
    caplog: pytest.LogCaptureFixture,
) -> None:
    """Queued acquire logs a warning when wait_time exceeds _LONG_QUEUE_WAIT_SECONDS."""
    caplog.set_level(logging.WARNING)
    clock_values = iter(
        [
            _INITIAL_MONOTONIC_TIME,  # __init__ _last_aging_sweep_time
            _INITIAL_MONOTONIC_TIME,  # holder acquire start_time
            _INITIAL_MONOTONIC_TIME,  # holder _track_acquire
            _INITIAL_MONOTONIC_TIME,  # waiter acquire start_time
            _INITIAL_MONOTONIC_TIME,  # waiter _compute_priority_score
            _INITIAL_MONOTONIC_TIME,  # release _should_run_aging_sweep
            _LONG_WAIT_MONOTONIC_TIME,  # waiter wait_time after await future
            _LONG_WAIT_MONOTONIC_TIME,  # waiter _track_acquire
        ]
    )

    def fake_monotonic() -> float:
        return next(clock_values, _LONG_WAIT_MONOTONIC_TIME)

    monkeypatch.setattr(
        "news_creator.gateway.hybrid_priority_semaphore.time.monotonic",
        fake_monotonic,
    )

    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
    )
    _, holder_sid = await sem.acquire(high_priority=True)
    waiter_task = asyncio.create_task(sem.acquire(high_priority=True))
    await asyncio.sleep(0)

    sem.release(was_high_priority=True, slot_id=holder_sid)
    wait_time, waiter_sid = await asyncio.wait_for(waiter_task, timeout=1.0)

    assert wait_time == _LONG_WAIT_MONOTONIC_TIME - _INITIAL_MONOTONIC_TIME
    assert any("Long queue wait detected" in rec.message for rec in caplog.records)
    sem.release(was_high_priority=True, slot_id=waiter_sid)


@pytest.mark.asyncio
async def test_acquire_cancelled_with_non_string_transfer_recovers_to_priority_pool() -> (
    None
):
    """Cancelled waiter with a non-string transferred future result recovers slot via priority fallback."""
    sem = HybridPrioritySemaphore(
        total_slots=_SINGLE_SLOT,
        rt_reserved_slots=_SINGLE_SLOT,
        preemption_enabled=False,
    )
    _, holder_sid = await sem.acquire(high_priority=True)
    rt_task = asyncio.create_task(sem.acquire(high_priority=True))
    await asyncio.sleep(0)

    # Simulate slot transfer with legacy non-str result, then cancel before task resumes.
    sem._track_release(holder_sid)
    req = heapq.heappop(sem._rt_queue)
    req.future.set_result(True)
    rt_task.cancel()

    with pytest.raises(asyncio.CancelledError):
        await rt_task

    assert sem._rt_available == _SINGLE_SLOT
    assert len(sem._acquired_slots) == 0
