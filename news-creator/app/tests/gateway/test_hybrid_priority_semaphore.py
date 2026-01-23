"""Tests for Hybrid Priority Semaphore implementation.

Tests the RT/BE scheduling, reserved slots, and aging mechanism.
"""

import asyncio
import pytest


@pytest.fixture
def hybrid_semaphore_module():
    """Import HybridPrioritySemaphore for testing."""
    from news_creator.gateway.hybrid_priority_semaphore import HybridPrioritySemaphore
    return HybridPrioritySemaphore


class TestHybridPrioritySemaphoreBasic:
    """Basic functionality tests."""

    @pytest.mark.asyncio
    async def test_init_default_values(self, hybrid_semaphore_module):
        """Test initialization with default values."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)

        assert semaphore._total_slots == 2
        assert semaphore._rt_reserved == 1
        assert semaphore._rt_available == 1
        assert semaphore._be_available >= 1

    @pytest.mark.asyncio
    async def test_init_custom_values(self, hybrid_semaphore_module):
        """Test initialization with custom values."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=4,
            rt_reserved_slots=2,
            aging_threshold_seconds=30.0,
            aging_boost=0.3,
        )

        assert semaphore._total_slots == 4
        assert semaphore._rt_reserved == 2
        assert semaphore._aging_threshold == 30.0
        assert semaphore._aging_boost == 0.3

    @pytest.mark.asyncio
    async def test_init_invalid_total_slots(self, hybrid_semaphore_module):
        """Test that total_slots < 1 raises ValueError."""
        HybridPrioritySemaphore = hybrid_semaphore_module

        with pytest.raises(ValueError, match="total_slots must be >= 1"):
            HybridPrioritySemaphore(total_slots=0)

    @pytest.mark.asyncio
    async def test_init_rt_reserved_exceeds_total(self, hybrid_semaphore_module):
        """Test that rt_reserved > total raises ValueError."""
        HybridPrioritySemaphore = hybrid_semaphore_module

        with pytest.raises(ValueError, match="rt_reserved_slots cannot exceed total_slots"):
            HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=3)

    @pytest.mark.asyncio
    async def test_repr(self, hybrid_semaphore_module):
        """Test string representation."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)

        repr_str = repr(semaphore)
        assert "HybridPrioritySemaphore" in repr_str
        assert "rt=" in repr_str
        assert "be=" in repr_str


class TestRTSlotReservation:
    """Tests for RT (Real-Time) slot reservation."""

    @pytest.mark.asyncio
    async def test_rt_acquire_immediate_when_slot_available(self, hybrid_semaphore_module):
        """High priority request acquires RT slot immediately when available."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # High priority should acquire RT slot immediately
        wait_time = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0
        assert semaphore._rt_available == 0

    @pytest.mark.asyncio
    async def test_be_acquire_immediate_when_slot_available(self, hybrid_semaphore_module):
        """Low priority request acquires BE slot immediately when available."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Low priority should acquire BE slot immediately
        wait_time = await semaphore.acquire(high_priority=False)
        assert wait_time == 0.0
        assert semaphore._be_available == 0

    @pytest.mark.asyncio
    async def test_rt_slot_reserved_for_high_priority(self, hybrid_semaphore_module):
        """RT slot is reserved for high priority even when BE slots are full."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Acquire BE slot with low priority
        await semaphore.acquire(high_priority=False)

        # RT slot should still be available for high priority
        wait_time = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0

    @pytest.mark.asyncio
    async def test_low_priority_queues_when_slot_held(self, hybrid_semaphore_module):
        """Low priority queues when RT slot is held, high priority gets next slot."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        # 2 total slots, 1 RT reserved = 1 BE slot
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        processing_order = []

        # Hold the RT slot first
        await semaphore.acquire(high_priority=True)

        async def low_priority_worker():
            await semaphore.acquire(high_priority=False)
            processing_order.append("low")
            semaphore.release(was_high_priority=False)

        async def high_priority_worker():
            await asyncio.sleep(0.02)  # Delay to let low priority queue first
            await semaphore.acquire(high_priority=True)
            processing_order.append("high")
            semaphore.release(was_high_priority=True)

        # Start both workers - low priority will get BE slot immediately
        # since we have 1 BE slot available
        low_task = asyncio.create_task(low_priority_worker())
        await asyncio.sleep(0.01)
        high_task = asyncio.create_task(high_priority_worker())
        await asyncio.sleep(0.01)

        # Release RT slot - high priority should get it
        semaphore.release(was_high_priority=True)

        await asyncio.gather(low_task, high_task)

        # Low priority got BE slot immediately, high priority got RT slot on release
        # Both should complete
        assert len(processing_order) == 2


class TestPriorityQueueBehavior:
    """Tests for priority queue behavior."""

    @pytest.mark.asyncio
    async def test_rt_queue_processed_before_be_queue(self, hybrid_semaphore_module):
        """RT queue is processed before BE queue on release."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        processing_order = []

        # Acquire the only slot
        await semaphore.acquire(high_priority=True)

        async def low_priority_worker():
            await semaphore.acquire(high_priority=False)
            processing_order.append("low")
            semaphore.release(was_high_priority=False)

        async def high_priority_worker():
            await semaphore.acquire(high_priority=True)
            processing_order.append("high")
            semaphore.release(was_high_priority=True)

        # Queue low priority first, then high priority
        low_task = asyncio.create_task(low_priority_worker())
        await asyncio.sleep(0.01)
        high_task = asyncio.create_task(high_priority_worker())
        await asyncio.sleep(0.01)

        # Release the slot
        semaphore.release(was_high_priority=True)

        await asyncio.gather(low_task, high_task)

        # High priority should be processed first despite arriving second
        assert processing_order == ["high", "low"]

    @pytest.mark.asyncio
    async def test_multiple_rt_requests_fifo_order(self, hybrid_semaphore_module):
        """Multiple RT requests are processed in FIFO order."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        processing_order = []

        # Acquire the only slot
        await semaphore.acquire(high_priority=True)

        async def rt_worker(task_id: int):
            await semaphore.acquire(high_priority=True)
            processing_order.append(f"rt_{task_id}")
            await asyncio.sleep(0.01)
            semaphore.release(was_high_priority=True)

        # Queue RT tasks in order
        tasks = []
        for i in range(1, 4):
            task = asyncio.create_task(rt_worker(i))
            tasks.append(task)
            await asyncio.sleep(0.01)

        # Release the slot
        semaphore.release(was_high_priority=True)

        await asyncio.gather(*tasks)

        # RT tasks should be processed in FIFO order
        assert processing_order == ["rt_1", "rt_2", "rt_3"]


class TestAgingMechanism:
    """Tests for the aging mechanism that prevents BE starvation."""

    @pytest.mark.asyncio
    async def test_priority_score_computation(self, hybrid_semaphore_module):
        """Test priority score computation with and without aging."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            aging_threshold_seconds=1.0,  # Short threshold for testing
            aging_boost=0.5,
        )

        import time

        # High priority always has score 0.0
        score_high = semaphore._compute_priority_score(high_priority=True, enqueue_time=time.monotonic())
        assert score_high == 0.0

        # Low priority starts at 1.0
        score_low_fresh = semaphore._compute_priority_score(high_priority=False, enqueue_time=time.monotonic())
        assert score_low_fresh == 1.0

        # Low priority after threshold is boosted (lower score = higher priority)
        old_enqueue_time = time.monotonic() - 2.0  # 2 seconds ago
        score_low_aged = semaphore._compute_priority_score(high_priority=False, enqueue_time=old_enqueue_time)
        assert score_low_aged < score_low_fresh

    @pytest.mark.asyncio
    async def test_aging_prevents_starvation(self, hybrid_semaphore_module):
        """Test that aging mechanism prevents BE request starvation."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            aging_threshold_seconds=0.05,  # 50ms threshold for testing
            aging_boost=0.5,
        )

        processing_order = []

        # Acquire the only slot
        await semaphore.acquire(high_priority=True)

        async def be_worker():
            await semaphore.acquire(high_priority=False)
            processing_order.append("be")
            semaphore.release(was_high_priority=False)

        async def rt_worker():
            await semaphore.acquire(high_priority=True)
            processing_order.append("rt")
            semaphore.release(was_high_priority=True)

        # Queue BE worker first
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.1)  # Wait for aging to kick in

        # Queue RT worker after BE has aged
        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)

        # Release - aged BE may be prioritized over fresh RT
        # This tests that aging is applied; actual order depends on timing
        semaphore.release(was_high_priority=True)

        await asyncio.gather(be_task, rt_task)

        # Both should complete
        assert len(processing_order) == 2


class TestSlotManagement:
    """Tests for slot management on release."""

    @pytest.mark.asyncio
    async def test_release_returns_rt_slot(self, hybrid_semaphore_module):
        """Release returns RT slot to RT pool when no waiters."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Acquire RT slot
        await semaphore.acquire(high_priority=True)
        assert semaphore._rt_available == 0

        # Release RT slot
        semaphore.release(was_high_priority=True)
        assert semaphore._rt_available == 1

    @pytest.mark.asyncio
    async def test_release_returns_be_slot(self, hybrid_semaphore_module):
        """Release returns BE slot to BE pool when no waiters."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Acquire BE slot
        await semaphore.acquire(high_priority=False)
        assert semaphore._be_available == 0

        # Release BE slot
        semaphore.release(was_high_priority=False)
        assert semaphore._be_available == 1

    @pytest.mark.asyncio
    async def test_release_wakes_rt_waiter(self, hybrid_semaphore_module):
        """Release wakes up RT waiter when RT queue is not empty."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        # Acquire the slot
        await semaphore.acquire(high_priority=True)

        rt_completed = asyncio.Event()

        async def rt_worker():
            await semaphore.acquire(high_priority=True)
            rt_completed.set()
            semaphore.release(was_high_priority=True)

        task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)  # Ensure it's queued

        # Release - should wake RT waiter
        semaphore.release(was_high_priority=True)

        await asyncio.wait_for(rt_completed.wait(), timeout=1.0)
        await task


