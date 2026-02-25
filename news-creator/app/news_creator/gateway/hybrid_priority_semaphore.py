"""
Hybrid Real-Time / Best-Effort Priority Semaphore.

Based on best practices:
- https://arxiv.org/html/2504.09590v1 (Hybrid RT/BE Scheduling)
- https://huggingface.co/blog/tngtech/llm-performance-request-queueing
- https://arxiv.org/html/2503.09304v1 (QLLM Preemption)

This semaphore implementation addresses TTFT latency issues caused by
low-priority batch requests blocking high-priority streaming requests.

Features:
- Reserved slots for real-time (streaming) requests
- Aging mechanism to prevent starvation of low-priority requests
- Application-level preemption for RT priority
"""

import asyncio
import heapq
import logging
import time
from dataclasses import dataclass, field
from typing import Dict, Optional

logger = logging.getLogger(__name__)


class PreemptedException(Exception):
    """Raised when a BE request is preempted for RT priority."""

    pass


class QueueFullError(Exception):
    """Raised when the queue depth limit is exceeded."""

    pass


@dataclass
class AcquiredSlot:
    """Tracks an acquired semaphore slot for leak detection."""

    slot_id: int
    acquired_at: float  # monotonic time
    is_high_priority: bool
    context: str = ""  # optional description for debugging


@dataclass
class CancellableRequest:
    """Tracks an active request that can be preempted."""

    task_id: str
    cancel_event: asyncio.Event
    start_time: float
    is_high_priority: bool


@dataclass(order=True)
class QueuedRequest:
    """Request with aging-aware priority.

    Sorting order: priority_score (ascending), then enqueue_time (ascending).
    This ensures FIFO ordering among requests with the same priority score.

    For LIFO mode, enqueue_time is negated so the newest request (largest
    real timestamp → smallest negated value) pops first from the min-heap.
    """

    priority_score: float  # Lower = higher priority
    enqueue_time: float  # FIFO: start_time, LIFO: -start_time
    future: asyncio.Future = field(compare=False)
    is_high_priority: bool = field(compare=False)


