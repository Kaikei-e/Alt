"""Relay the notification outbox to alt-data-hub.

Claim → commit the claim → call out → finalise. At-least-once on purpose:
idempotency lives downstream in the dedupe key, so a crash between the call and
the mark costs a duplicate enqueue that DataHub collapses, not a lost
notification.
"""

from __future__ import annotations

import asyncio
import time
from typing import TYPE_CHECKING

import structlog

if TYPE_CHECKING:
    from acolyte.port.notification_outbox import (
        NotificationForwarderPort,
        NotificationOutboxPort,
    )

logger = structlog.get_logger(__name__)


class RelayNotificationsUsecase:
    """Forwards claimed outbox rows and records what happened to each."""

    def __init__(
        self,
        outbox: NotificationOutboxPort,
        forwarder: NotificationForwarderPort,
        *,
        worker_id: str,
        batch_size: int,
    ) -> None:
        self._outbox = outbox
        self._forwarder = forwarder
        self._worker_id = worker_id
        self._batch_size = batch_size

    async def tick(self) -> int:
        """Drain one batch. Returns how many rows were forwarded."""
        batch = await self._outbox.claim_batch(self._worker_id, self._batch_size)
        forwarded = 0
        for notification in batch:
            try:
                await self._forwarder.forward(notification)
            except Exception as exc:  # noqa: BLE001 — any outbound failure is a retry, and the reason is recorded on the row
                logger.warning(
                    "notification_forward_failed",
                    notification_id=str(notification.notification_id),
                    dedupe_key=notification.dedupe_key,
                    attempts=notification.attempts,
                    error=str(exc),
                )
                await self._outbox.mark_failed(notification.notification_id, notification.attempts, str(exc))
            else:
                await self._outbox.mark_forwarded(notification.notification_id)
                forwarded += 1

        oldest_pending_age = await self._outbox.oldest_pending_age_seconds()
        # This log line is the whole observability surface of the relay: this
        # service publishes no /metrics endpoint (the listener authenticates
        # nobody, so every route on it is public) and wires no OTLP metrics
        # exporter, so the two fields below reach an operator only through the
        # rask log pipeline. Emitted last and on every completed tick, including
        # ticks that moved nothing — a liveness signal that stops being written,
        # or that a half-failed tick still advances, cannot show a wedged relay.
        logger.info(
            "notification_relay_tick",
            forwarded=forwarded,
            notification_outbox_oldest_pending_age_seconds=oldest_pending_age,
            notification_outbox_last_tick_timestamp_seconds=time.time(),
        )
        return forwarded

    async def run_forever(self, interval_seconds: float) -> None:
        """Poll until cancelled. A failed tick is logged and retried next round —
        the rows it claimed come back on their own via the lease."""
        while True:
            try:
                await self.tick()
            except asyncio.CancelledError:
                raise
            except Exception:  # noqa: BLE001 — the loop outlives any single tick failure (DB blip, DataHub outage)
                logger.warning("notification_relay_tick_failed", exc_info=True)
            await asyncio.sleep(interval_seconds)