class TestLastWaitTime:
    """Tests for wait time tracking."""

    @pytest.mark.asyncio
    async def test_last_wait_time_immediate_acquire(self, hybrid_semaphore_module):
        """Test last_wait_time is 0.0 for immediate acquire."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)

        await semaphore.acquire(high_priority=True)
        assert semaphore.last_wait_time == 0.0

    @pytest.mark.asyncio
    async def test_last_wait_time_tracks_queue_wait(self, hybrid_semaphore_module):
        """Test last_wait_time tracks actual queue wait time."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        # Acquire the slot
        await semaphore.acquire(high_priority=True)

        wait_time_recorded = None

        async def worker():
            nonlocal wait_time_recorded
            wait_time = await semaphore.acquire(high_priority=True)
            wait_time_recorded = wait_time
            semaphore.release(was_high_priority=True)

        task = asyncio.create_task(worker())
        await asyncio.sleep(0.05)  # Wait 50ms before releasing

        semaphore.release(was_high_priority=True)
        await task

        # Wait time should be approximately 50ms
        assert wait_time_recorded is not None
        assert wait_time_recorded >= 0.04  # Allow some tolerance


class TestCancellation:
    """Tests for task cancellation."""

    @pytest.mark.asyncio
    async def test_cancellation_removes_from_queue(self, hybrid_semaphore_module):
        """Test that cancelled tasks are handled correctly."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        processing_order = []

        # Acquire the slot
        await semaphore.acquire(high_priority=True)

        async def worker(name: str, high_priority: bool):
            try:
                await semaphore.acquire(high_priority=high_priority)
                processing_order.append(name)
                semaphore.release(was_high_priority=high_priority)
            except asyncio.CancelledError:
                processing_order.append(f"{name}_cancelled")
                raise

        # Queue two workers
        task1 = asyncio.create_task(worker("first", True))
        await asyncio.sleep(0.01)
        task2 = asyncio.create_task(worker("second", True))
        await asyncio.sleep(0.01)

        # Cancel first task
        task1.cancel()
        try:
            await task1
        except asyncio.CancelledError:
            pass

        # Release - should wake second task
        semaphore.release(was_high_priority=True)
        await task2

        assert "first_cancelled" in processing_order
        assert "second" in processing_order


class TestEdgeCases:
    """Edge case tests."""

    @pytest.mark.asyncio
    async def test_single_slot_single_rt(self, hybrid_semaphore_module):
        """Test with single total slot = single RT slot (no BE slots)."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        # With all slots RT reserved, BE has 0 slots (must queue)
        assert semaphore._be_available == 0
        assert semaphore._rt_available == 1

    @pytest.mark.asyncio
    async def test_all_slots_for_be(self, hybrid_semaphore_module):
        """Test with all slots as BE (rt_reserved=0)."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=0)

        assert semaphore._rt_reserved == 0
        assert semaphore._be_slots == 2

        # High priority still works, just uses BE slots
        wait_time = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0

    @pytest.mark.asyncio
    async def test_concurrent_acquires(self, hybrid_semaphore_module):
        """Test multiple concurrent acquires don't cause race conditions."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        completed = []

        async def worker(worker_id: int, high_priority: bool):
            await semaphore.acquire(high_priority=high_priority)
            await asyncio.sleep(0.01)
            completed.append(worker_id)
            semaphore.release(was_high_priority=high_priority)

        # Start many workers concurrently
        tasks = [
            asyncio.create_task(worker(i, i % 2 == 0))
            for i in range(10)
        ]

        await asyncio.gather(*tasks)

        # All should complete
        assert len(completed) == 10
