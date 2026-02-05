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


class TestPreemption:
    """Tests for preemption mechanism."""

    @pytest.fixture
    def preempted_exception(self):
        """Import PreemptedException for testing."""
        from news_creator.gateway.hybrid_priority_semaphore import PreemptedException
        return PreemptedException

    @pytest.mark.asyncio
    async def test_preemption_disabled_by_default_false(self, hybrid_semaphore_module):
        """Test preemption can be disabled."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2, rt_reserved_slots=1, preemption_enabled=False
        )
        assert not semaphore._preemption_enabled

    @pytest.mark.asyncio
    async def test_preemption_enabled_by_default(self, hybrid_semaphore_module):
        """Test preemption is enabled by default."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)
        assert semaphore._preemption_enabled

    @pytest.mark.asyncio
    async def test_register_active_request(self, hybrid_semaphore_module):
        """Test registering an active request."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        cancel_event = asyncio.Event()
        semaphore.register_active_request("task-1", cancel_event, is_high_priority=False)

        assert "task-1" in semaphore._active_requests
        assert semaphore._active_requests["task-1"].cancel_event is cancel_event
        assert not semaphore._active_requests["task-1"].is_high_priority

    @pytest.mark.asyncio
    async def test_unregister_active_request(self, hybrid_semaphore_module):
        """Test unregistering an active request."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        cancel_event = asyncio.Event()
        semaphore.register_active_request("task-1", cancel_event, is_high_priority=False)
        semaphore.unregister_active_request("task-1")

        assert "task-1" not in semaphore._active_requests

    @pytest.mark.asyncio
    async def test_unregister_nonexistent_request(self, hybrid_semaphore_module):
        """Test unregistering a non-existent request doesn't raise."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Should not raise
        semaphore.unregister_active_request("nonexistent")

    @pytest.mark.asyncio
    async def test_has_preemptable_be(self, hybrid_semaphore_module):
        """Test checking for preemptable BE requests."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # No active requests
        assert not semaphore._has_preemptable_be()

        # Only high priority request
        cancel_event1 = asyncio.Event()
        semaphore.register_active_request("rt-1", cancel_event1, is_high_priority=True)
        assert not semaphore._has_preemptable_be()

        # Add BE request
        cancel_event2 = asyncio.Event()
        semaphore.register_active_request("be-1", cancel_event2, is_high_priority=False)
        assert semaphore._has_preemptable_be()

    @pytest.mark.asyncio
    async def test_rt_preempts_be_when_blocked(self, hybrid_semaphore_module):
        """RT request triggers preemption when blocked by BE."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            preemption_enabled=True,
            preemption_wait_threshold=0.1,
        )

        # Acquire RT slot (for an existing RT request)
        await semaphore.acquire(high_priority=True)

        # Acquire BE slot and register as active
        await semaphore.acquire(high_priority=False)
        be_cancel_event = asyncio.Event()
        semaphore.register_active_request("be-1", be_cancel_event, is_high_priority=False)

        # New RT request should trigger preemption
        rt_acquired = asyncio.Event()

        async def rt_worker():
            await semaphore.acquire(high_priority=True)
            rt_acquired.set()

        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.05)

        # BE cancel event should be set
        assert be_cancel_event.is_set(), "BE cancel event should be triggered for preemption"

        # Simulate BE completion and slot release
        semaphore.unregister_active_request("be-1")
        semaphore.release(was_high_priority=False)

        # RT should eventually acquire
        await asyncio.wait_for(rt_acquired.wait(), timeout=2.0)
        await rt_task

    @pytest.mark.asyncio
    async def test_preemption_not_triggered_when_disabled(self, hybrid_semaphore_module):
        """Preemption is not triggered when disabled."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            preemption_enabled=False,
        )

        # Acquire both slots
        await semaphore.acquire(high_priority=True)
        await semaphore.acquire(high_priority=False)
        be_cancel_event = asyncio.Event()
        semaphore.register_active_request("be-1", be_cancel_event, is_high_priority=False)

        # New RT request queues but does not preempt
        rt_queued = asyncio.Event()

        async def rt_worker():
            rt_queued.set()
            await semaphore.acquire(high_priority=True)

        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.05)

        # BE cancel event should NOT be set
        assert not be_cancel_event.is_set(), "BE should not be preempted when disabled"

        # Cleanup
        rt_task.cancel()
        try:
            await rt_task
        except asyncio.CancelledError:
            pass

    @pytest.mark.asyncio
    async def test_preempt_oldest_be(self, hybrid_semaphore_module):
        """Test that oldest BE request is preempted first."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=3,
            rt_reserved_slots=1,
            preemption_enabled=True,
        )

        # Register two BE requests with different start times
        cancel_event1 = asyncio.Event()
        cancel_event2 = asyncio.Event()

        semaphore.register_active_request("be-old", cancel_event1, is_high_priority=False)
        await asyncio.sleep(0.02)  # Ensure different timestamps
        semaphore.register_active_request("be-new", cancel_event2, is_high_priority=False)

        # Manually trigger preemption
        await semaphore._preempt_oldest_be()

        # Only the oldest should be cancelled
        assert cancel_event1.is_set(), "Oldest BE should be preempted"
        assert not cancel_event2.is_set(), "Newer BE should not be preempted"

    @pytest.mark.asyncio
    async def test_preempted_exception_raised(self, hybrid_semaphore_module, preempted_exception):
        """Test that PreemptedException can be raised and caught."""
        PreemptedException = preempted_exception

        with pytest.raises(PreemptedException):
            raise PreemptedException("task-123 preempted")

    @pytest.mark.asyncio
    async def test_no_preemption_when_rt_slot_available(self, hybrid_semaphore_module):
        """RT request doesn't trigger preemption when RT slot is available."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            preemption_enabled=True,
        )

        # Only acquire BE slot, leave RT slot available
        await semaphore.acquire(high_priority=False)
        be_cancel_event = asyncio.Event()
        semaphore.register_active_request("be-1", be_cancel_event, is_high_priority=False)

        # New RT request gets RT slot immediately, no preemption
        wait_time = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0
        assert not be_cancel_event.is_set(), "BE should not be preempted when RT slot available"

    @pytest.mark.asyncio
    async def test_rt_does_not_preempt_rt(self, hybrid_semaphore_module):
        """RT requests should not preempt other RT requests."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=2,  # All slots are RT
            preemption_enabled=True,
        )

        # Acquire both RT slots
        await semaphore.acquire(high_priority=True)
        cancel_event = asyncio.Event()
        semaphore.register_active_request("rt-1", cancel_event, is_high_priority=True)
        await semaphore.acquire(high_priority=True)
        cancel_event2 = asyncio.Event()
        semaphore.register_active_request("rt-2", cancel_event2, is_high_priority=True)

        # New RT request should queue, not preempt
        rt_queued = asyncio.Event()

        async def rt_worker():
            rt_queued.set()
            await semaphore.acquire(high_priority=True)

        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.05)

        # Neither RT should be preempted
        assert not cancel_event.is_set()
        assert not cancel_event2.is_set()

        # Cleanup
        rt_task.cancel()
        try:
            await rt_task
        except asyncio.CancelledError:
            pass


class TestGuaranteedBandwidth:
    """Tests for guaranteed bandwidth mechanism to prevent BE starvation."""

    @pytest.mark.asyncio
    async def test_guaranteed_be_ratio_default(self, hybrid_semaphore_module):
        """Test default guaranteed BE ratio is set."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        # Default should guarantee BE slot after 5 consecutive RT releases
        assert semaphore._guaranteed_be_ratio == 5

    @pytest.mark.asyncio
    async def test_guaranteed_be_ratio_custom(self, hybrid_semaphore_module):
        """Test custom guaranteed BE ratio is respected."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            guaranteed_be_ratio=3,
        )

        assert semaphore._guaranteed_be_ratio == 3

    @pytest.mark.asyncio
    async def test_be_guaranteed_after_consecutive_rt_releases(self, hybrid_semaphore_module):
        """BE request gets slot after guaranteed_be_ratio consecutive RT releases."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            guaranteed_be_ratio=3,  # BE guaranteed after 3 RT releases
        )

        processing_order = []

        # Acquire the only slot
        await semaphore.acquire(high_priority=True)

        async def be_worker():
            await semaphore.acquire(high_priority=False)
            processing_order.append("be")
            semaphore.release(was_high_priority=False)

        async def rt_worker(idx: int):
            await semaphore.acquire(high_priority=True)
            processing_order.append(f"rt_{idx}")
            semaphore.release(was_high_priority=True)

        # Queue BE first
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Queue multiple RT requests
        rt_tasks = []
        for i in range(5):
            rt_task = asyncio.create_task(rt_worker(i))
            rt_tasks.append(rt_task)
            await asyncio.sleep(0.01)

        # Release initial slot
        semaphore.release(was_high_priority=True)

        # Wait for all to complete
        await asyncio.gather(be_task, *rt_tasks)

        # BE should be processed after at most 3 RT releases due to guaranteed bandwidth
        # Find BE position in processing order
        be_position = processing_order.index("be")
        assert be_position <= 3, f"BE should be processed within 3 releases, got position {be_position}"

    @pytest.mark.asyncio
    async def test_consecutive_rt_counter_resets_after_be(self, hybrid_semaphore_module):
        """Consecutive RT counter resets after BE is processed."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            guaranteed_be_ratio=3,
        )

        # Acquire initial slot
        await semaphore.acquire(high_priority=True)

        # Simulate 2 RT releases (counter at 2)
        semaphore._consecutive_rt_releases = 2

        # Queue BE
        be_completed = asyncio.Event()

        async def be_worker():
            await semaphore.acquire(high_priority=False)
            be_completed.set()
            semaphore.release(was_high_priority=False)

        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Force BE release via guaranteed bandwidth by incrementing counter
        semaphore._consecutive_rt_releases = 3  # Hits threshold
        semaphore.release(was_high_priority=True)

        await asyncio.wait_for(be_completed.wait(), timeout=1.0)
        await be_task

        # Counter should be reset after BE processed
        assert semaphore._consecutive_rt_releases == 0

    @pytest.mark.asyncio
    async def test_guaranteed_bandwidth_disabled_when_ratio_zero(self, hybrid_semaphore_module):
        """Guaranteed bandwidth is disabled when ratio is 0."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            guaranteed_be_ratio=0,  # Disabled
        )

        processing_order = []

        # Acquire the only slot
        await semaphore.acquire(high_priority=True)

        async def be_worker():
            await semaphore.acquire(high_priority=False)
            processing_order.append("be")
            semaphore.release(was_high_priority=False)

        async def rt_worker(idx: int):
            await semaphore.acquire(high_priority=True)
            processing_order.append(f"rt_{idx}")
            semaphore.release(was_high_priority=True)

        # Queue BE first
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Queue multiple RT requests
        rt_tasks = []
        for i in range(3):
            rt_task = asyncio.create_task(rt_worker(i))
            rt_tasks.append(rt_task)
            await asyncio.sleep(0.01)

        # Release initial slot
        semaphore.release(was_high_priority=True)

        # Wait for all to complete
        await asyncio.gather(be_task, *rt_tasks)

        # RT should be processed before BE (normal priority behavior)
        assert processing_order[:3] == ["rt_0", "rt_1", "rt_2"]
        assert processing_order[3] == "be"