class HybridPrioritySemaphore:
    """
    Hybrid RT/BE semaphore with reserved slots, aging, and guaranteed bandwidth.

    Features:
    - Reserved slot for real-time (streaming) requests
    - Aging mechanism to prevent starvation of low-priority requests
    - Priority promotion: BE requests promoted to RT after threshold
    - Guaranteed bandwidth: BE requests guaranteed processing every N RT releases
    - Fair scheduling within priority levels

    Args:
        total_slots: Total concurrent slots (OLLAMA_NUM_PARALLEL)
        rt_reserved_slots: Slots reserved for real-time requests (default: 1)
        aging_threshold_seconds: Time after which BE priority is boosted (default: 60)
        aging_boost: Priority boost applied after threshold (default: 0.5)
        priority_promotion_threshold_seconds: Time after which BE is promoted to RT (default: 600)
        guaranteed_be_ratio: BE guaranteed after this many consecutive RT releases (default: 5, 0 to disable)
    """

    def __init__(
        self,
        total_slots: int = 2,
        rt_reserved_slots: int = 1,
        aging_threshold_seconds: float = 60.0,
        aging_boost: float = 0.5,
        preemption_enabled: bool = True,
        preemption_wait_threshold: float = 2.0,
        priority_promotion_threshold_seconds: float = 600.0,
        guaranteed_be_ratio: int = 5,
        max_queue_depth: int = 0,
        rt_scheduling_mode: str = "fifo",
    ):
        if total_slots < 1:
            raise ValueError("total_slots must be >= 1")
        if rt_reserved_slots > total_slots:
            raise ValueError("rt_reserved_slots cannot exceed total_slots")

        self._total_slots = total_slots
        self._rt_reserved = rt_reserved_slots
        self._be_slots = total_slots - rt_reserved_slots
        self._max_queue_depth = max_queue_depth
        self._rt_scheduling_mode = rt_scheduling_mode

        # Slot counters
        self._rt_available = rt_reserved_slots
        # BE slots are the non-RT slots; when all slots are RT reserved, BE must queue
        self._be_available = self._be_slots

        # Priority queues (min-heap by priority_score)
        self._rt_queue: list[QueuedRequest] = []
        self._be_queue: list[QueuedRequest] = []

        # Aging configuration
        self._aging_threshold = aging_threshold_seconds
        self._aging_boost = aging_boost

        # Priority promotion configuration (BE -> RT after threshold)
        self._priority_promotion_threshold = priority_promotion_threshold_seconds

        # Guaranteed bandwidth configuration
        self._guaranteed_be_ratio = guaranteed_be_ratio
        self._consecutive_rt_releases = 0

        # Preemption configuration
        self._preemption_enabled = preemption_enabled
        self._preemption_threshold = preemption_wait_threshold
        self._active_requests: Dict[str, CancellableRequest] = {}

        self._lock = asyncio.Lock()
        self._last_wait_time: float = 0.0

        # Leak detection: track acquired slots
        self._slot_counter: int = 0
        self._acquired_slots: Dict[int, AcquiredSlot] = {}
        self._leak_threshold_seconds: float = 300.0  # 5 minutes default

        logger.info(
            "HybridPrioritySemaphore initialized",
            extra={
                "total_slots": total_slots,
                "rt_reserved": rt_reserved_slots,
                "be_slots": self._be_slots,
                "aging_threshold": aging_threshold_seconds,
                "priority_promotion_threshold": priority_promotion_threshold_seconds,
                "guaranteed_be_ratio": guaranteed_be_ratio,
                "preemption_enabled": preemption_enabled,
                "preemption_wait_threshold": preemption_wait_threshold,
                "max_queue_depth": max_queue_depth,
                "rt_scheduling_mode": rt_scheduling_mode,
            },
        )

    def _compute_priority_score(
        self, high_priority: bool, enqueue_time: float
    ) -> float:
        """
        Compute priority score with aging and priority promotion.
        Lower score = higher priority.

        Priority progression for BE requests:
        - Fresh (0 - aging_threshold): score = 1.0
        - Aging (aging_threshold - promotion_threshold): score gradually decreases
        - Promoted (> promotion_threshold): score = 0.0 (RT level)
        """
        base_score = 0.0 if high_priority else 1.0

        # Apply aging and promotion for BE requests
        if not high_priority:
            wait_time = time.monotonic() - enqueue_time

            # Priority promotion: BE becomes RT-like after promotion threshold
            if wait_time > self._priority_promotion_threshold:
                # Promote to RT level (score = 0.0)
                return 0.0

            # Standard aging: gradually reduce score after aging threshold
            if wait_time > self._aging_threshold:
                # Boost priority based on excess wait time
                aging_factor = (
                    (wait_time - self._aging_threshold) * self._aging_boost / 60.0
                )
                base_score = max(0.1, base_score - aging_factor)  # Don't go below 0.1

        return base_score

    def register_active_request(
        self, task_id: str, cancel_event: asyncio.Event, is_high_priority: bool
    ) -> None:
        """
        Register an active request that can be preempted.

        Args:
            task_id: Unique identifier for the request
            cancel_event: Event to signal cancellation
            is_high_priority: Whether this is a high priority request
        """
        self._active_requests[task_id] = CancellableRequest(
            task_id=task_id,
            cancel_event=cancel_event,
            start_time=time.monotonic(),
            is_high_priority=is_high_priority,
        )
        logger.debug(
            "Registered active request",
            extra={"task_id": task_id, "is_high_priority": is_high_priority},
        )

    def unregister_active_request(self, task_id: str) -> None:
        """
        Unregister an active request.

        Args:
            task_id: Unique identifier for the request
        """
        if task_id in self._active_requests:
            del self._active_requests[task_id]
            logger.debug("Unregistered active request", extra={"task_id": task_id})

    def _has_preemptable_be(self) -> bool:
        """Check if there are any preemptable BE requests."""
        return any(
            not req.is_high_priority for req in self._active_requests.values()
        )

    async def _preempt_oldest_be(self) -> bool:
        """
        Preempt the oldest BE request to free up a slot for RT.

        Returns:
            True if preemption was triggered, False otherwise
        """
        # Find BE requests (non-high-priority)
        be_requests = [
            req for req in self._active_requests.values() if not req.is_high_priority
        ]
        if not be_requests:
            return False

        # Find the oldest BE request
        oldest = min(be_requests, key=lambda r: r.start_time)
        logger.warning(
            "Preempting BE request for RT priority",
            extra={
                "task_id": oldest.task_id,
                "running_time": time.monotonic() - oldest.start_time,
            },
        )

        # Signal cancellation
        oldest.cancel_event.set()
        return True

    def _track_acquire(self, is_high_priority: bool, context: str = "") -> int:
        """Track a slot acquisition for leak detection. Returns slot_id."""
        self._slot_counter += 1
        slot_id = self._slot_counter
        self._acquired_slots[slot_id] = AcquiredSlot(
            slot_id=slot_id,
            acquired_at=time.monotonic(),
            is_high_priority=is_high_priority,
            context=context,
        )
        return slot_id

    def _track_release(self, slot_id: int) -> None:
        """Remove a tracked slot on release."""
        if slot_id in self._acquired_slots:
            del self._acquired_slots[slot_id]

    def check_leaks(self) -> list[AcquiredSlot]:
        """Check for potentially leaked slots (held longer than threshold).

        Returns:
            List of AcquiredSlot entries that have exceeded the leak threshold.
        """
        now = time.monotonic()
        leaked = []
        for slot in self._acquired_slots.values():
            hold_time = now - slot.acquired_at
            if hold_time > self._leak_threshold_seconds:
                leaked.append(slot)
                logger.warning(
                    "Potential semaphore slot leak detected",
                    extra={
                        "slot_id": slot.slot_id,
                        "hold_time_seconds": round(hold_time, 2),
                        "threshold_seconds": self._leak_threshold_seconds,
                        "is_high_priority": slot.is_high_priority,
                        "context": slot.context,
                    },
                )
        return leaked

    async def acquire(self, high_priority: bool = False) -> float:
        """
        Acquire a slot with RT/BE scheduling.

        Args:
            high_priority: True for RT (streaming), False for BE (batch)

        Returns:
            Wait time in seconds

        Raises:
            QueueFullError: If max_queue_depth is set and queue is full
        """
        start_time = time.monotonic()

        async with self._lock:
            # Check queue depth limit before allowing queuing
            if self._max_queue_depth > 0:
                current_depth = len(self._rt_queue) + len(self._be_queue)
                if current_depth >= self._max_queue_depth:
                    # Check if a slot is immediately available (no queuing needed)
                    slot_available = False
                    if high_priority:
                        slot_available = self._rt_available > 0 or (
                            self._rt_reserved == 0 and self._be_available > 0
                        )
                    else:
                        slot_available = self._be_available > 0 or (
                            self._be_slots == 0 and self._rt_available > 0
                        )

                    if not slot_available:
                        logger.warning(
                            "Queue full, rejecting request",
                            extra={
                                "current_depth": current_depth,
                                "max_queue_depth": self._max_queue_depth,
                                "high_priority": high_priority,
                            },
                        )
                        raise QueueFullError(
                            f"Queue depth {current_depth} >= max {self._max_queue_depth}"
                        )
            if high_priority:
                # Try RT reserved slot first
                if self._rt_available > 0:
                    self._rt_available -= 1
                    self._last_wait_time = 0.0
                    self._track_acquire(is_high_priority=True, context="rt_immediate")
                    logger.debug(
                        "RT slot acquired immediately",
                        extra={"rt_available": self._rt_available},
                    )
                    return 0.0
                # Fallback to BE slot if no RT slots reserved (rt_reserved=0)
                elif self._rt_reserved == 0 and self._be_available > 0:
                    self._be_available -= 1
                    self._last_wait_time = 0.0
                    self._track_acquire(is_high_priority=True, context="hp_be_fallback")
                    logger.debug(
                        "High priority acquired BE slot (no RT reserved)",
                        extra={"be_available": self._be_available},
                    )
                    return 0.0

                # No slot available for RT - try preemption
                if self._preemption_enabled and self._has_preemptable_be():
                    logger.info(
                        "RT request blocked, triggering preemption",
                        extra={"active_requests": len(self._active_requests)},
                    )
                    await self._preempt_oldest_be()
            else:
                # Try BE slot
                if self._be_available > 0:
                    self._be_available -= 1
                    self._last_wait_time = 0.0
                    self._track_acquire(is_high_priority=False, context="be_immediate")
                    logger.debug(
                        "BE slot acquired immediately",
                        extra={"be_available": self._be_available},
                    )
                    return 0.0
                # Fallback to RT slot if no BE slots exist (all slots are RT)
                # This prevents deadlock when rt_reserved == total_slots
                elif self._be_slots == 0 and self._rt_available > 0:
                    self._rt_available -= 1
                    self._last_wait_time = 0.0
                    self._track_acquire(is_high_priority=False, context="lp_rt_fallback")
                    logger.debug(
                        "Low priority acquired RT slot (no BE slots configured)",
                        extra={"rt_available": self._rt_available},
                    )
                    return 0.0

            # No slot available, queue the request
            future = asyncio.get_event_loop().create_future()
            priority_score = self._compute_priority_score(high_priority, start_time)
            # LIFO mode for RT: negate enqueue_time so newest (largest timestamp)
            # becomes smallest value and pops first from the min-heap
            if high_priority and self._rt_scheduling_mode == "lifo":
                enqueue_time = -start_time
            else:
                enqueue_time = start_time
            request = QueuedRequest(
                priority_score=priority_score,
                enqueue_time=enqueue_time,
                future=future,
                is_high_priority=high_priority,
            )

            if high_priority:
                heapq.heappush(self._rt_queue, request)
                logger.info(
                    "RT request queued",
                    extra={
                        "queue_size": len(self._rt_queue),
                        "priority_score": priority_score,
                    },
                )
            else:
                heapq.heappush(self._be_queue, request)
                logger.info(
                    "BE request queued",
                    extra={
                        "queue_size": len(self._be_queue),
                        "priority_score": priority_score,
                    },
                )

        # Wait for slot
        try:
            await future
            wait_time = time.monotonic() - start_time
            self._last_wait_time = wait_time
            self._track_acquire(is_high_priority=high_priority, context="queued")

            if wait_time > 10.0:
                logger.warning(
                    "Long queue wait detected",
                    extra={
                        "wait_time_seconds": round(wait_time, 2),
                        "high_priority": high_priority,
                    },
                )

            return wait_time
        except asyncio.CancelledError:
            if not future.done():
                future.cancel()
            # Purge this (and any other) cancelled futures from queues
            # so they don't occupy queue slots or get popped by release()
            async with self._lock:
                self._purge_cancelled_from_queues()
            raise

    def release(self, was_high_priority: bool = False, slot_id: Optional[int] = None) -> None:
        """
        Release a slot and wake up next waiter.

        Implements guaranteed bandwidth for BE requests:
        - After guaranteed_be_ratio consecutive RT releases, force BE processing
        - This prevents BE starvation even when RT requests are continuous

        Args:
            was_high_priority: Whether the released slot was RT
            slot_id: Optional slot_id from acquire tracking (for precise leak detection)
        """
        # Untrack acquired slot
        if slot_id is not None:
            self._track_release(slot_id)
        else:
            # Find and release the oldest matching slot
            matching = [
                s for s in self._acquired_slots.values()
                if s.is_high_priority == was_high_priority
            ]
            if matching:
                oldest = min(matching, key=lambda s: s.acquired_at)
                self._track_release(oldest.slot_id)

        # Recompute priorities with aging and handle promotions
        self._apply_aging()

        # Check guaranteed bandwidth: force BE after N consecutive RT releases
        force_be = False
        if self._guaranteed_be_ratio > 0 and self._be_queue:
            if was_high_priority:
                self._consecutive_rt_releases += 1
                if self._consecutive_rt_releases >= self._guaranteed_be_ratio:
                    force_be = True
                    logger.info(
                        "Guaranteed bandwidth triggered: forcing BE request",
                        extra={
                            "consecutive_rt_releases": self._consecutive_rt_releases,
                            "guaranteed_be_ratio": self._guaranteed_be_ratio,
                            "be_queue_size": len(self._be_queue),
                        },
                    )

        woke_up = False

        # If guaranteed bandwidth triggered, process BE first
        if force_be:
            while self._be_queue and not woke_up:
                request = heapq.heappop(self._be_queue)
                if not request.future.done() and not request.future.cancelled():
                    loop = asyncio.get_event_loop()
                    if loop.is_running():
                        loop.call_soon_threadsafe(request.future.set_result, True)
                    else:
                        request.future.set_result(True)
                    woke_up = True
                    self._consecutive_rt_releases = 0  # Reset counter after BE processed
                    logger.debug("Woke up BE waiter (guaranteed bandwidth)")
                    break

        # Standard priority order: RT queue first, then BE queue
        if not woke_up:
            # Try RT queue first
            while self._rt_queue and not woke_up:
                request = heapq.heappop(self._rt_queue)
                if not request.future.done() and not request.future.cancelled():
                    loop = asyncio.get_event_loop()
                    if loop.is_running():
                        loop.call_soon_threadsafe(request.future.set_result, True)
                    else:
                        request.future.set_result(True)
                    woke_up = True
                    logger.debug("Woke up RT waiter")

            # Try BE queue if no RT waiters
            if not woke_up:
                while self._be_queue:
                    request = heapq.heappop(self._be_queue)
                    if not request.future.done() and not request.future.cancelled():
                        loop = asyncio.get_event_loop()
                        if loop.is_running():
                            loop.call_soon_threadsafe(request.future.set_result, True)
                        else:
                            request.future.set_result(True)
                        woke_up = True
                        self._consecutive_rt_releases = 0  # Reset counter after BE processed
                        logger.debug("Woke up BE waiter")
                        break

        # No waiters, return slot to pool
        if not woke_up:
            if was_high_priority:
                self._rt_available = min(self._rt_available + 1, self._rt_reserved)
            else:
                self._be_available = min(self._be_available + 1, self._be_slots)

    def _purge_cancelled_from_queues(self) -> None:
        """Remove cancelled/done futures from both queues.

        Called when a waiting request is cancelled (e.g. HTTP client disconnect)
        to prevent stale entries from occupying queue slots.
        """
        purged = 0
        for _, queue in [("rt", self._rt_queue), ("be", self._be_queue)]:
            original_len = len(queue)
            live = [r for r in queue if not r.future.done() and not r.future.cancelled()]
            removed = original_len - len(live)
            if removed > 0:
                purged += removed
                queue.clear()
                for r in live:
                    heapq.heappush(queue, r)

        if purged > 0:
            logger.info(
                "Purged cancelled/done requests from queues",
                extra={
                    "purged_count": purged,
                    "rt_queue_size": len(self._rt_queue),
                    "be_queue_size": len(self._be_queue),
                },
            )

    def _apply_aging(self) -> None:
        """Recompute BE queue priorities with aging and handle promotions.

        Priority promotion: BE requests waiting longer than promotion threshold
        are promoted to the RT queue, ensuring they get processed with RT priority.
        This is a key mechanism to prevent starvation of batch requests.
        """
        if not self._be_queue:
            return

        current_time = time.monotonic()
        new_be_queue: list[QueuedRequest] = []
        promoted_count = 0
        purged_count = 0

        for request in self._be_queue:
            # Skip cancelled/done requests (client disconnected while queued)
            if request.future.done() or request.future.cancelled():
                purged_count += 1
                continue

            wait_time = current_time - request.enqueue_time
            new_score = self._compute_priority_score(False, request.enqueue_time)

            # Check for priority promotion (BE -> RT)
            if wait_time > self._priority_promotion_threshold:
                # Promote to RT queue
                request.priority_score = 0.0  # RT priority
                request.is_high_priority = True  # Mark as promoted
                heapq.heappush(self._rt_queue, request)
                promoted_count += 1
                logger.warning(
                    "BE request promoted to RT queue due to long wait",
                    extra={
                        "wait_time_seconds": round(wait_time, 2),
                        "promotion_threshold": self._priority_promotion_threshold,
                    },
                )
            else:
                # Apply normal aging
                if new_score != request.priority_score:
                    logger.debug(
                        "Aging applied to BE request",
                        extra={
                            "old_score": request.priority_score,
                            "new_score": new_score,
                            "wait_time": wait_time,
                        },
                    )
                request.priority_score = new_score
                heapq.heappush(new_be_queue, request)

        self._be_queue = new_be_queue

        if purged_count > 0:
            logger.info(
                "Purged cancelled/done requests during aging",
                extra={"purged_count": purged_count},
            )

        if promoted_count > 0:
            logger.info(
                "BE requests promoted to RT queue",
                extra={
                    "promoted_count": promoted_count,
                    "remaining_be_queue": len(self._be_queue),
                    "rt_queue_size": len(self._rt_queue),
                },
            )

    @property
    def last_wait_time(self) -> float:
        """Get the wait time from the last acquire operation."""
        return self._last_wait_time

    def queue_status(self) -> dict:
        """Get current queue status for monitoring."""
        available = self._rt_available + self._be_available
        current_depth = len(self._rt_queue) + len(self._be_queue)
        accepting = (
            self._max_queue_depth == 0
            or current_depth < self._max_queue_depth
            or available > 0
        )
        return {
            "rt_queue": len(self._rt_queue),
            "be_queue": len(self._be_queue),
            "total_slots": self._total_slots,
            "available_slots": available,
            "accepting": accepting,
            "max_queue_depth": self._max_queue_depth,
            "acquired_slots": len(self._acquired_slots),
        }

    def __repr__(self) -> str:
        return (
            f"<HybridPrioritySemaphore "
            f"rt={self._rt_available}/{self._rt_reserved} "
            f"be={self._be_available}/{self._be_slots} "
            f"rt_queue={len(self._rt_queue)} be_queue={len(self._be_queue)}>"
        )
