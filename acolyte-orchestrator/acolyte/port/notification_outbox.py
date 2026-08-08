"""Ports for the notification outbox relay."""

from __future__ import annotations

from typing import TYPE_CHECKING, Protocol

if TYPE_CHECKING:
    from uuid import UUID

    from acolyte.domain.notification import PendingNotification


class NotificationOutboxPort(Protocol):
    """The acolyte-db side of the relay: claim, finalise, observe."""

    async def claim_batch(self, worker_id: str, limit: int) -> list[PendingNotification]: ...

    async def mark_forwarded(self, notification_id: UUID) -> None: ...

    async def mark_failed(self, notification_id: UUID, attempts: int, error: str) -> None: ...

    async def oldest_pending_age_seconds(self) -> float: ...


class NotificationForwarderPort(Protocol):
    """The outbound side: hand one notification to whoever owns delivery.

    Raises on any failure — the relay treats a raise as "retry later", so a
    forwarder that swallows its own errors silently drains the outbox.
    """

    async def forward(self, notification: PendingNotification) -> None: ...
