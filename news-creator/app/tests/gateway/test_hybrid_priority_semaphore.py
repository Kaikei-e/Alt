"""Tests for Hybrid Priority Semaphore implementation.

Tests the RT/BE scheduling, reserved slots, aging mechanism, and LIFO mode.
"""

import asyncio
import logging
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
        wait_time, _sid = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0
        assert semaphore._rt_available == 0

    @pytest.mark.asyncio
    async def test_be_acquire_immediate_when_slot_available(self, hybrid_semaphore_module):
        """Low priority request acquires BE slot immediately when available."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Low priority should acquire BE slot immediately
        wait_time, _sid = await semaphore.acquire(high_priority=False)
        assert wait_time == 0.0
        assert semaphore._be_available == 0

    @pytest.mark.asyncio
    async def test_rt_slot_reserved_for_high_priority(self, hybrid_semaphore_module):
        """RT slot is reserved for high priority even when BE slots are full."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Acquire BE slot with low priority
        _, _sid = await semaphore.acquire(high_priority=False)

        # RT slot should still be available for high priority
        wait_time, _sid = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0

    @pytest.mark.asyncio
    async def test_low_priority_queues_when_slot_held(self, hybrid_semaphore_module):
        """Low priority queues when RT slot is held, high priority gets next slot."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        # 2 total slots, 1 RT reserved = 1 BE slot
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        processing_order = []

        # Hold the RT slot first
        _, _sid = await semaphore.acquire(high_priority=True)

        async def low_priority_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append("low")
            semaphore.release(slot_id=_sid, was_high_priority=False)

        async def high_priority_worker():
            await asyncio.sleep(0.02)  # Delay to let low priority queue first
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append("high")
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Start both workers - low priority will get BE slot immediately
        # since we have 1 BE slot available
        low_task = asyncio.create_task(low_priority_worker())
        await asyncio.sleep(0.01)
        high_task = asyncio.create_task(high_priority_worker())
        await asyncio.sleep(0.01)

        # Release RT slot - high priority should get it
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def low_priority_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append("low")
            semaphore.release(slot_id=_sid, was_high_priority=False)

        async def high_priority_worker():
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append("high")
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Queue low priority first, then high priority
        low_task = asyncio.create_task(low_priority_worker())
        await asyncio.sleep(0.01)
        high_task = asyncio.create_task(high_priority_worker())
        await asyncio.sleep(0.01)

        # Release the slot
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def rt_worker(task_id: int):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(f"rt_{task_id}")
            await asyncio.sleep(0.01)
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Queue RT tasks in order
        tasks = []
        for i in range(1, 4):
            task = asyncio.create_task(rt_worker(i))
            tasks.append(task)
            await asyncio.sleep(0.01)

        # Release the slot
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append("be")
            semaphore.release(slot_id=_sid, was_high_priority=False)

        async def rt_worker():
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append("rt")
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Queue BE worker first
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.1)  # Wait for aging to kick in

        # Queue RT worker after BE has aged
        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)

        # Release - aged BE may be prioritized over fresh RT
        # This tests that aging is applied; actual order depends on timing
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)
        assert semaphore._rt_available == 0

        # Release RT slot
        semaphore.release(slot_id=_sid, was_high_priority=True)
        assert semaphore._rt_available == 1

    @pytest.mark.asyncio
    async def test_release_returns_be_slot(self, hybrid_semaphore_module):
        """Release returns BE slot to BE pool when no waiters."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        # Acquire BE slot
        _, _sid = await semaphore.acquire(high_priority=False)
        assert semaphore._be_available == 0

        # Release BE slot
        semaphore.release(slot_id=_sid, was_high_priority=False)
        assert semaphore._be_available == 1

    @pytest.mark.asyncio
    async def test_release_wakes_rt_waiter(self, hybrid_semaphore_module):
        """Release wakes up RT waiter when RT queue is not empty."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        # Acquire the slot
        _, _sid = await semaphore.acquire(high_priority=True)

        rt_completed = asyncio.Event()

        async def rt_worker():
            _, _sid = await semaphore.acquire(high_priority=True)
            rt_completed.set()
            semaphore.release(slot_id=_sid, was_high_priority=True)

        task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)  # Ensure it's queued

        # Release - should wake RT waiter
        semaphore.release(slot_id=_sid, was_high_priority=True)

        await asyncio.wait_for(rt_completed.wait(), timeout=1.0)
        await task


class TestLastWaitTime:
    """Tests for wait time tracking."""

    @pytest.mark.asyncio
    async def test_last_wait_time_immediate_acquire(self, hybrid_semaphore_module):
        """Test last_wait_time is 0.0 for immediate acquire."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)

        _, _sid = await semaphore.acquire(high_priority=True)
        assert semaphore.last_wait_time == 0.0

    @pytest.mark.asyncio
    async def test_last_wait_time_tracks_queue_wait(self, hybrid_semaphore_module):
        """Test last_wait_time tracks actual queue wait time."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        # Acquire the slot
        _, _sid = await semaphore.acquire(high_priority=True)

        wait_time_recorded = None

        async def worker():
            nonlocal wait_time_recorded
            wait_time, _sid = await semaphore.acquire(high_priority=True)
            wait_time_recorded = wait_time
            semaphore.release(slot_id=_sid, was_high_priority=True)

        task = asyncio.create_task(worker())
        await asyncio.sleep(0.05)  # Wait 50ms before releasing

        semaphore.release(slot_id=_sid, was_high_priority=True)
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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def worker(name: str, high_priority: bool):
            try:
                _, _sid = await semaphore.acquire(high_priority=high_priority)
                processing_order.append(name)
                semaphore.release(slot_id=_sid, was_high_priority=high_priority)
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
        semaphore.release(slot_id=_sid, was_high_priority=True)
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
        wait_time, _sid = await semaphore.acquire(high_priority=True)
        assert wait_time == 0.0

    @pytest.mark.asyncio
    async def test_concurrent_acquires(self, hybrid_semaphore_module):
        """Test multiple concurrent acquires don't cause race conditions."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        completed = []

        async def worker(worker_id: int, high_priority: bool):
            _, _sid = await semaphore.acquire(high_priority=high_priority)
            await asyncio.sleep(0.01)
            completed.append(worker_id)
            semaphore.release(slot_id=_sid, was_high_priority=high_priority)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        # Acquire BE slot and register as active
        _, _sid = await semaphore.acquire(high_priority=False)
        be_cancel_event = asyncio.Event()
        semaphore.register_active_request("be-1", be_cancel_event, is_high_priority=False)

        # New RT request should trigger preemption
        rt_acquired = asyncio.Event()

        async def rt_worker():
            _, _sid = await semaphore.acquire(high_priority=True)
            rt_acquired.set()

        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.05)

        # BE cancel event should be set
        assert be_cancel_event.is_set(), "BE cancel event should be triggered for preemption"

        # Simulate BE completion and slot release
        semaphore.unregister_active_request("be-1")
        semaphore.release(slot_id=_sid, was_high_priority=False)

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
        _, _sid = await semaphore.acquire(high_priority=True)
        _, _sid = await semaphore.acquire(high_priority=False)
        be_cancel_event = asyncio.Event()
        semaphore.register_active_request("be-1", be_cancel_event, is_high_priority=False)

        # New RT request queues but does not preempt
        rt_queued = asyncio.Event()

        async def rt_worker():
            rt_queued.set()
            _, _sid = await semaphore.acquire(high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=False)
        be_cancel_event = asyncio.Event()
        semaphore.register_active_request("be-1", be_cancel_event, is_high_priority=False)

        # New RT request gets RT slot immediately, no preemption
        wait_time, _sid = await semaphore.acquire(high_priority=True)
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
        _, _sid = await semaphore.acquire(high_priority=True)
        cancel_event = asyncio.Event()
        semaphore.register_active_request("rt-1", cancel_event, is_high_priority=True)
        _, _sid = await semaphore.acquire(high_priority=True)
        cancel_event2 = asyncio.Event()
        semaphore.register_active_request("rt-2", cancel_event2, is_high_priority=True)

        # New RT request should queue, not preempt
        rt_queued = asyncio.Event()

        async def rt_worker():
            rt_queued.set()
            _, _sid = await semaphore.acquire(high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append("be")
            semaphore.release(slot_id=_sid, was_high_priority=False)

        async def rt_worker(idx: int):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(f"rt_{idx}")
            semaphore.release(slot_id=_sid, was_high_priority=True)

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
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        # Simulate 2 RT releases (counter at 2)
        semaphore._consecutive_rt_releases = 2

        # Queue BE
        be_completed = asyncio.Event()

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            be_completed.set()
            semaphore.release(slot_id=_sid, was_high_priority=False)

        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Force BE release via guaranteed bandwidth by incrementing counter
        semaphore._consecutive_rt_releases = 3  # Hits threshold
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append("be")
            semaphore.release(slot_id=_sid, was_high_priority=False)

        async def rt_worker(idx: int):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(f"rt_{idx}")
            semaphore.release(slot_id=_sid, was_high_priority=True)

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
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append("be_promoted")
            semaphore.release(slot_id=_sid, was_high_priority=False)

        async def rt_worker():
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append("rt_fresh")
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Queue BE first
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.1)  # Wait for promotion threshold

        # Queue RT after BE has aged past promotion threshold
        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)

        # Release slot - promoted BE should be processed like RT
        semaphore.release(slot_id=_sid, was_high_priority=True)

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
        _, _sid = await semaphore.acquire(high_priority=True)

        # Queue BE request
        be_completed = asyncio.Event()

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            be_completed.set()
            semaphore.release(slot_id=_sid, was_high_priority=False)

        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Verify BE is in BE queue
        assert len(semaphore._be_queue) == 1
        assert len(semaphore._rt_queue) == 0

        # Wait for promotion threshold
        await asyncio.sleep(0.06)

        # Trigger aging check via release
        semaphore.release(slot_id=_sid, was_high_priority=True)

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


