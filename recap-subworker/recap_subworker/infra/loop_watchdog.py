"""Event-loop watchdog using faulthandler's GIL-independent C timer."""

from __future__ import annotations

import asyncio
import contextlib
import faulthandler
import sys
from collections.abc import Callable, Coroutine
from typing import Any

import structlog

logger = structlog.get_logger(__name__)

# WHY: faulthandler's timer runs in an OS C thread independent of the Python GIL.
# A Python watchdog thread cannot fire when native C/Fortran code freezes while holding the GIL.


def _default_arm(timeout_seconds: float) -> None:
    faulthandler.dump_traceback_later(
        timeout_seconds,
        repeat=False,
        file=sys.stderr,
        exit=True,
    )


def _default_cancel() -> None:
    faulthandler.cancel_dump_traceback_later()


class LoopWatchdog:
    """Event-loop watchdog that re-arms a GIL-independent faulthandler timer."""

    def __init__(
        self,
        timeout_seconds: float,
        *,
        arm_fn: Callable[[float], None] = _default_arm,
        cancel_fn: Callable[[], None] = _default_cancel,
        sleep_fn: Callable[[float], Coroutine[Any, Any, None]] = asyncio.sleep,
    ) -> None:
        self._timeout_seconds = timeout_seconds
        self._interval_seconds = timeout_seconds / 4
        self._arm_fn = arm_fn
        self._cancel_fn = cancel_fn
        self._sleep_fn = sleep_fn
        self._task: asyncio.Task[None] | None = None

    def start(self) -> None:
        """Start the watchdog task re-arming the timer periodically."""
        if self._task is None:
            self._arm_fn(self._timeout_seconds)
            self._task = asyncio.create_task(self._rearm_loop(), name="loop-watchdog")
            self._task.add_done_callback(self._on_task_done)

    def _on_task_done(self, task: asyncio.Task[None]) -> None:
        if task.cancelled():
            return
        exc = task.exception()
        if exc is not None:
            logger.critical("loop_watchdog_task_failed", exc_info=exc)

    async def _rearm_loop(self) -> None:
        try:
            while True:
                await self._sleep_fn(self._interval_seconds)
                self._arm_fn(self._timeout_seconds)
        except asyncio.CancelledError:
            self._cancel_fn()
            raise

    async def stop(self) -> None:
        """Stop the watchdog task and cancel any pending faulthandler timer."""
        if self._task is not None:
            task = self._task
            self._task = None
            task.cancel()
            with contextlib.suppress(asyncio.CancelledError):
                await task
        else:
            self._cancel_fn()
