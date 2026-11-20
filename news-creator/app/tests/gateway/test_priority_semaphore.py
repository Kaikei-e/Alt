"""Tests for Priority Semaphore implementation."""

import asyncio
import contextlib

import pytest

from news_creator.gateway.priority_semaphore import PrioritySemaphore

IMMEDIATE_WAIT_MAX_SECONDS = 0.1
FIRST_TASK_WAIT_MAX_SECONDS = 0.02


@pytest.fixture
def priority_semaphore_module() -> type[PrioritySemaphore]:
    """Import PrioritySemaphore for testing."""
    return PrioritySemaphore


@pytest.mark.asyncio
async def test_priority_semaphore_basic_acquire_release(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test basic acquire and release functionality."""
    semaphore = priority_semaphore_module(1)

    # Should acquire immediately (default low priority)
    wait_time = await semaphore.acquire(high_priority=False)
    assert semaphore._value == 0
    assert wait_time < IMMEDIATE_WAIT_MAX_SECONDS  # Immediate acquire

    # Release
    semaphore.release()
    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_priority_semaphore_high_priority_acquire(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test high priority acquire functionality."""
    semaphore = priority_semaphore_module(1)

    # Should acquire immediately with high priority
    wait_time = await semaphore.acquire(high_priority=True)
    assert semaphore._value == 0
    assert wait_time < IMMEDIATE_WAIT_MAX_SECONDS  # Immediate acquire

    # Release
    semaphore.release()
    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_priority_semaphore_context_manager(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test priority semaphore as context manager (low priority by default)."""
    semaphore = priority_semaphore_module(1)

    async with semaphore:
        assert semaphore._value == 0

    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_priority_semaphore_high_priority_first(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that high priority requests are processed before low priority ones."""
    semaphore = priority_semaphore_module(1)

    processing_order = []

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)

    # 2. Start low priority task first (will wait in queue)
    async def low_priority_worker() -> None:
        await semaphore.acquire(high_priority=False)
        processing_order.append("low")
        semaphore.release()

    # 3. Start high priority task second (should be processed first when released)
    async def high_priority_worker() -> None:
        await semaphore.acquire(high_priority=True)
        processing_order.append("high")
        semaphore.release()

    # Start both workers - low priority first
    low_task = asyncio.create_task(low_priority_worker())
    await asyncio.sleep(0.01)  # Ensure low priority is queued first
    high_task = asyncio.create_task(high_priority_worker())
    await asyncio.sleep(0.01)  # Ensure high priority is queued

    # 4. Release semaphore - high priority should be woken first
    semaphore.release()

    # Wait for both tasks to complete
    await asyncio.gather(low_task, high_task)

    # High priority should have been processed first despite arriving second
    assert processing_order == ["high", "low"], (
        f"Expected high priority first, got {processing_order}"
    )


@pytest.mark.asyncio
async def test_priority_semaphore_multiple_high_priority_fifo(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that multiple high priority requests maintain FIFO order among themselves."""
    semaphore = priority_semaphore_module(1)

    processing_order = []
    lock = asyncio.Lock()

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)

    # 2. Start multiple high priority tasks in sequence
    async def high_priority_worker(task_id: int) -> None:
        await semaphore.acquire(high_priority=True)
        async with lock:
            processing_order.append(f"high_{task_id}")
        await asyncio.sleep(0.01)  # Simulate work
        semaphore.release()

    # Create tasks in order: 1, 2, 3
    tasks = []
    for i in range(1, 4):
        task = asyncio.create_task(high_priority_worker(i))
        tasks.append(task)
        await asyncio.sleep(0.01)  # Ensure they queue in order

    # 3. Release semaphore
    semaphore.release()

    # Wait for all tasks
    await asyncio.gather(*tasks)

    # High priority tasks should be processed in FIFO order
    assert processing_order == ["high_1", "high_2", "high_3"], (
        f"Expected FIFO order for high priority, got {processing_order}"
    )


@pytest.mark.asyncio
async def test_priority_semaphore_mixed_priority_order(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test mixed priority requests - all high priority before any low priority."""
    semaphore = priority_semaphore_module(1)

    processing_order = []
    lock = asyncio.Lock()

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)

    # 2. Queue tasks in mixed order: low1, high1, low2, high2
    async def worker(priority: str, task_id: int) -> None:
        is_high = priority == "high"
        await semaphore.acquire(high_priority=is_high)
        async with lock:
            processing_order.append(f"{priority}_{task_id}")
        await asyncio.sleep(0.01)  # Simulate work
        semaphore.release()

    # Create tasks in mixed order
    tasks = []
    for priority, task_id in [("low", 1), ("high", 1), ("low", 2), ("high", 2)]:
        task = asyncio.create_task(worker(priority, task_id))
        tasks.append(task)
        await asyncio.sleep(0.01)  # Ensure they queue in order

    # 3. Release semaphore
    semaphore.release()

    # Wait for all tasks
    await asyncio.gather(*tasks)

    # All high priority should come first (in FIFO order), then low priority (in FIFO order)
    assert processing_order == ["high_1", "high_2", "low_1", "low_2"], (
        f"Expected high priority first then low priority, got {processing_order}"
    )


@pytest.mark.asyncio
async def test_priority_semaphore_concurrent_slots(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test priority semaphore with multiple concurrent slots."""
    semaphore = priority_semaphore_module(2)  # Allow 2 concurrent tasks

    processing_order = []
    lock = asyncio.Lock()

    async def worker(priority: str, task_id: int) -> None:
        is_high = priority == "high"
        await semaphore.acquire(high_priority=is_high)
        async with lock:
            processing_order.append(f"{priority}_{task_id}")
        await asyncio.sleep(0.01)  # Simulate work
        semaphore.release()

    # Start tasks in sequence
    task_specs = [("low", 1), ("low", 2), ("high", 1), ("high", 2)]
    tasks = []
    for priority, task_id in task_specs:
        task = asyncio.create_task(worker(priority, task_id))
        tasks.append(task)
        await asyncio.sleep(0.005)  # Small delay to ensure order

    await asyncio.gather(*tasks)

    # With 2 slots, first 2 tasks (low_1, low_2) start immediately
    # Then high_1, high_2 should process (high priority goes first when released)
    assert len(processing_order) == len(task_specs)
    # First two should be low_1 and low_2 (they got slots immediately)
    assert set(processing_order[:2]) == {"low_1", "low_2"}


@pytest.mark.asyncio
async def test_priority_semaphore_cancellation(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that cancellation works correctly for priority semaphore."""
    semaphore = priority_semaphore_module(1)

    processing_order = []

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)
    processing_order.append("initial")

    # 2. Start two tasks that will wait
    async def worker(priority: str, task_id: int) -> None:
        is_high = priority == "high"
        try:
            await semaphore.acquire(high_priority=is_high)
            processing_order.append(f"{priority}_{task_id}")
            semaphore.release()
        except asyncio.CancelledError:
            processing_order.append(f"{priority}_{task_id}_cancelled")
            raise

    # Start high priority task, then low priority task
    high_task = asyncio.create_task(worker("high", 1))
    await asyncio.sleep(0.01)
    low_task = asyncio.create_task(worker("low", 1))
    await asyncio.sleep(0.01)

    # 3. Cancel the high priority task
    high_task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await high_task

    # 4. Release semaphore - should wake low priority task
    semaphore.release()

    # Wait for low priority task to complete
    await low_task

    # Verify: initial ran, high cancelled, low ran
    assert processing_order == ["initial", "high_1_cancelled", "low_1"]


@pytest.mark.asyncio
async def test_priority_semaphore_tracks_wait_time(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that priority semaphore tracks queue wait time."""
    semaphore = priority_semaphore_module(1)

    # First acquire immediately - should have minimal wait time
    wait_time = await semaphore.acquire(high_priority=True)
    assert wait_time >= 0  # Should return wait time in seconds
    assert wait_time < IMMEDIATE_WAIT_MAX_SECONDS  # Immediate acquire should be fast
    semaphore.release()


@pytest.mark.asyncio
async def test_priority_semaphore_wait_time_for_queued_requests(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that queue wait time is tracked when actually waiting."""
    semaphore = priority_semaphore_module(1)

    wait_times = []

    async def worker(priority: str, task_id: int) -> None:
        """Worker that tracks wait time."""
        is_high = priority == "high"
        wait_time = await semaphore.acquire(high_priority=is_high)
        try:
            wait_times.append((f"{priority}_{task_id}", wait_time))
            await asyncio.sleep(0.05)  # Hold semaphore for 50ms
        finally:
            semaphore.release()

    # Start 3 tasks concurrently - high priority should be favored
    workers = [worker("low", 1), worker("high", 1), worker("low", 2)]
    await asyncio.gather(*workers)

    assert len(wait_times) == len(workers)

    # First completed task should have minimal wait
    first_wait = wait_times[0][1]
    assert first_wait < FIRST_TASK_WAIT_MAX_SECONDS


@pytest.mark.asyncio
async def test_priority_semaphore_last_wait_time_property(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that last_wait_time property is available."""
    semaphore = priority_semaphore_module(1)

    # After acquire, last_wait_time should be available
    await semaphore.acquire(high_priority=False)
    assert semaphore.last_wait_time >= 0
    assert semaphore.last_wait_time < IMMEDIATE_WAIT_MAX_SECONDS  # Immediate acquire
    semaphore.release()


@pytest.mark.asyncio
async def test_priority_semaphore_negative_value_raises(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test that negative initial value raises ValueError."""
    with pytest.raises(ValueError, match="Semaphore initial value must be >= 0"):
        priority_semaphore_module(-1)


@pytest.mark.asyncio
async def test_priority_semaphore_repr(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test string representation of priority semaphore."""
    semaphore = priority_semaphore_module(2)

    repr_str = repr(semaphore)
    assert "PrioritySemaphore" in repr_str
    assert "2" in repr_str  # Max value


@pytest.mark.asyncio
async def test_priority_semaphore_locked(
    priority_semaphore_module: type[PrioritySemaphore],
) -> None:
    """Test locked() method."""
    semaphore = priority_semaphore_module(1)

    # Initially not locked
    assert not semaphore.locked()

    # After acquire, should be locked
    await semaphore.acquire(high_priority=False)
    assert semaphore.locked()

    # After release, should not be locked
    semaphore.release()
    assert not semaphore.locked()
