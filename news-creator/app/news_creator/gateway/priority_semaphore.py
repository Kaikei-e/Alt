"""Priority semaphore implementation for request prioritization.

This module provides a semaphore implementation that prioritizes high-priority
requests (streaming/on-time) over low-priority requests (batch processing).
"""

import asyncio
import logging
import time

logger = logging.getLogger(__name__)


class PrioritySemaphore:
    """
    Priority-based semaphore - prioritizes high-priority requests (streaming).

    Extends the concept of FIFOSemaphore with two queues:
    - high_priority_waiters: streaming requests (on-time from alt-frontend-sv)
    - low_priority_waiters: non-streaming requests (batch from pre-processor)

    On release, high-priority queue is checked first, ensuring streaming
    requests bypass batch processing queue.

    Example:
        semaphore = PrioritySemaphore(2)

        # High priority (streaming) - will be processed first
        wait_time = await semaphore.acquire(high_priority=True)
        try:
            await do_streaming_work()
        finally:
            semaphore.release()

        # Low priority (batch) - processed after all high priority
        wait_time = await semaphore.acquire(high_priority=False)
        try:
            await do_batch_work()
        finally:
            semaphore.release()
    """

    def __init__(self, value: int = 1):
        """
        Initialize priority semaphore.

        Args:
            value: Initial value for the semaphore (maximum concurrent tasks).
                  Must be >= 0.

        Raises:
            ValueError: If value < 0
        """
        if value < 0:
            raise ValueError("Semaphore initial value must be >= 0")

        self._value = value
        self._max_value = value
        self._high_priority_waiters: asyncio.Queue[asyncio.Future] = asyncio.Queue()
        self._low_priority_waiters: asyncio.Queue[asyncio.Future] = asyncio.Queue()
        self._lock = asyncio.Lock()
        self._last_wait_time: float = 0.0

    def __repr__(self):
        """String representation of the semaphore."""
        high_count = self._high_priority_waiters.qsize()
        low_count = self._low_priority_waiters.qsize()
        return (
            f"<PrioritySemaphore value={self._value}/{self._max_value}, "
            f"high_priority_waiters={high_count}, low_priority_waiters={low_count}>"
        )

    @property
    def last_wait_time(self) -> float:
        """
        Get the wait time from the last acquire operation.

        Returns:
            Wait time in seconds from the most recent acquire call.
        """
        return self._last_wait_time

    def locked(self) -> bool:
        """
        Returns True if semaphore cannot be acquired immediately.

        Returns:
            True if semaphore is at capacity (value == 0) or has waiters
        """
        return (
            self._value == 0
            or not self._high_priority_waiters.empty()
            or not self._low_priority_waiters.empty()
        )

    async def acquire(self, high_priority: bool = False) -> float:
        """
        Acquire the semaphore with specified priority.

        If the internal counter is larger than zero, decrement it and return
        immediately. If it is zero, block and wait until a slot becomes
        available. High-priority requests are served before low-priority ones.

        Args:
            high_priority: If True, request is added to high-priority queue.
                          Typically True for streaming requests (on-time).

        Returns:
            Wait time in seconds (0.0 for immediate acquire, positive for waited)
        """
        start_time = time.monotonic()

        async with self._lock:
            if self._value > 0:
                # Slot available, acquire immediately
                self._value -= 1
                self._last_wait_time = 0.0
                return 0.0

            # No slot available, create future and add to appropriate queue
            future = asyncio.get_event_loop().create_future()
            if high_priority:
                self._high_priority_waiters.put_nowait(future)
                logger.debug(
                    "High priority request queued",
                    extra={"queue_size": self._high_priority_waiters.qsize()},
                )
            else:
                self._low_priority_waiters.put_nowait(future)
                logger.debug(
                    "Low priority request queued",
                    extra={"queue_size": self._low_priority_waiters.qsize()},
                )

        # Wait for a slot to become available
        try:
            await future
            wait_time = time.monotonic() - start_time
            self._last_wait_time = wait_time
            return wait_time
        except asyncio.CancelledError:
            if not future.done():
                future.cancel()
            raise

    def release(self) -> None:
        """
        Release the semaphore, incrementing the internal counter.

        If there are waiting tasks, wake up the first high-priority waiter.
        If no high-priority waiters, wake up the first low-priority waiter.
        """
        # Try to wake up a high-priority waiter first
        while not self._high_priority_waiters.empty():
            try:
                future = self._high_priority_waiters.get_nowait()
                if not future.done() and not future.cancelled():
                    loop = asyncio.get_event_loop()
                    if loop.is_running():
                        loop.call_soon_threadsafe(future.set_result, True)
                    else:
                        future.set_result(True)
                    logger.debug("Woke up high priority waiter")
                    return
            except asyncio.QueueEmpty:
                break

        # No high-priority waiters, try low-priority queue
        while not self._low_priority_waiters.empty():
            try:
                future = self._low_priority_waiters.get_nowait()
                if not future.done() and not future.cancelled():
                    loop = asyncio.get_event_loop()
                    if loop.is_running():
                        loop.call_soon_threadsafe(future.set_result, True)
                    else:
                        future.set_result(True)
                    logger.debug("Woke up low priority waiter")
                    return
            except asyncio.QueueEmpty:
                break

        # No waiters or all waiters were cancelled, increment value
        if self._value < self._max_value:
            self._value += 1

    async def __aenter__(self):
        """Async context manager entry (low priority by default)."""
        await self.acquire(high_priority=False)
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Async context manager exit."""
        self.release()
        return False