class TestQueueDepthLimit:
    """Tests for queue depth limiting and QueueFullError."""

    @pytest.fixture
    def queue_full_error(self):
        """Import QueueFullError for testing."""
        from news_creator.gateway.hybrid_priority_semaphore import QueueFullError
        return QueueFullError

    @pytest.mark.asyncio
    async def test_acquire_raises_queue_full_when_depth_exceeded(self, hybrid_semaphore_module, queue_full_error):
        """Test that acquire raises QueueFullError when max_queue_depth is exceeded."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        QueueFullError = queue_full_error

        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            max_queue_depth=2,
        )

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        # Queue 2 requests (fills max_queue_depth)
        async def queued_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            semaphore.release(slot_id=_sid, was_high_priority=False)

        task1 = asyncio.create_task(queued_worker())
        await asyncio.sleep(0.01)
        task2 = asyncio.create_task(queued_worker())
        await asyncio.sleep(0.01)

        # Third request should raise QueueFullError
        with pytest.raises(QueueFullError):
            _, _sid = await semaphore.acquire(high_priority=False)

        # Cleanup
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.gather(task1, task2)

    @pytest.mark.asyncio
    async def test_acquire_succeeds_when_under_depth_limit(self, hybrid_semaphore_module):
        """Test that acquire succeeds when under max_queue_depth."""
        HybridPrioritySemaphore = hybrid_semaphore_module

        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            max_queue_depth=5,
        )

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        # Queue 1 request (under depth limit of 5)
        completed = asyncio.Event()

        async def queued_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            completed.set()
            semaphore.release(slot_id=_sid, was_high_priority=False)

        task = asyncio.create_task(queued_worker())
        await asyncio.sleep(0.01)

        # Should not raise
        assert len(semaphore._rt_queue) + len(semaphore._be_queue) <= 5

        # Cleanup
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.wait_for(completed.wait(), timeout=1.0)
        await task

    @pytest.mark.asyncio
    async def test_queue_full_error_includes_rt_and_be_queues(self, hybrid_semaphore_module, queue_full_error):
        """Test that QueueFullError considers both RT and BE queues for depth check."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        QueueFullError = queue_full_error

        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            max_queue_depth=2,
        )

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        # Queue 1 RT and 1 BE (total = 2 = max_queue_depth)
        async def rt_worker():
            _, _sid = await semaphore.acquire(high_priority=True)
            semaphore.release(slot_id=_sid, was_high_priority=True)

        async def be_worker():
            _, _sid = await semaphore.acquire(high_priority=False)
            semaphore.release(slot_id=_sid, was_high_priority=False)

        rt_task = asyncio.create_task(rt_worker())
        await asyncio.sleep(0.01)
        be_task = asyncio.create_task(be_worker())
        await asyncio.sleep(0.01)

        # Third request (either priority) should raise QueueFullError
        with pytest.raises(QueueFullError):
            _, _sid = await semaphore.acquire(high_priority=True)

        # Cleanup
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.gather(rt_task, be_task)

    @pytest.mark.asyncio
    async def test_default_max_queue_depth_is_zero_unlimited(self, hybrid_semaphore_module):
        """Test that default max_queue_depth=0 means unlimited."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)
        assert semaphore._max_queue_depth == 0

    @pytest.mark.asyncio
    async def test_queue_status_returns_correct_state(self, hybrid_semaphore_module):
        """Test that queue_status() returns correct queue state."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            max_queue_depth=20,
        )

        status = semaphore.queue_status()
        assert status["rt_queue"] == 0
        assert status["be_queue"] == 0
        assert status["total_slots"] == 2
        assert status["available_slots"] == 2
        assert status["accepting"] is True
        assert status["max_queue_depth"] == 20

        # Acquire one slot
        _, _sid = await semaphore.acquire(high_priority=True)
        status = semaphore.queue_status()
        assert status["available_slots"] == 1
        assert status["accepting"] is True


