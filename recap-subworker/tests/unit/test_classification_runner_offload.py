"""Tests verifying ClassificationRunner offloads blocking calls off the event loop thread."""

from __future__ import annotations

import asyncio
import threading
from unittest.mock import MagicMock

import pytest

from recap_subworker.infra.config import Settings
from recap_subworker.services.classification_runner import ClassificationRunner


@pytest.mark.asyncio
async def test_classification_runner_predict_batch_offloads_acquire_pool():
    """_acquire_pool must execute on a worker thread, not on the event loop thread."""
    loop_thread_id = threading.get_ident()
    called_thread_id = None

    settings = Settings()
    runner = ClassificationRunner(settings)

    fake_pool = MagicMock()
    fake_async_result = MagicMock()
    fake_async_result.get.return_value = [{"label": "test", "score": 0.99}]
    fake_pool.apply_async.return_value = fake_async_result

    def fake_acquire_pool():
        nonlocal called_thread_id
        called_thread_id = threading.get_ident()
        return fake_pool

    runner._acquire_pool = fake_acquire_pool

    result = await runner.predict_batch(["sample text"])

    assert result == [{"label": "test", "score": 0.99}]
    assert called_thread_id is not None
    assert called_thread_id != loop_thread_id, (
        f"_acquire_pool ran on event loop thread ({called_thread_id}); "
        "must be offloaded via asyncio.to_thread"
    )


@pytest.mark.asyncio
async def test_classification_runner_predict_batch_offloads_task_lifecycle_and_result_get():
    """Task acquire, result waiting, and task finish must all run off the event loop thread."""
    loop_thread_id = threading.get_ident()
    acquire_thread_id = None
    get_thread_id = None
    finish_thread_id = None

    settings = Settings()
    runner = ClassificationRunner(settings)

    fake_pool = MagicMock()
    fake_async_result = MagicMock()

    def fake_get(timeout):
        nonlocal get_thread_id
        get_thread_id = threading.get_ident()
        return [{"label": "test", "score": 0.99}]

    fake_async_result.get.side_effect = fake_get
    fake_pool.apply_async.return_value = fake_async_result

    orig_acquire = getattr(runner, "_acquire_pool", None)
    orig_finish = runner._finish_task

    def fake_acquire():
        nonlocal acquire_thread_id
        acquire_thread_id = threading.get_ident()
        runner._pool = fake_pool
        if orig_acquire is not None:
            return orig_acquire()
        return fake_pool

    def fake_finish():
        nonlocal finish_thread_id
        finish_thread_id = threading.get_ident()
        return orig_finish()

    runner._acquire_pool = fake_acquire
    runner._finish_task = fake_finish

    result = await runner.predict_batch(["sample text"])

    assert result == [{"label": "test", "score": 0.99}]
    assert acquire_thread_id is not None
    assert acquire_thread_id != loop_thread_id, (
        f"_acquire_pool ran on event loop thread ({acquire_thread_id}); "
        "must be offloaded via asyncio.to_thread"
    )
    assert get_thread_id is not None
    assert get_thread_id != loop_thread_id, (
        f"async_result.get ran on event loop thread ({get_thread_id}); "
        "must be offloaded via asyncio.to_thread"
    )
    assert finish_thread_id is not None
    assert finish_thread_id != loop_thread_id, (
        f"_finish_task ran on event loop thread ({finish_thread_id}); "
        "must be offloaded via asyncio.to_thread"
    )


@pytest.mark.asyncio
async def test_cancellation_during_slow_acquire_leaves_active_tasks_zero():
    """Cancellation during slow acquire leaves _active_tasks at 0 after acquire completes."""
    settings = Settings()
    runner = ClassificationRunner(settings)

    acquire_started = threading.Event()
    acquire_proceed = threading.Event()

    fake_pool = MagicMock()
    fake_async_result = MagicMock()
    fake_async_result.get.return_value = [{"label": "test", "score": 0.99}]
    fake_pool.apply_async.return_value = fake_async_result

    def slow_acquire():
        acquire_started.set()
        acquire_proceed.wait(timeout=5.0)
        with runner._lock:
            runner._active_tasks += 1
            runner._pool = fake_pool
            return fake_pool

    runner._acquire_pool = slow_acquire

    task = asyncio.create_task(runner.predict_batch(["sample text"]))

    # Wait for acquire to start in worker thread
    await asyncio.to_thread(acquire_started.wait, 5.0)
    assert acquire_started.is_set()

    # Cancel predict_batch while acquire is in progress
    task.cancel()

    with pytest.raises(asyncio.CancelledError):
        await task

    # Allow acquire to finish
    acquire_proceed.set()

    # Wait for the done-callback / finish_task thread to release the task
    for _ in range(50):
        if runner._active_tasks == 0:
            break
        await asyncio.sleep(0.02)

    assert runner._active_tasks == 0, (
        f"_active_tasks was {runner._active_tasks} after acquire completed on cancelled task; "
        "must be 0"
    )


def test_idle_timer_cannot_close_pool_between_ensure_and_increment():
    """_acquire_pool ensures pool, cancels timer, and increments active tasks under single lock hold."""
    settings = Settings()
    runner = ClassificationRunner(settings)

    fake_pool = MagicMock()
    lock_held_during_ensure = False

    def fake_ensure():
        nonlocal lock_held_during_ensure
        lock_held_during_ensure = runner._lock.locked()
        runner._pool = fake_pool
        return fake_pool

    runner._ensure_pool_locked = fake_ensure

    timer_cancelled = False
    timer_mock = MagicMock()

    def fake_cancel():
        nonlocal timer_cancelled
        timer_cancelled = True

    timer_mock.cancel.side_effect = fake_cancel
    runner._idle_timer = timer_mock

    assert hasattr(runner, "_acquire_pool"), "runner must define synchronous _acquire_pool"
    pool = runner._acquire_pool()

    assert pool is fake_pool
    assert lock_held_during_ensure is True
    assert timer_cancelled is True
    assert runner._active_tasks == 1


def test_shutdown_returns_within_timeout_when_idle_timer_scheduled():
    """shutdown() must not deadlock with self._lock when an idle timer is scheduled."""
    settings = Settings()
    runner = ClassificationRunner(settings)

    idle_timer = threading.Timer(60.0, lambda: None)
    runner._idle_timer = idle_timer
    idle_timer.start()

    try:
        t = threading.Thread(target=runner.shutdown)
        t.start()
        t.join(timeout=1.0)
        assert not t.is_alive(), "shutdown() deadlocked on self._lock!"
    finally:
        idle_timer.cancel()
        if t.is_alive() and runner._lock.locked():
            try:
                runner._lock.release()
            except RuntimeError:
                pass
            t.join(timeout=0.5)
