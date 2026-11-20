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

from news_creator.domain.errors import QueueFullError

logger = logging.getLogger(__name__)
_LONG_QUEUE_WAIT_SECONDS = 10.0


@dataclass
class AcquiredSlot:
    """Tracks an acquired semaphore slot for leak detection and ownership."""

    slot_id: int
    acquired_at: float  # monotonic time
    is_high_priority: bool
    home_pool: str = ""  # "rt" or "be" — which pool this slot was taken from
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
        aging_sweep_interval_releases: Releases between full aging sweeps on
            release() (default: 8); a sweep also runs sooner if overdue by
            wall-clock so starvation guarantees hold under low traffic too
    """

    def __init__(  # noqa: PLR0913 - constructor injection of semaphore scheduling parameters
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
        aging_sweep_interval_releases: int = 8,
    ) -> None:
        if total_slots < 1:
            msg = "total_slots must be >= 1"
            raise ValueError(msg)
        if rt_reserved_slots > total_slots:
            msg = "rt_reserved_slots cannot exceed total_slots"
            raise ValueError(msg)

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

        # Aging sweep throttle: _apply_aging() does an O(N) rebuild of the BE
        # queue. Running it on every release() call makes release() cost
        # scale with BE queue depth on every call, which is expensive under
        # a release burst (e.g. large hierarchical job fan-out). Instead,
        # run the full sweep only every `aging_sweep_interval_releases`
        # releases, or sooner if overdue by wall-clock (bounded by the
        # tightest of aging/promotion thresholds) so anti-starvation
        # guarantees still hold for low-traffic queues.
        self._aging_sweep_interval_releases = max(1, aging_sweep_interval_releases)
        self._releases_since_aging_sweep = 0
        self._last_aging_sweep_time = time.monotonic()

        # Priority promotion configuration (BE -> RT after threshold)
        self._priority_promotion_threshold = priority_promotion_threshold_seconds

        # Guaranteed bandwidth configuration
        self._guaranteed_be_ratio = guaranteed_be_ratio
        self._consecutive_rt_releases = 0

        # Preemption configuration
        self._preemption_enabled = preemption_enabled
        self._preemption_threshold = preemption_wait_threshold
        self._active_requests: dict[str, CancellableRequest] = {}

        self._lock = asyncio.Lock()
        self._last_wait_time: float = 0.0

        # Leak detection: track acquired slots
        self._slot_counter: int = 0
        self._acquired_slots: dict[int, AcquiredSlot] = {}
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
                "aging_sweep_interval_releases": self._aging_sweep_interval_releases,
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
        return any(not req.is_high_priority for req in self._active_requests.values())

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

    def _track_acquire(
        self, is_high_priority: bool, context: str = "", home_pool: str = ""
    ) -> int:
        """Track a slot acquisition for leak detection. Returns slot_id."""
        self._slot_counter += 1
        slot_id = self._slot_counter
        self._acquired_slots[slot_id] = AcquiredSlot(
            slot_id=slot_id,
            acquired_at=time.monotonic(),
            is_high_priority=is_high_priority,
            home_pool=home_pool,
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

    async def acquire(self, high_priority: bool = False) -> tuple[float, int]:
        """
        Acquire a slot with RT/BE scheduling.

        Args:
            high_priority: True for RT (streaming), False for BE (batch)

        Returns:
            Tuple of (wait_time_seconds, slot_id). The slot_id MUST be passed
            back to release() for correct pool ownership tracking.

        Raises:
            QueueFullError: If max_queue_depth is set and queue is full
        """
        start_time = time.monotonic()

        async with self._lock:
            self._ensure_queue_capacity(high_priority)
            immediate_sid = self._try_acquire_immediate(high_priority)
            if immediate_sid is not None:
                return 0.0, immediate_sid

            # No slot available for RT - try preemption
            if (
                high_priority
                and self._preemption_enabled
                and self._has_preemptable_be()
            ):
                logger.info(
                    "RT request blocked, triggering preemption",
                    extra={"active_requests": len(self._active_requests)},
                )
                await self._preempt_oldest_be()

            future = self._enqueue_waiter(high_priority, start_time)

        # Wait for slot
        try:
            result = await future
            wait_time, sid = self._record_queued_acquire(
                result, start_time, high_priority
            )
        except asyncio.CancelledError:
            if future.done() and not future.cancelled():
                # Slot was transferred to us via set_result() in release(),
                # but our task was cancelled before _track_acquire() ran.
                # Recover the slot by releasing it back to the pool or
                # forwarding it to the next queued waiter.
                home_pool = self._resolve_transferred_pool(
                    future.result(), high_priority
                )
                logger.info(
                    "Recovering slot from cancelled waiter",
                    extra={"home_pool": home_pool, "high_priority": high_priority},
                )
                self._release_slot_to_pool_or_waiter(home_pool)
            elif not future.done():
                future.cancel()
            # Purge this (and any other) cancelled futures from queues
            # so they don't occupy queue slots or get popped by release()
            async with self._lock:
                self._purge_cancelled_from_queues()
            raise
        else:
            return wait_time, sid

    def _has_immediate_slot(self, high_priority: bool) -> bool:
        """Return True if a slot is immediately available for the given priority."""
        if high_priority:
            return self._rt_available > 0 or (
                self._rt_reserved == 0 and self._be_available > 0
            )
        return self._be_available > 0 or (
            self._be_slots == 0 and self._rt_available > 0
        )

    def _ensure_queue_capacity(self, high_priority: bool) -> None:
        """Raise QueueFullError if max_queue_depth is reached and no immediate slot is free."""
        if self._max_queue_depth <= 0:
            return
        current_depth = len(self._rt_queue) + len(self._be_queue)
        if current_depth >= self._max_queue_depth and not self._has_immediate_slot(
            high_priority
        ):
            logger.warning(
                "Queue full, rejecting request",
                extra={
                    "current_depth": current_depth,
                    "max_queue_depth": self._max_queue_depth,
                    "high_priority": high_priority,
                },
            )
            msg = f"Queue depth {current_depth} >= max {self._max_queue_depth}"
            raise QueueFullError(msg)

    def _try_acquire_immediate(self, high_priority: bool) -> int | None:
        """Try to acquire an immediately available slot, returning slot_id or None."""
        if high_priority:
            if self._rt_available > 0:
                self._rt_available -= 1
                self._last_wait_time = 0.0
                sid = self._track_acquire(
                    is_high_priority=True, context="rt_immediate", home_pool="rt"
                )
                logger.debug(
                    "RT slot acquired immediately",
                    extra={"rt_available": self._rt_available},
                )
                return sid
            if self._rt_reserved == 0 and self._be_available > 0:
                self._be_available -= 1
                self._last_wait_time = 0.0
                sid = self._track_acquire(
                    is_high_priority=True, context="hp_be_fallback", home_pool="be"
                )
                logger.debug(
                    "High priority acquired BE slot (no RT reserved)",
                    extra={"be_available": self._be_available},
                )
                return sid
            return None

        if self._be_available > 0:
            self._be_available -= 1
            self._last_wait_time = 0.0
            sid = self._track_acquire(
                is_high_priority=False, context="be_immediate", home_pool="be"
            )
            logger.debug(
                "BE slot acquired immediately",
                extra={"be_available": self._be_available},
            )
            return sid
        if self._be_slots == 0 and self._rt_available > 0:
            self._rt_available -= 1
            self._last_wait_time = 0.0
            sid = self._track_acquire(
                is_high_priority=False, context="lp_rt_fallback", home_pool="rt"
            )
            logger.debug(
                "Low priority acquired RT slot (no BE slots configured)",
                extra={"rt_available": self._rt_available},
            )
            return sid
        return None

    def _enqueue_waiter(self, high_priority: bool, start_time: float) -> asyncio.Future:
        """Create and enqueue a QueuedRequest future into the RT or BE priority queue."""
        future = asyncio.get_running_loop().create_future()
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
        return future

    @staticmethod
    def _resolve_transferred_pool(result: object, high_priority: bool) -> str:
        """Resolve home_pool from a waiter future result or fall back to caller priority."""
        if isinstance(result, str) and result:
            return result
        return "rt" if high_priority else "be"

    def _record_queued_acquire(
        self, result: object, start_time: float, high_priority: bool
    ) -> tuple[float, int]:
        """Record wait time and slot ownership after a queued future completes."""
        wait_time = time.monotonic() - start_time
        self._last_wait_time = wait_time
        inherited_pool = self._resolve_transferred_pool(result, high_priority)
        sid = self._track_acquire(
            is_high_priority=high_priority,
            context="queued",
            home_pool=inherited_pool,
        )

        if wait_time > _LONG_QUEUE_WAIT_SECONDS:
            logger.warning(
                "Long queue wait detected",
                extra={
                    "wait_time_seconds": round(wait_time, 2),
                    "high_priority": high_priority,
                },
            )
        return wait_time, sid

    def release(
        self, was_high_priority: bool = False, slot_id: int | None = None
    ) -> None:
        """
        Release a slot and wake up next waiter.

        Slot ownership: The slot is returned to its *home pool* (the pool it was
        originally acquired from), not based on the caller's priority. This prevents
        slot migration across pools during preemption → release chains.

        Args:
            was_high_priority: Whether the caller was high priority (used for
                guaranteed bandwidth tracking; NOT used for pool return)
            slot_id: Optional slot_id from acquire tracking (for precise ownership)
        """
        home_pool = self._resolve_release_home_pool(was_high_priority, slot_id)

        # Recompute priorities with aging and handle promotions (throttled --
        # see _should_run_aging_sweep()).
        if self._should_run_aging_sweep():
            self._apply_aging()

        force_be = self._should_force_be(was_high_priority)

        woke_up = False
        if force_be:
            woke_up = self._try_wake_from_be_queue(home_pool, guaranteed=True)

        # Standard priority order: RT queue first, then BE queue
        if not woke_up:
            woke_up = self._try_wake_from_rt_queue(home_pool)
        if not woke_up:
            woke_up = self._try_wake_from_be_queue(home_pool, guaranteed=False)

        # No waiters, return slot to its home pool
        if not woke_up:
            self._return_slot_to_pool(home_pool)

        self._check_slot_invariant(home_pool, woke_up=woke_up)

    def _resolve_release_home_pool(
        self, was_high_priority: bool, slot_id: int | None
    ) -> str:
        """Determine home_pool from tracked slot ownership and untrack the slot."""
        home_pool = ""
        if slot_id is not None:
            slot_info = self._acquired_slots.get(slot_id)
            if slot_info:
                home_pool = slot_info.home_pool
            self._track_release(slot_id)
        else:
            # Legacy fallback for callers that haven't migrated to (wait_time, slot_id).
            # Find oldest matching slot to determine home_pool.
            matching = [
                s
                for s in self._acquired_slots.values()
                if s.is_high_priority == was_high_priority
            ]
            if matching:
                oldest = min(matching, key=lambda s: s.acquired_at)
                home_pool = oldest.home_pool
                self._track_release(oldest.slot_id)
            else:
                logger.warning(
                    "release() called without slot_id and no matching tracked slots",
                    extra={
                        "was_high_priority": was_high_priority,
                        "acquired_slots": len(self._acquired_slots),
                    },
                )

        # Last resort: infer from caller priority (only if no tracking data at all)
        if not home_pool:
            home_pool = "rt" if was_high_priority else "be"
        return home_pool

    def _should_force_be(self, was_high_priority: bool) -> bool:
        """Check guaranteed bandwidth and update consecutive RT release count."""
        if self._guaranteed_be_ratio > 0 and self._be_queue and was_high_priority:
            self._consecutive_rt_releases += 1
            if self._consecutive_rt_releases >= self._guaranteed_be_ratio:
                logger.info(
                    "Guaranteed bandwidth triggered: forcing BE request",
                    extra={
                        "consecutive_rt_releases": self._consecutive_rt_releases,
                        "guaranteed_be_ratio": self._guaranteed_be_ratio,
                        "be_queue_size": len(self._be_queue),
                    },
                )
                return True
        return False

    def _try_wake_from_be_queue(self, home_pool: str, *, guaranteed: bool) -> bool:
        """Pop and wake the next live BE waiter, resetting consecutive RT count."""
        while self._be_queue:
            request = heapq.heappop(self._be_queue)
            if self._try_wake_waiter(request, home_pool):
                self._consecutive_rt_releases = 0
                if guaranteed:
                    logger.debug("Woke up BE waiter (guaranteed bandwidth)")
                else:
                    logger.debug("Woke up BE waiter")
                return True
        return False

    def _try_wake_from_rt_queue(self, home_pool: str) -> bool:
        """Pop and wake the next live RT waiter."""
        while self._rt_queue:
            request = heapq.heappop(self._rt_queue)
            if self._try_wake_waiter(request, home_pool):
                logger.debug("Woke up RT waiter")
                return True
        return False

    def _return_slot_to_pool(self, home_pool: str) -> None:
        """Return an unconsumed slot back to its home pool counter."""
        if home_pool == "rt":
            self._rt_available = min(self._rt_available + 1, self._rt_reserved)
        else:
            self._be_available = min(self._be_available + 1, self._be_slots)

    def _check_slot_invariant(self, home_pool: str, *, woke_up: bool) -> None:
        """Verify available + acquired equals total_slots (accounting for in-transit wake)."""
        expected_total = self._total_slots
        if woke_up:
            expected_total -= 1

        total_tracked = (
            self._rt_available + self._be_available + len(self._acquired_slots)
        )
        if total_tracked != expected_total:
            logger.error(
                "SLOT INVARIANT VIOLATION: available + acquired != total_slots",
                extra={
                    "rt_available": self._rt_available,
                    "be_available": self._be_available,
                    "acquired_count": len(self._acquired_slots),
                    "total_tracked": total_tracked,
                    "expected_total": expected_total,
                    "total_slots": self._total_slots,
                    "in_transit": woke_up,
                    "rt_queue_size": len(self._rt_queue),
                    "be_queue_size": len(self._be_queue),
                    "home_pool": home_pool,
                },
            )

    def _release_slot_to_pool_or_waiter(self, home_pool: str) -> None:
        """Return a slot to its home pool or transfer to the next queued waiter.

        Used by release() and by the CancelledError recovery path in acquire()
        when a waiter's task is cancelled after receiving a slot via set_result().
        """
        woke_up = False

        # Try RT queue first, then BE queue
        while self._rt_queue and not woke_up:
            request = heapq.heappop(self._rt_queue)
            if self._try_wake_waiter(request, home_pool):
                woke_up = True

        if not woke_up:
            while self._be_queue:
                request = heapq.heappop(self._be_queue)
                if self._try_wake_waiter(request, home_pool):
                    woke_up = True
                    break

        # No waiters — return slot to its home pool
        if not woke_up:
            if home_pool == "rt":
                self._rt_available = min(self._rt_available + 1, self._rt_reserved)
            else:
                self._be_available = min(self._be_available + 1, self._be_slots)

    def _try_wake_waiter(self, request: QueuedRequest, home_pool: str) -> bool:
        """Try to wake a queued waiter by setting its future result.

        Uses direct set_result() instead of call_soon_threadsafe() to avoid
        a race condition where the future gets cancelled between scheduling
        and execution, causing the slot to be permanently lost.

        Args:
            request: The queued request to wake
            home_pool: The home pool of the slot being transferred

        Returns:
            True if the waiter was successfully woken, False if the future
            was already done/cancelled (caller should try next waiter).
        """
        if request.future.done() or request.future.cancelled():
            return False
        try:
            request.future.set_result(home_pool)
        except asyncio.InvalidStateError:
            # Future was cancelled between our check and set_result.
            # This should be rare but can happen under high concurrency.
            logger.warning(
                "Failed to wake waiter: future cancelled during set_result",
                extra={"home_pool": home_pool},
            )
            return False
        else:
            return True

    def _purge_cancelled_from_queues(self) -> None:
        """Remove cancelled/done futures from both queues.

        Called when a waiting request is cancelled (e.g. HTTP client disconnect)
        to prevent stale entries from occupying queue slots.
        """
        purged = 0
        for _, queue in [("rt", self._rt_queue), ("be", self._be_queue)]:
            original_len = len(queue)
            live = [
                r for r in queue if not r.future.done() and not r.future.cancelled()
            ]
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

    def _should_run_aging_sweep(self) -> bool:
        """Decide whether release() should run the full _apply_aging() sweep.

        Returns True (and resets the throttle counters) when either:
        - `aging_sweep_interval_releases` releases have elapsed since the
          last sweep (bounds the O(N) rebuild cost under a release burst), or
        - the sweep is overdue by wall-clock, using the tighter of the aging
          and priority-promotion thresholds as the staleness budget. This
          keeps the anti-starvation guarantee intact for low-traffic queues,
          where a burst of releases may never happen but a BE request must
          still age/promote in bounded time.
        """
        self._releases_since_aging_sweep += 1

        if self._releases_since_aging_sweep >= self._aging_sweep_interval_releases:
            self._releases_since_aging_sweep = 0
            self._last_aging_sweep_time = time.monotonic()
            return True

        overdue_budget = min(self._aging_threshold, self._priority_promotion_threshold)
        if time.monotonic() - self._last_aging_sweep_time >= overdue_budget:
            self._releases_since_aging_sweep = 0
            self._last_aging_sweep_time = time.monotonic()
            return True

        return False

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
