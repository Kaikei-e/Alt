"""Tests for FIFO Semaphore implementation."""

import asyncio
import pytest


@pytest.fixture
def fifo_semaphore_module():
    """Import FIFOSemaphore for testing."""
    from news_creator.gateway.fifo_semaphore import FIFOSemaphore
    return FIFOSemaphore


@pytest.mark.asyncio
async def test_fifo_semaphore_basic_acquire_release(fifo_semaphore_module):
    """Test basic acquire and release functionality."""
    FIFOSemaphore = fifo_semaphore_module
    semaphore = FIFOSemaphore(1)

    # Should acquire immediately
    await semaphore.acquire()
    assert semaphore._value == 0

    # Release
    semaphore.release()
    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_fifo_semaphore_context_manager(fifo_semaphore_module):
    """Test FIFO semaphore as context manager."""
    FIFOSemaphore = fifo_semaphore_module
    semaphore = FIFOSemaphore(1)

    async with semaphore:
        assert semaphore._value == 0

    assert semaphore._value == 1


@pytest.mark.asyncio
async def test_fifo_semaphore_fifo_order(fifo_semaphore_module):
    """Test that FIFO semaphore processes tasks in FIFO order."""
    FIFOSemaphore = fifo_semaphore_module
    semaphore = FIFOSemaphore(1)

    processing_order = []
    lock = asyncio.Lock()

    async def worker(task_id: int):
        """Worker that processes a task."""
        await semaphore.acquire()
        try:
            async with lock:
                processing_order.append(task_id)
            await asyncio.sleep(0.01)  # Simulate work
        finally:
            semaphore.release()

    # Start 5 tasks concurrently
    tasks = [worker(i) for i in range(1, 6)]
    await asyncio.gather(*tasks)

    # Verify FIFO order
    assert processing_order == [1, 2, 3, 4, 5], (
        f"Expected FIFO order [1, 2, 3, 4, 5], got {processing_order}"
    )


@pytest.mark.asyncio
async def test_fifo_semaphore_concurrent_access(fifo_semaphore_module):
    """Test FIFO semaphore with concurrent access."""
    FIFOSemaphore = fifo_semaphore_module
    semaphore = FIFOSemaphore(2)  # Allow 2 concurrent tasks

    processing_order = []
    lock = asyncio.Lock()

    async def worker(task_id: int):
        """Worker that processes a task."""
        async with semaphore:
            async with lock:
                processing_order.append(task_id)
            await asyncio.sleep(0.01)  # Simulate work

    # Start 5 tasks concurrently
    tasks = [worker(i) for i in range(1, 6)]
    await asyncio.gather(*tasks)

    # With concurrency=2, first 2 should start together, then next 2, then last 1
    # But order within each batch may vary, so we just verify all are processed
    assert len(processing_order) == 5
    assert set(processing_order) == {1, 2, 3, 4, 5}

    # First two should be 1 and 2 (in some order)
    assert set(processing_order[:2]) == {1, 2}


@pytest.mark.asyncio
async def test_fifo_semaphore_cancellation(fifo_semaphore_module):
    """Test that cancellation of a waiting task doesn't break FIFO order for others."""
    FIFOSemaphore = fifo_semaphore_module
    semaphore = FIFOSemaphore(1)

    processing_order = []

    # 1. Acquire the semaphore initially
    await semaphore.acquire()
    processing_order.append(1)

    # 2. Start two tasks that will wait
    async def worker(task_id: int):
        try:
            await semaphore.acquire()
            processing_order.append(task_id)
            semaphore.release()
        except asyncio.CancelledError:
            processing_order.append(f"{task_id}_cancelled")
            raise

    task2 = asyncio.create_task(worker(2))
    task3 = asyncio.create_task(worker(3))

    # Wait a bit to ensure they are in the queue (FIFO: 2, then 3)
    await asyncio.sleep(0.01)

    # 3. Cancel the first waiter (Task 2)
    task2.cancel()
    try:
        await task2
    except asyncio.CancelledError:
        pass

    # 4. Release semaphore - should wake Task 3 (since 2 is cancelled)
    semaphore.release()

    # Wait for Task 3 to complete
    await task3

    # Verify: 1 ran, 2 cancelled, 3 ran (skipped 2)
    assert processing_order == [1, "2_cancelled", 3]

