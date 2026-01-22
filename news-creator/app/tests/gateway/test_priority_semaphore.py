"""Tests for Priority Semaphore implementation."""

import asyncio
import pytest


@pytest.fixture
def priority_semaphore_module():
    """Import PrioritySemaphore for testing."""
    from news_creator.gateway.priority_semaphore import PrioritySemaphore
    return PrioritySemaphore


@pytest.mark.asyncio
async def test_priority_semaphore_basic_acquire_release(priority_semaphore_module):
    """Test basic acquire and release functionality."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    # Should acquire immediately (default low priority)
    wait_time = await semaphore.acquire(high_priority=False)
    assert semaphore._value == 0
    assert wait_time < 0.1  # Immediate acquire

    # Release
    semaphore.release()
    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_priority_semaphore_high_priority_acquire(priority_semaphore_module):
    """Test high priority acquire functionality."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    # Should acquire immediately with high priority
    wait_time = await semaphore.acquire(high_priority=True)
    assert semaphore._value == 0
    assert wait_time < 0.1  # Immediate acquire

    # Release
    semaphore.release()
    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_priority_semaphore_context_manager(priority_semaphore_module):
    """Test priority semaphore as context manager (low priority by default)."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    async with semaphore:
        assert semaphore._value == 0

    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_priority_semaphore_high_priority_first(priority_semaphore_module):
    """Test that high priority requests are processed before low priority ones."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    processing_order = []

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)

    # 2. Start low priority task first (will wait in queue)
    async def low_priority_worker():
        await semaphore.acquire(high_priority=False)
        processing_order.append("low")
        semaphore.release()

    # 3. Start high priority task second (should be processed first when released)
    async def high_priority_worker():
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
async def test_priority_semaphore_multiple_high_priority_fifo(priority_semaphore_module):
    """Test that multiple high priority requests maintain FIFO order among themselves."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    processing_order = []
    lock = asyncio.Lock()

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)

    # 2. Start multiple high priority tasks in sequence
    async def high_priority_worker(task_id: int):
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
async def test_priority_semaphore_mixed_priority_order(priority_semaphore_module):
    """Test mixed priority requests - all high priority before any low priority."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    processing_order = []
    lock = asyncio.Lock()

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)

    # 2. Queue tasks in mixed order: low1, high1, low2, high2
    async def worker(priority: str, task_id: int):
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
async def test_priority_semaphore_concurrent_slots(priority_semaphore_module):
    """Test priority semaphore with multiple concurrent slots."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(2)  # Allow 2 concurrent tasks

    processing_order = []
    lock = asyncio.Lock()

    async def worker(priority: str, task_id: int):
        is_high = priority == "high"
        await semaphore.acquire(high_priority=is_high)
        async with lock:
            processing_order.append(f"{priority}_{task_id}")
        await asyncio.sleep(0.01)  # Simulate work
        semaphore.release()

    # Start tasks in sequence
    tasks = []
    for priority, task_id in [("low", 1), ("low", 2), ("high", 1), ("high", 2)]:
        task = asyncio.create_task(worker(priority, task_id))
        tasks.append(task)
        await asyncio.sleep(0.005)  # Small delay to ensure order

    await asyncio.gather(*tasks)

    # With 2 slots, first 2 tasks (low_1, low_2) start immediately
    # Then high_1, high_2 should process (high priority goes first when released)
    assert len(processing_order) == 4
    # First two should be low_1 and low_2 (they got slots immediately)
    assert set(processing_order[:2]) == {"low_1", "low_2"}


@pytest.mark.asyncio
async def test_priority_semaphore_cancellation(priority_semaphore_module):
    """Test that cancellation works correctly for priority semaphore."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    processing_order = []

    # 1. Acquire the semaphore initially
    await semaphore.acquire(high_priority=False)
    processing_order.append("initial")

    # 2. Start two tasks that will wait
    async def worker(priority: str, task_id: int):
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
    try:
        await high_task
    except asyncio.CancelledError:
        pass

    # 4. Release semaphore - should wake low priority task
    semaphore.release()

    # Wait for low priority task to complete
    await low_task

    # Verify: initial ran, high cancelled, low ran
    assert processing_order == ["initial", "high_1_cancelled", "low_1"]


@pytest.mark.asyncio
async def test_priority_semaphore_tracks_wait_time(priority_semaphore_module):
    """Test that priority semaphore tracks queue wait time."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    # First acquire immediately - should have minimal wait time
    wait_time = await semaphore.acquire(high_priority=True)
    assert wait_time >= 0  # Should return wait time in seconds
    assert wait_time < 0.1  # Immediate acquire should be very fast
    semaphore.release()


@pytest.mark.asyncio
async def test_priority_semaphore_wait_time_for_queued_requests(priority_semaphore_module):
    """Test that queue wait time is tracked when actually waiting."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    wait_times = []

    async def worker(priority: str, task_id: int):
        """Worker that tracks wait time."""
        is_high = priority == "high"
        wait_time = await semaphore.acquire(high_priority=is_high)
        try:
            wait_times.append((f"{priority}_{task_id}", wait_time))
            await asyncio.sleep(0.05)  # Hold semaphore for 50ms
        finally:
            semaphore.release()

    # Start 3 tasks concurrently - high priority should be favored
    await asyncio.gather(
        worker("low", 1),
        worker("high", 1),
        worker("low", 2),
    )

    assert len(wait_times) == 3

    # First completed task should have minimal wait
    first_wait = wait_times[0][1]
    assert first_wait < 0.02


@pytest.mark.asyncio
async def test_priority_semaphore_last_wait_time_property(priority_semaphore_module):
    """Test that last_wait_time property is available."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    # After acquire, last_wait_time should be available
    await semaphore.acquire(high_priority=False)
    assert semaphore.last_wait_time >= 0
    assert semaphore.last_wait_time < 0.1  # Immediate acquire
    semaphore.release()


@pytest.mark.asyncio
async def test_priority_semaphore_negative_value_raises(priority_semaphore_module):
    """Test that negative initial value raises ValueError."""
    PrioritySemaphore = priority_semaphore_module

    with pytest.raises(ValueError, match="Semaphore initial value must be >= 0"):
        PrioritySemaphore(-1)


@pytest.mark.asyncio
async def test_priority_semaphore_repr(priority_semaphore_module):
    """Test string representation of priority semaphore."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(2)

    repr_str = repr(semaphore)
    assert "PrioritySemaphore" in repr_str
    assert "2" in repr_str  # Max value


@pytest.mark.asyncio
async def test_priority_semaphore_locked(priority_semaphore_module):
    """Test locked() method."""
    PrioritySemaphore = priority_semaphore_module
    semaphore = PrioritySemaphore(1)

    # Initially not locked
    assert not semaphore.locked()

    # After acquire, should be locked
    await semaphore.acquire(high_priority=False)
    assert semaphore.locked()

    # After release, should not be locked
    semaphore.release()
    assert not semaphore.locked()
