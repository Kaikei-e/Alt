"""FIFO-guaranteed semaphore implementation.

This module provides a semaphore implementation that guarantees FIFO (First In First Out)
ordering of waiting tasks. This ensures that requests are processed in the order they
arrive, which is important for fairness and predictability.
"""

import asyncio
import logging
from typing import Optional

logger = logging.getLogger(__name__)


class FIFOSemaphore:
    """
    A semaphore implementation that guarantees FIFO ordering of waiting tasks.

    Unlike asyncio.Semaphore, which doesn't guarantee FIFO order, this implementation
    uses an asyncio.Queue to ensure that tasks are processed in the order they arrive.

    Example:
        semaphore = FIFOSemaphore(1)
        async with semaphore:
            # Critical section - only one task at a time
            await do_work()
    """

    def __init__(self, value: int = 1):
        """
        Initialize FIFO semaphore.

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
        self._waiters: asyncio.Queue[asyncio.Future] = asyncio.Queue()
        self._lock = asyncio.Lock()

    def __repr__(self):
        """String representation of the semaphore."""
        return f"<FIFOSemaphore value={self._value}/{self._max_value}, waiters={self._waiters.qsize()}>"

    def locked(self) -> bool:
        """
        Returns True if semaphore cannot be acquired immediately.

        Returns:
            True if semaphore is at capacity (value == 0) or has waiters
        """
        return self._value == 0 or not self._waiters.empty()

    async def acquire(self) -> bool:
        """
        Acquire the semaphore.

        If the internal counter is larger than zero, decrement it and return True immediately.
        If it is zero, block and wait until a slot becomes available, maintaining FIFO order.

        Returns:
            True when semaphore is acquired
        """
        # Check if slot is available (atomic check)
        async with self._lock:
            if self._value > 0:
                # Slot available, acquire immediately
                self._value -= 1
                return True

            # No slot available, create future and add to queue
            future = asyncio.get_event_loop().create_future()
            # Put future in queue (this is synchronous, queue is thread-safe)
            self._waiters.put_nowait(future)

        # Wait for a slot to become available
        try:
            await future
            return True
        except asyncio.CancelledError:
            # If cancelled, try to remove from queue
            # Note: The future may have already been processed, so we just cancel it
            if not future.done():
                future.cancel()
            raise

    def release(self) -> None:
        """
        Release the semaphore, incrementing the internal counter by one.

        If there are waiting tasks, wake up the first one (FIFO order).
        """
        # Try to wake up a waiter (FIFO order)
        while not self._waiters.empty():
            try:
                future = self._waiters.get_nowait()
                if not future.done() and not future.cancelled():
                    # Wake up the first waiter (FIFO)
                    # Schedule the result setting in the event loop to avoid blocking
                    loop = asyncio.get_event_loop()
                    if loop.is_running():
                        loop.call_soon_threadsafe(future.set_result, True)
                    else:
                        future.set_result(True)
                    return
            except asyncio.QueueEmpty:
                break

        # No waiters or all waiters were cancelled, increment value
        if self._value < self._max_value:
            self._value += 1

    async def __aenter__(self):
        """Async context manager entry."""
        await self.acquire()
        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Async context manager exit."""
        self.release()
        return False

