"""Forward one outbox row to alt-data-hub's notification queue.

Everything on the wire is derived from the row: the dedupe key DataHub
de-duplicates on, the business time it records, and the expiry it measures
staleness against. Nothing is minted at send time, so a retry produces the same
request.
"""

from __future__ import annotations

import json
from datetime import timedelta
from typing import TYPE_CHECKING

from google.protobuf.timestamp_pb2 import Timestamp

from acolyte.gen.proto.services.datahub.v1 import datahub_pb2

if TYPE_CHECKING:
    from acolyte.domain.notification import PendingNotification
    from acolyte.driver.datahub_client import DataHubClientFactory


class DataHubNotificationGateway:
    """NotificationForwarderPort over services.datahub.v1.DataHubService."""

    def __init__(self, client_factory: DataHubClientFactory, *, ttl_seconds: int) -> None:
        self._client_factory = client_factory
        self._ttl_seconds = ttl_seconds

    async def forward(self, notification: PendingNotification) -> None:
        occurred_at = Timestamp()
        occurred_at.FromDatetime(notification.occurred_at)
        expires_at = Timestamp()
        expires_at.FromDatetime(notification.occurred_at + timedelta(seconds=self._ttl_seconds))

        request = datahub_pb2.EnqueueNotificationRequest(
            dedupe_key=notification.dedupe_key,
            user_id=str(notification.user_id),
            kind=notification.kind,
            payload=json.dumps(notification.payload, separators=(",", ":"), sort_keys=True).encode(),
            occurred_at=occurred_at,
            expires_at=expires_at,
        )
        # get() per call so a leaf rotated by pki-agent is picked up without a
        # restart; any transport or Connect error propagates to the relay,
        # which reschedules the row.
        await self._client_factory.get().enqueue_notification(request)