class TestLIFOSchedulingMode:
    """Tests for LIFO (Last-In-First-Out) RT queue scheduling mode.

    LIFO mode allows the most recently requested summary to be processed first,
    optimizing for the user's current view in swipe-feed UIs.
    """

    @pytest.mark.asyncio
    async def test_lifo_rt_queue_processes_newest_first(self, hybrid_semaphore_module):
        """3 RT requests queued; with LIFO, verify C→B→A processing order."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            rt_scheduling_mode="lifo",
        )

        processing_order = []

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        async def rt_worker(name: str):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(name)
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Queue A, B, C in order with small delays to ensure distinct enqueue times
        task_a = asyncio.create_task(rt_worker("A"))
        await asyncio.sleep(0.01)
        task_b = asyncio.create_task(rt_worker("B"))
        await asyncio.sleep(0.01)
        task_c = asyncio.create_task(rt_worker("C"))
        await asyncio.sleep(0.01)

        # Release slot - LIFO should process C first (newest)
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.gather(task_a, task_b, task_c)

        assert processing_order == ["C", "B", "A"]

    @pytest.mark.asyncio
    async def test_fifo_rt_queue_processes_oldest_first(self, hybrid_semaphore_module):
        """Regression: with FIFO (default), verify A→B→C processing order."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            rt_scheduling_mode="fifo",
        )

        processing_order = []

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        async def rt_worker(name: str):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(name)
            semaphore.release(slot_id=_sid, was_high_priority=True)

        # Queue A, B, C in order
        task_a = asyncio.create_task(rt_worker("A"))
        await asyncio.sleep(0.01)
        task_b = asyncio.create_task(rt_worker("B"))
        await asyncio.sleep(0.01)
        task_c = asyncio.create_task(rt_worker("C"))
        await asyncio.sleep(0.01)

        # Release slot - FIFO should process A first (oldest)
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.gather(task_a, task_b, task_c)

        assert processing_order == ["A", "B", "C"]

    @pytest.mark.asyncio
    async def test_lifo_does_not_affect_be_queue(self, hybrid_semaphore_module):
        """BE queue remains FIFO regardless of rt_scheduling_mode."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            rt_scheduling_mode="lifo",
        )

        processing_order = []

        # Acquire the BE slot
        _, _sid = await semaphore.acquire(high_priority=False)

        async def be_worker(name: str):
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append(name)
            semaphore.release(slot_id=_sid, was_high_priority=False)

        # Queue BE requests
        task_a = asyncio.create_task(be_worker("A"))
        await asyncio.sleep(0.01)
        task_b = asyncio.create_task(be_worker("B"))
        await asyncio.sleep(0.01)
        task_c = asyncio.create_task(be_worker("C"))
        await asyncio.sleep(0.01)

        # Release - BE should still be FIFO (A→B→C)
        semaphore.release(slot_id=_sid, was_high_priority=False)
        await asyncio.gather(task_a, task_b, task_c)

        assert processing_order == ["A", "B", "C"]

    @pytest.mark.asyncio
    async def test_lifo_with_cancellation(self, hybrid_semaphore_module):
        """Cancel newest RT request in LIFO; verify second-newest processes next."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            rt_scheduling_mode="lifo",
        )

        processing_order = []

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        async def rt_worker(name: str):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(name)
            semaphore.release(slot_id=_sid, was_high_priority=True)

        task_a = asyncio.create_task(rt_worker("A"))
        await asyncio.sleep(0.01)
        task_b = asyncio.create_task(rt_worker("B"))
        await asyncio.sleep(0.01)
        task_c = asyncio.create_task(rt_worker("C"))
        await asyncio.sleep(0.01)

        # Cancel C (newest) before releasing
        task_c.cancel()
        try:
            await task_c
        except asyncio.CancelledError:
            pass

        # Release slot - should process B next (second-newest in LIFO)
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.gather(task_a, task_b, return_exceptions=True)

        assert processing_order == ["B", "A"]

    @pytest.mark.asyncio
    async def test_lifo_with_guaranteed_bandwidth(self, hybrid_semaphore_module):
        """Guaranteed BE bandwidth still works when RT queue is LIFO."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            rt_scheduling_mode="lifo",
            guaranteed_be_ratio=2,  # Force BE after 2 consecutive RT releases
        )

        processing_order = []

        # Acquire the only slot
        _, _sid = await semaphore.acquire(high_priority=True)

        async def rt_worker(name: str):
            _, _sid = await semaphore.acquire(high_priority=True)
            processing_order.append(name)
            semaphore.release(slot_id=_sid, was_high_priority=True)

        async def be_worker(name: str):
            _, _sid = await semaphore.acquire(high_priority=False)
            processing_order.append(name)
            semaphore.release(slot_id=_sid, was_high_priority=False)

        # Queue: RT-A, RT-B, BE-X
        task_rt_a = asyncio.create_task(rt_worker("RT-A"))
        await asyncio.sleep(0.01)
        task_rt_b = asyncio.create_task(rt_worker("RT-B"))
        await asyncio.sleep(0.01)
        task_be = asyncio.create_task(be_worker("BE-X"))
        await asyncio.sleep(0.01)

        # First release (RT) - LIFO processes RT-B first
        semaphore.release(slot_id=_sid, was_high_priority=True)
        await asyncio.sleep(0.05)

        # RT-B processed, releases as RT → consecutive_rt_releases = 2
        # Guaranteed bandwidth triggers: BE-X should be next
        await asyncio.gather(task_rt_a, task_rt_b, task_be)

        # RT-B first (LIFO), then BE-X (guaranteed bandwidth after 2 RT),
        # then RT-A
        assert processing_order[0] == "RT-B"
        assert "BE-X" in processing_order

    @pytest.mark.asyncio
    async def test_default_rt_scheduling_mode_is_fifo(self, hybrid_semaphore_module):
        """Default rt_scheduling_mode should be 'fifo'."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        semaphore = HybridPrioritySemaphore(total_slots=2)

        assert semaphore._rt_scheduling_mode == "fifo"