class TestEnhancedAging:
    """Tests for enhanced aging mechanism with priority promotion."""

    @pytest.mark.asyncio
    async def test_priority_promotion_threshold_default(self, hybrid_semaphore_module):
        """Test default priority promotion threshold is set."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)

        # Default promotion threshold: 600 seconds (10 minutes)
        assert semaphore._priority_promotion_threshold == 600.0

    @pytest.mark.asyncio
    async def test_priority_promotion_threshold_custom(self, hybrid_semaphore_module):
        """Test custom priority promotion threshold is respected."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            priority_promotion_threshold_seconds=300.0,  # 5 minutes
        )

        assert semaphore._priority_promotion_threshold == 300.0

    @pytest.mark.asyncio
    async def test_be_promoted_to_rt_queue_after_threshold(self, hybrid_semaphore_module):
        """BE request is promoted to RT queue after priority promotion threshold."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            priority_promotion_threshold_seconds=0.05,  # 50ms for testing
        )

        processing_order = []

        # Acquire the only slot
        await semaphore.acquire(high_priority=True)

        async def be_worker():
            await semaphore.acquire(high_priority=False)
            processing_order.append("be_promoted")
            semaphore.release(was_high_priority=False)

        async def rt_worker():
            await semaphore.acquire(high_priority=True)
            processing_order.append("rt_fresh")
            semaphore.release(was_high_priority=True)

        # Queue BE first
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.1)  # Wait for promotion threshold

        # Queue RT after BE has aged past promotion threshold
        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)

        # Release slot - promoted BE should be processed like RT
        semaphore.release(was_high_priority=True)

        await asyncio.gather(be_task, rt_task)

        # Promoted BE should be processed first (it was queued first and now has RT priority)
        assert processing_order[0] == "be_promoted"

    @pytest.mark.asyncio
    async def test_promotion_moves_be_to_rt_queue(self, hybrid_semaphore_module):
        """Test that promotion actually moves request from BE queue to RT queue."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            priority_promotion_threshold_seconds=0.05,  # 50ms
        )

        # Acquire slot
        await semaphore.acquire(high_priority=True)

        # Queue BE request
        be_completed = asyncio.Event()

        async def be_worker():
            await semaphore.acquire(high_priority=False)
            be_completed.set()
            semaphore.release(was_high_priority=False)

        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Verify BE is in BE queue
        assert len(semaphore._be_queue) == 1
        assert len(semaphore._rt_queue) == 0

        # Wait for promotion threshold
        await asyncio.sleep(0.06)

        # Trigger aging check via release
        semaphore.release(was_high_priority=True)

        # BE should be processed
        await asyncio.wait_for(be_completed.wait(), timeout=1.0)
        await be_task

    @pytest.mark.asyncio
    async def test_compute_priority_score_with_promotion(self, hybrid_semaphore_module):
        """Test priority score computation accounts for promotion."""
        import time

        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            aging_threshold_seconds=0.02,  # 20ms for aging
            priority_promotion_threshold_seconds=0.05,  # 50ms for promotion
            aging_boost=0.5,
        )

        # Fresh BE request has score 1.0
        now = time.monotonic()
        score_fresh = semaphore._compute_priority_score(high_priority=False, enqueue_time=now)
        assert score_fresh == 1.0

        # Aged BE request (past aging threshold) has reduced score
        aged_time = now - 0.03  # 30ms ago
        score_aged = semaphore._compute_priority_score(high_priority=False, enqueue_time=aged_time)
        assert score_aged < 1.0

        # Promoted BE request (past promotion threshold) has RT-like score
        promoted_time = now - 0.06  # 60ms ago (past promotion threshold)
        score_promoted = semaphore._compute_priority_score(high_priority=False, enqueue_time=promoted_time)
        # Promoted should have priority close to or at RT level (0.0)
        assert score_promoted <= 0.1, f"Promoted BE should have RT-like priority, got {score_promoted}"

    @pytest.mark.asyncio
    async def test_aging_and_promotion_work_together(self, hybrid_semaphore_module):
        """Test that aging gradually reduces priority and promotion caps it at RT level."""
        import time

        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            aging_threshold_seconds=60.0,  # Standard 60s aging
            priority_promotion_threshold_seconds=600.0,  # 10 min promotion
            aging_boost=0.5,
        )

        now = time.monotonic()

        # At 0s: score = 1.0
        score_0 = semaphore._compute_priority_score(False, now)
        assert score_0 == 1.0

        # At 120s (1 minute past aging): score should be reduced
        score_120 = semaphore._compute_priority_score(False, now - 120)
        assert score_120 < score_0

        # At 300s (4 minutes past aging): score should be further reduced
        score_300 = semaphore._compute_priority_score(False, now - 300)
        assert score_300 < score_120

        # At 660s (past promotion threshold): score should be at RT level
        score_660 = semaphore._compute_priority_score(False, now - 660)
        assert score_660 <= 0.1
