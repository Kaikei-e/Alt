"""Cancellation + deadline for enrollment HTTP and the renewal loop."""

from __future__ import annotations

import threading
import time


class CancelledError(Exception):
    """Enrollment context was canceled or its deadline elapsed."""


class Ctx:
    """Mirrors Go context.Context for timeouts and cancel without asyncio."""

    def __init__(self, *, timeout: float | None = None) -> None:
        self._cancel = threading.Event()
        self._deadline: float | None = (
            time.monotonic() + timeout if timeout is not None else None
        )

    def cancel(self) -> None:
        self._cancel.set()

    def cancelled(self) -> bool:
        if self._cancel.is_set():
            return True
        return self._deadline is not None and time.monotonic() >= self._deadline

    def raise_if_cancelled(self) -> None:
        if not self.cancelled():
            return
        if self._cancel.is_set():
            raise CancelledError("canceled") from None
        raise CancelledError("timed out") from None

    def remaining(self) -> float | None:
        if self._deadline is None:
            return None
        return max(0.0, self._deadline - time.monotonic())