class TestSlotLeakAfterPreemption:
    """Tests for slot leak after preemption (PM-2026-012 reproduction).

    Reproduces the exact sequence from 2026-03-27 09:35-09:49 UTC logs:
    preemption causes RT slot to migrate into BE pool; per-pool cap in
    release() permanently loses one slot, starving subsequent RT requests.
    """

    @pytest.mark.asyncio
    async def test_invariant_holds_after_preemption_release_chain(
        self, hybrid_semaphore_module
    ):
        """Slot invariant: rt_available + be_available + acquired == total_slots.

        Sequence (mirrors production logs):
        1. RT#1 acquires RT slot
        2. BE#1 acquires BE slot, registers as preemptable
        3. RT#2 arrives → RT slot busy → preempts BE#1
        4. BE#1 detects cancel, releases (was_high_priority=False)
        5. RT#2 wakes from queue
        6. RT#1 releases → wakes a BE waiter
        7. RT#2 releases → wakes another BE waiter
        8. Both BE finish, no more waiters
        After step 8, rt_available + be_available must == total_slots.
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            preemption_enabled=True,
            guaranteed_be_ratio=0,  # disable to isolate the bug
        )

        # Step 1: RT#1 acquires RT slot
        _, rt1_sid = await sem.acquire(high_priority=True)
        assert sem._rt_available == 0

        # Step 2: BE#1 acquires BE slot
        _, be1_sid = await sem.acquire(high_priority=False)
        assert sem._be_available == 0

        # Register BE#1 as preemptable
        cancel_event = asyncio.Event()
        sem.register_active_request("be-1", cancel_event, is_high_priority=False)

        # Step 3: RT#2 arrives — both slots busy, triggers preemption
        rt2_task = asyncio.create_task(sem.acquire(high_priority=True))
        # Let the event loop process the preemption
        await asyncio.sleep(0)

        # BE#1 should have been signalled
        assert cancel_event.is_set(), "BE#1 should be preempted"

        # Step 4: BE#1 detects cancel, releases its slot
        sem.unregister_active_request("be-1")
        sem.release(slot_id=be1_sid, was_high_priority=False)

        # Step 5: RT#2 wakes from queue (may need multiple yields)
        for _ in range(5):
            await asyncio.sleep(0)
        assert rt2_task.done(), "RT#2 should have been woken by BE#1 release"
        _, rt2_sid = rt2_task.result()

        # Queue two BE waiters so RT releases can transfer to them
        be_waiter_1 = asyncio.create_task(sem.acquire(high_priority=False))
        be_waiter_2 = asyncio.create_task(sem.acquire(high_priority=False))
        for _ in range(3):
            await asyncio.sleep(0)

        # Step 6: RT#1 releases → should wake be_waiter_1
        sem.release(slot_id=rt1_sid, was_high_priority=True)
        for _ in range(3):
            await asyncio.sleep(0)
        assert be_waiter_1.done(), "BE waiter 1 should be woken by RT#1 release"
        _, bw1_sid = be_waiter_1.result()

        # Step 7: RT#2 releases → should wake be_waiter_2
        sem.release(slot_id=rt2_sid, was_high_priority=True)
        for _ in range(3):
            await asyncio.sleep(0)
        assert be_waiter_2.done(), "BE waiter 2 should be woken by RT#2 release"
        _, bw2_sid = be_waiter_2.result()

        # Step 8: Both BE finish, release with no waiters
        sem.release(slot_id=bw1_sid, was_high_priority=False)
        sem.release(slot_id=bw2_sid, was_high_priority=False)

        # INVARIANT: total available must equal total_slots
        total_available = sem._rt_available + sem._be_available
        total_acquired = len(sem._acquired_slots)
        assert total_available + total_acquired == 2, (
            f"Slot leak detected: rt_available={sem._rt_available}, "
            f"be_available={sem._be_available}, "
            f"acquired={total_acquired}, "
            f"expected total=2"
        )
        # Specifically: RT pool must have recovered its reserved slot
        assert sem._rt_available == 1, (
            f"RT slot permanently lost: rt_available={sem._rt_available}, "
            f"expected 1"
        )

    @pytest.mark.asyncio
    async def test_rt_not_starved_when_gpu_idle(self, hybrid_semaphore_module):
        """RT request must not wait indefinitely when no BE is active.

        Reproduces the 10-minute idle GPU scenario:
        After preemption chain with wake-ups, rt_available=0, be_available=1.
        A new RT request arrives but cannot acquire because rt_available=0
        and rt_reserved!=0 blocks BE-slot fallback. The RT starves until
        a new BE request happens to arrive and release.
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            preemption_enabled=True,
            guaranteed_be_ratio=0,
        )

        # --- Reproduce the full preemption → wake chain → leak sequence ---

        # RT#1 acquires RT slot, BE#1 acquires BE slot
        _, rt1_sid = await sem.acquire(high_priority=True)
        _, be1_sid = await sem.acquire(high_priority=False)

        cancel_event = asyncio.Event()
        sem.register_active_request("be-1", cancel_event, is_high_priority=False)

        # Queue TWO BE waiters so BOTH RT releases transfer to BE (not pool)
        be_waiter_1 = asyncio.create_task(sem.acquire(high_priority=False))
        be_waiter_2 = asyncio.create_task(sem.acquire(high_priority=False))
        for _ in range(3):
            await asyncio.sleep(0)

        # RT#2 preempts BE#1
        rt2_task = asyncio.create_task(sem.acquire(high_priority=True))
        for _ in range(3):
            await asyncio.sleep(0)
        assert cancel_event.is_set()

        # BE#1 releases → wakes RT#2
        sem.unregister_active_request("be-1")
        sem.release(slot_id=be1_sid, was_high_priority=False)
        for _ in range(5):
            await asyncio.sleep(0)
        assert rt2_task.done()
        _, rt2_sid = rt2_task.result()

        # RT#1 releases → wakes be_waiter_1 (slot migrates RT→BE)
        sem.release(slot_id=rt1_sid, was_high_priority=True)
        for _ in range(3):
            await asyncio.sleep(0)
        assert be_waiter_1.done()
        _, bw1_sid = be_waiter_1.result()

        # RT#2 releases → wakes be_waiter_2 (slot migrates RT→BE AGAIN)
        sem.release(slot_id=rt2_sid, was_high_priority=True)
        for _ in range(3):
            await asyncio.sleep(0)
        assert be_waiter_2.done()
        _, bw2_sid = be_waiter_2.result()

        # Both BE complete, release with no waiters
        sem.release(slot_id=bw1_sid, was_high_priority=False)
        sem.release(slot_id=bw2_sid, was_high_priority=False)

        # --- Now GPU is idle, all slots should be available ---
        rt3_task = asyncio.create_task(sem.acquire(high_priority=True))
        for _ in range(3):
            await asyncio.sleep(0)

        assert rt3_task.done(), (
            f"RT request should acquire immediately when GPU is idle. "
            f"rt_available={sem._rt_available}, be_available={sem._be_available}"
        )

    @pytest.mark.asyncio
    async def test_slot_count_invariant_after_mixed_releases(
        self, hybrid_semaphore_module
    ):
        """Available + acquired slots must always equal total_slots.

        Tests that release() never drops a slot due to per-pool cap,
        regardless of the acquire/release priority sequence.
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=2,
            rt_reserved_slots=1,
            guaranteed_be_ratio=0,
        )

        # Acquire both slots as RT (simulating preemption path)
        _, sid1 = await sem.acquire(high_priority=True)   # RT slot
        _, sid2 = await sem.acquire(high_priority=False)   # BE slot

        # Release both — slot_id tracks home_pool correctly regardless of caller priority
        sem.release(slot_id=sid1, was_high_priority=False)
        sem.release(slot_id=sid2, was_high_priority=True)

        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 2, (
            f"Slot count mismatch: rt={sem._rt_available}, "
            f"be={sem._be_available}, acquired={len(sem._acquired_slots)}, "
            f"total={total}, expected=2"
        )


class TestSingleSlotPreemption:
    """Tests for total_slots=1 (RAG-dedicated single-slot mode).

    When be_slots=0, preemption chains can tag a slot with home_pool="be"
    but no BE pool exists. The slot must be remapped to the RT pool.
    """

    @pytest.mark.asyncio
    async def test_slot_invariant_single_slot_after_preemption(
        self, hybrid_semaphore_module
    ):
        """With total_slots=1, rt_reserved=1, be_slots=0.

        A summarize (LOW) request holds the single RT slot. A chat (HIGH)
        request arrives and preempts it. After the preemption chain completes,
        the single slot must not disappear.
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            preemption_enabled=True,
            guaranteed_be_ratio=0,
        )

        # BE acquires the single slot (falls back to RT slot since be_slots=0)
        _, be_sid = await sem.acquire(high_priority=False)

        # Register as preemptable
        cancel_event = asyncio.Event()
        sem.register_active_request("be-1", cancel_event, is_high_priority=False)

        # RT arrives — triggers preemption
        rt_task = asyncio.create_task(sem.acquire(high_priority=True))
        await asyncio.sleep(0)
        assert cancel_event.is_set(), "BE should be preempted"

        # BE detects cancel, releases
        sem.unregister_active_request("be-1")
        sem.release(slot_id=be_sid, was_high_priority=False)

        # RT wakes from queue
        for _ in range(5):
            await asyncio.sleep(0)
        assert rt_task.done(), "RT should have acquired after preemption"
        _, rt_sid = rt_task.result()

        # RT finishes, releases — no waiters
        sem.release(slot_id=rt_sid, was_high_priority=True)

        # INVARIANT: the single slot must not have disappeared
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1, (
            f"Slot leak: rt_available={sem._rt_available}, "
            f"be_available={sem._be_available}, "
            f"acquired={len(sem._acquired_slots)}, total={total}, expected=1"
        )
        assert sem._rt_available == 1, (
            f"Single RT slot lost: rt_available={sem._rt_available}"
        )

    @pytest.mark.asyncio
    async def test_slot_returns_to_rt_when_be_pool_zero(
        self, hybrid_semaphore_module
    ):
        """When be_slots=0, a slot with home_pool='be' must remap to RT."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            preemption_enabled=False,
            guaranteed_be_ratio=0,
        )
        assert sem._be_slots == 0, "be_slots should be 0 when rt_reserved == total"

        # Acquire the single slot as LOW priority (will get RT slot via fallback)
        _, sid = await sem.acquire(high_priority=False)
        assert len(sem._acquired_slots) == 1

        # Release — regardless of tracked home_pool, it must return to RT (the only pool)
        sem.release(slot_id=sid, was_high_priority=False)

        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1, f"Slot lost: rt={sem._rt_available}, be={sem._be_available}"
        assert sem._rt_available == 1, "Slot must return to RT pool (only existing pool)"


class TestSlotIdOwnershipPropagation:
    """Tests for explicit slot_id ownership propagation (ADR-000604 item 4).

    acquire() must return (wait_time, slot_id) so callers can pass slot_id
    back to release(), eliminating was_high_priority inference.
    """

    @pytest.mark.asyncio
    async def test_acquire_returns_tuple_with_slot_id(self, hybrid_semaphore_module):
        """acquire() must return (wait_time, slot_id) tuple."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        result = await sem.acquire(high_priority=True)
        assert isinstance(result, tuple), (
            f"acquire() should return tuple, got {type(result)}"
        )
        wait_time, slot_id = result
        assert isinstance(wait_time, float)
        assert isinstance(slot_id, int)
        assert slot_id > 0

    @pytest.mark.asyncio
    async def test_release_with_slot_id_returns_to_home_pool(
        self, hybrid_semaphore_module
    ):
        """release(slot_id=X) must return slot to the pool it came from."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(total_slots=2, rt_reserved_slots=1)

        _, slot_id = await sem.acquire(high_priority=True)
        sem.release(slot_id=slot_id)

        # RT slot should be back in RT pool
        assert sem._rt_available == 1

    @pytest.mark.asyncio
    async def test_acquire_different_slots_get_different_ids(
        self, hybrid_semaphore_module
    ):
        """Each acquire() must return a unique slot_id."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(total_slots=3, rt_reserved_slots=1)

        _, id1 = await sem.acquire(high_priority=True)
        _, id2 = await sem.acquire(high_priority=False)
        _, id3 = await sem.acquire(high_priority=False)

        assert len({id1, id2, id3}) == 3, "All slot_ids must be unique"

    @pytest.mark.asyncio
    async def test_preemption_release_chain_with_slot_id(
        self, hybrid_semaphore_module
    ):
        """Reproduce the exact preemption→release chain from production logs.

        Scenario:
        1. BE acquires a slot from BE pool
        2. RT arrives, preempts BE
        3. BE gets a transferred RT slot
        4. Both release with their slot_ids
        5. Slots must return to correct home pools
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1,
            rt_reserved_slots=1,
            preemption_enabled=True,
        )

        # BE acquires the only slot (falls back to RT pool since no BE slots)
        _, be_slot_id = await sem.acquire(high_priority=False)

        # RT arrives — triggers preemption
        rt_task = asyncio.create_task(sem.acquire(high_priority=True))
        await asyncio.sleep(0.05)  # Let preemption happen

        # BE releases its slot (which came from RT pool)
        sem.release(slot_id=be_slot_id)
        await asyncio.sleep(0.05)

        # RT should now have acquired
        assert rt_task.done(), "RT should have acquired after BE release"
        _, rt_slot_id = rt_task.result()

        # Release RT
        sem.release(slot_id=rt_slot_id)

        # Invariant: all slots back
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == sem._total_slots, (
            f"Slot leak: rt={sem._rt_available}, be={sem._be_available}, "
            f"acquired={len(sem._acquired_slots)}"
        )


class TestBeSlotZeroSlotLoss:
    """Tests for be_slots=0 (total_slots=1, rt_reserved=1) slot loss bug.

    When be_slots=0, BE requests fall back to RT slots. The release() method
    uses call_soon_threadsafe to schedule future.set_result(), which can fail
    if the future is cancelled between scheduling and execution, causing the
    slot to be permanently lost.
    """

    @pytest.mark.asyncio
    async def test_be_slots_zero_basic_acquire_release(self, hybrid_semaphore_module):
        """BE acquires RT slot when be_slots=0 and releases back correctly."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        assert sem._be_slots == 0
        assert sem._rt_available == 1

        # BE acquires the RT slot (fallback path)
        wait_time, slot_id = await sem.acquire(high_priority=False)
        assert wait_time == 0.0
        assert sem._rt_available == 0

        # Release: slot should return to RT pool
        sem.release(slot_id=slot_id, was_high_priority=False)
        assert sem._rt_available == 1
        assert sem._be_available == 0

        # Invariant
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1

    @pytest.mark.asyncio
    async def test_be_slots_zero_transfer_to_rt_waiter(self, hybrid_semaphore_module):
        """When BE releases and RT is queued, slot transfers correctly."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # BE acquires the only slot
        _, be_sid = await sem.acquire(high_priority=False)

        # RT queues (no preemption)
        rt_task = asyncio.create_task(sem.acquire(high_priority=True))
        await asyncio.sleep(0.02)
        assert not rt_task.done()

        # BE releases → should wake RT
        sem.release(slot_id=be_sid, was_high_priority=False)
        await asyncio.sleep(0.02)

        assert rt_task.done()
        _, rt_sid = rt_task.result()

        # RT releases → slot back to RT pool
        sem.release(slot_id=rt_sid, was_high_priority=True)

        # Invariant
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1, (
            f"Slot leak: rt={sem._rt_available}, be={sem._be_available}, "
            f"acquired={len(sem._acquired_slots)}"
        )

    @pytest.mark.asyncio
    async def test_be_slots_zero_transfer_to_be_waiter(self, hybrid_semaphore_module):
        """When BE releases and another BE is queued, slot transfers correctly."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # BE-1 acquires the only slot
        _, be1_sid = await sem.acquire(high_priority=False)

        # BE-2 queues
        be2_task = asyncio.create_task(sem.acquire(high_priority=False))
        await asyncio.sleep(0.02)
        assert not be2_task.done()

        # BE-1 releases → should wake BE-2
        sem.release(slot_id=be1_sid, was_high_priority=False)
        await asyncio.sleep(0.02)

        assert be2_task.done()
        _, be2_sid = be2_task.result()

        # BE-2 releases → slot back to RT pool (home_pool=rt)
        sem.release(slot_id=be2_sid, was_high_priority=False)

        # Invariant: slot must return to RT pool since be_slots=0
        assert sem._rt_available == 1
        assert sem._be_available == 0
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1, (
            f"Slot leak: rt={sem._rt_available}, be={sem._be_available}, "
            f"acquired={len(sem._acquired_slots)}"
        )

    @pytest.mark.asyncio
    async def test_be_slots_zero_cancelled_waiter_no_slot_loss(self, hybrid_semaphore_module):
        """If a queued waiter is cancelled, the slot must not be lost.

        This is the key bug: release() uses call_soon_threadsafe to schedule
        future.set_result(). If the future is cancelled before set_result
        executes, the slot vanishes.
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # BE-1 acquires the only slot
        _, be1_sid = await sem.acquire(high_priority=False)

        # BE-2 queues
        be2_task = asyncio.create_task(sem.acquire(high_priority=False))
        await asyncio.sleep(0.02)
        assert not be2_task.done()

        # Cancel BE-2 before it gets woken up
        be2_task.cancel()
        await asyncio.sleep(0.02)

        # BE-1 releases — the release should detect that the waiter was
        # cancelled and return the slot to the pool instead
        sem.release(slot_id=be1_sid, was_high_priority=False)
        await asyncio.sleep(0.02)

        # Invariant: slot must NOT be lost
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1, (
            f"Slot lost after cancelled waiter: rt={sem._rt_available}, "
            f"be={sem._be_available}, acquired={len(sem._acquired_slots)}"
        )

    @pytest.mark.asyncio
    async def test_be_slots_zero_rapid_acquire_release_invariant(self, hybrid_semaphore_module):
        """Rapid acquire/release cycles must not leak slots."""
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(total_slots=1, rt_reserved_slots=1)

        for _ in range(20):
            _, sid = await sem.acquire(high_priority=False)
            sem.release(slot_id=sid, was_high_priority=False)

            total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
            assert total == 1, f"Invariant broken on iteration: rt={sem._rt_available}, be={sem._be_available}"

        for _ in range(20):
            _, sid = await sem.acquire(high_priority=True)
            sem.release(slot_id=sid, was_high_priority=True)

            total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
            assert total == 1


class TestCancelledWaiterSlotRecovery:
    """Tests for slot recovery when a waiter's task is cancelled AFTER
    receiving a slot via set_result().

    Bug: When release() transfers a slot to a queued waiter via
    _try_wake_waiter → set_result(home_pool), and the waiter's asyncio
    task is then cancelled before _track_acquire() runs, the slot is
    permanently lost — not in _acquired_slots, not in pool counters.
    """

    @pytest.mark.asyncio
    async def test_cancelled_task_after_slot_transfer_recovers_slot(
        self, hybrid_semaphore_module
    ):
        """Slot must be recovered when a waiter's task is cancelled after
        receiving the slot via set_result().

        Scenario (be_slots=0):
        1. BE-1 acquires the only slot (RT fallback)
        2. BE-2 queues
        3. BE-1 releases → slot transferred to BE-2 via set_result()
        4. BE-2's task is cancelled before _track_acquire()
        5. Invariant: slot must be returned to RT pool
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # BE-1 acquires the only slot
        _, be1_sid = await sem.acquire(high_priority=False)

        # BE-2 queues (will block waiting for slot)
        be2_task = asyncio.create_task(sem.acquire(high_priority=False))
        await asyncio.sleep(0.02)
        assert not be2_task.done()

        # BE-1 releases → slot transferred to BE-2 via set_result()
        sem.release(slot_id=be1_sid, was_high_priority=False)

        # Cancel BE-2's task immediately — before _track_acquire() runs
        be2_task.cancel()
        try:
            await be2_task
        except asyncio.CancelledError:
            pass

        # Allow event loop to process
        await asyncio.sleep(0.02)

        # Invariant: slot must NOT be lost
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1, (
            f"Slot lost after cancelled waiter: rt={sem._rt_available}, "
            f"be={sem._be_available}, acquired={len(sem._acquired_slots)}"
        )

    @pytest.mark.asyncio
    async def test_cancelled_task_transfers_to_next_waiter(
        self, hybrid_semaphore_module
    ):
        """When a waiter's task is cancelled after slot transfer, the slot
        must be forwarded to the next waiter in the queue.

        Scenario (be_slots=0):
        1. BE-1 acquires
        2. BE-2 queues, BE-3 queues
        3. BE-1 releases → slot goes to BE-2
        4. BE-2's task is cancelled → slot must go to BE-3
        5. BE-3 should get the slot
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # BE-1 acquires
        _, be1_sid = await sem.acquire(high_priority=False)

        # BE-2 and BE-3 queue
        be2_task = asyncio.create_task(sem.acquire(high_priority=False))
        await asyncio.sleep(0.02)
        be3_task = asyncio.create_task(sem.acquire(high_priority=False))
        await asyncio.sleep(0.02)
        assert not be2_task.done()
        assert not be3_task.done()

        # BE-1 releases → slot transferred to BE-2
        sem.release(slot_id=be1_sid, was_high_priority=False)

        # Cancel BE-2 before it processes the slot
        be2_task.cancel()
        try:
            await be2_task
        except asyncio.CancelledError:
            pass

        await asyncio.sleep(0.05)

        # BE-3 should have received the recovered slot
        assert be3_task.done(), "BE-3 should have received the slot"
        _, be3_sid = be3_task.result()

        # Clean up
        sem.release(slot_id=be3_sid, was_high_priority=False)

        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1

    @pytest.mark.asyncio
    async def test_rt_cancelled_after_transfer_recovers_to_rt_pool(
        self, hybrid_semaphore_module
    ):
        """When an RT waiter's task is cancelled after slot transfer,
        the slot must return to the RT pool (be_slots=0 config).
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # RT-1 acquires
        _, rt1_sid = await sem.acquire(high_priority=True)

        # RT-2 queues (preemption disabled, so it just waits)
        rt2_task = asyncio.create_task(sem.acquire(high_priority=True))
        await asyncio.sleep(0.02)
        assert not rt2_task.done()

        # RT-1 releases → slot transferred to RT-2
        sem.release(slot_id=rt1_sid, was_high_priority=True)

        # Cancel RT-2
        rt2_task.cancel()
        try:
            await rt2_task
        except asyncio.CancelledError:
            pass

        await asyncio.sleep(0.02)

        # Slot must return to RT pool
        assert sem._rt_available == 1, (
            f"RT slot not recovered: rt={sem._rt_available}, "
            f"be={sem._be_available}, acquired={len(sem._acquired_slots)}"
        )
        total = sem._rt_available + sem._be_available + len(sem._acquired_slots)
        assert total == 1

    @pytest.mark.asyncio
    async def test_invariant_no_false_positive_during_transfer(
        self, hybrid_semaphore_module, caplog
    ):
        """release() must not log SLOT INVARIANT VIOLATION during normal
        slot transfer to a queued waiter (false positive).
        """
        HybridPrioritySemaphore = hybrid_semaphore_module
        sem = HybridPrioritySemaphore(
            total_slots=1, rt_reserved_slots=1, preemption_enabled=False,
        )

        # BE-1 acquires
        _, be1_sid = await sem.acquire(high_priority=False)

        # BE-2 queues
        be2_task = asyncio.create_task(sem.acquire(high_priority=False))
        await asyncio.sleep(0.02)

        # BE-1 releases → slot transferred to BE-2
        with caplog.at_level(logging.ERROR, logger="news_creator.gateway.hybrid_priority_semaphore"):
            sem.release(slot_id=be1_sid, was_high_priority=False)

        # No SLOT INVARIANT VIOLATION should be logged
        violation_msgs = [
            r for r in caplog.records
            if "SLOT INVARIANT VIOLATION" in r.message
        ]
        assert len(violation_msgs) == 0, (
            f"False positive invariant violation during normal transfer: "
            f"{[r.message for r in violation_msgs]}"
        )

        # Clean up
        await asyncio.sleep(0.02)
        assert be2_task.done()
        _, be2_sid = be2_task.result()
        sem.release(slot_id=be2_sid, was_high_priority=False)
