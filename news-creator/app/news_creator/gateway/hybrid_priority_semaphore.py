"""
Hybrid Real-Time / Best-Effort Priority Semaphore.

Based on best practices:
- https://arxiv.org/html/2504.09590v1 (Hybrid RT/BE Scheduling)
- https://huggingface.co/blog/tngtech/llm-performance-request-queueing

This semaphore implementation addresses TTFT latency issues caused by
low-priority batch requests blocking high-priority streaming requests.
"""

import asyncio
import heapq
import logging
import time
from dataclasses import dataclass, field

logger = logging.getLogger(__name__)


@dataclass(order=True)
class QueuedRequest:
    """Request with aging-aware priority."""

    priority_score: float  # Lower = higher priority
    enqueue_time: float = field(compare=False)
    future: asyncio.Future = field(compare=False)
    is_high_priority: bool = field(compare=False)


class HybridPrioritySemaphore:
    """
    Hybrid RT/BE semaphore with reserved slots and aging.

    Features:
    - Reserved slot for real-time (streaming) requests
    - Aging mechanism to prevent starvation of low-priority requests
    - Fair scheduling within priority levels

    Args:
        total_slots: Total concurrent slots (OLLAMA_NUM_PARALLEL)
        rt_reserved_slots: Slots reserved for real-time requests (default: 1)
        aging_threshold_seconds: Time after which BE priority is boosted (default: 60)
        aging_boost: Priority boost applied after threshold (default: 0.5)
    """

    def __init__(
        self,
        total_slots: int = 2,
        rt_reserved_slots: int = 1,
        aging_threshold_seconds: float = 60.0,
        aging_boost: float = 0.5,
    ):
        if total_slots < 1:
            raise ValueError("total_slots must be >= 1")
        if rt_reserved_slots > total_slots:
            raise ValueError("rt_reserved_slots cannot exceed total_slots")

        self._total_slots = total_slots
        self._rt_reserved = rt_reserved_slots
        self._be_slots = total_slots - rt_reserved_slots

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

        self._lock = asyncio.Lock()
        self._last_wait_time: float = 0.0

        logger.info(
            "HybridPrioritySemaphore initialized",
            extra={
                "total_slots": total_slots,
                "rt_reserved": rt_reserved_slots,
                "be_slots": self._be_slots,
                "aging_threshold": aging_threshold_seconds,
            },
        )

    def _compute_priority_score(
        self, high_priority: bool, enqueue_time: float
    ) -> float:
        """
        Compute priority score with aging.
        Lower score = higher priority.
        """
        base_score = 0.0 if high_priority else 1.0

        # Apply aging for BE requests
        if not high_priority:
            wait_time = time.monotonic() - enqueue_time
            if wait_time > self._aging_threshold:
                # Boost priority based on excess wait time
                aging_factor = (
                    (wait_time - self._aging_threshold) * self._aging_boost / 60.0
                )
                base_score = max(0.1, base_score - aging_factor)  # Don't go below 0.1

        return base_score

    async def acquire(self, high_priority: bool = False) -> float:
        """
        Acquire a slot with RT/BE scheduling.

        Args:
            high_priority: True for RT (streaming), False for BE (batch)

        Returns:
            Wait time in seconds
        """
        start_time = time.monotonic()

        async with self._lock:
            if high_priority:
                # Try RT reserved slot first
                if self._rt_available > 0:
                    self._rt_available -= 1
                    self._last_wait_time = 0.0
                    logger.debug(
                        "RT slot acquired immediately",
                        extra={"rt_available": self._rt_available},
                    )
                    return 0.0
                # Fallback to BE slot if no RT slots reserved (rt_reserved=0)
                elif self._rt_reserved == 0 and self._be_available > 0:
                    self._be_available -= 1
                    self._last_wait_time = 0.0
                    logger.debug(
                        "High priority acquired BE slot (no RT reserved)",
                        extra={"be_available": self._be_available},
                    )
                    return 0.0
            else:
                # Try BE slot
                if self._be_available > 0:
                    self._be_available -= 1
                    self._last_wait_time = 0.0
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
                    logger.debug(
                        "Low priority acquired RT slot (no BE slots configured)",
                        extra={"rt_available": self._rt_available},
                    )
                    return 0.0

            # No slot available, queue the request
            future = asyncio.get_event_loop().create_future()
            priority_score = self._compute_priority_score(high_priority, start_time)
            request = QueuedRequest(
                priority_score=priority_score,
                enqueue_time=start_time,
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
            raise

    def release(self, was_high_priority: bool = False) -> None:
        """
        Release a slot and wake up next waiter.

        Args:
            was_high_priority: Whether the released slot was RT
        """
        # Recompute priorities with aging before selecting next
        self._apply_aging()

        # Priority order: RT queue first, then BE queue
        woke_up = False

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
                    logger.debug("Woke up BE waiter")
                    break

        # No waiters, return slot to pool
        if not woke_up:
            if was_high_priority:
                self._rt_available = min(self._rt_available + 1, self._rt_reserved)
            else:
                self._be_available = min(self._be_available + 1, self._be_slots)

    def _apply_aging(self) -> None:
        """Recompute BE queue priorities with aging."""
        if not self._be_queue:
            return

        current_time = time.monotonic()
        new_queue: list[QueuedRequest] = []

        for request in self._be_queue:
            new_score = self._compute_priority_score(False, request.enqueue_time)
            if new_score != request.priority_score:
                logger.debug(
                    "Aging applied to BE request",
                    extra={
                        "old_score": request.priority_score,
                        "new_score": new_score,
                        "wait_time": current_time - request.enqueue_time,
                    },
                )
            request.priority_score = new_score
            heapq.heappush(new_queue, request)

        self._be_queue = new_queue

    @property
    def last_wait_time(self) -> float:
        """Get the wait time from the last acquire operation."""
        return self._last_wait_time

    def __repr__(self) -> str:
        return (
            f"<HybridPrioritySemaphore "
            f"rt={self._rt_available}/{self._rt_reserved} "
            f"be={self._be_available}/{self._be_slots} "
            f"rt_queue={len(self._rt_queue)} be_queue={len(self._be_queue)}>"
        )
