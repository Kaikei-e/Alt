"""Minimal reproduction of HybridPrioritySemaphore.acquire return path."""

import asyncio
from collections.abc import Callable


class PrioritySemaphore:
    def __init__(
        self,
        record_queued_acquire: Callable[[str, float, bool], tuple[float, int]],
    ) -> None:
        self._record_queued_acquire = record_queued_acquire

    async def acquire(
        self,
        future: asyncio.Future[str],
        start_time: float,
        high_priority: bool,
    ):
        try:
            result = await future
            wait_time, sid = self._record_queued_acquire(
                result, start_time, high_priority
            )
        except asyncio.CancelledError:
            if not future.done():
                future.cancel()
            raise
        else:
            return wait_time, sid
