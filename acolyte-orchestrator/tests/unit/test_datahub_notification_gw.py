"""Unit tests for the DataHub forwarder.

Everything DataHub needs to collapse a duplicate — the dedupe key — and
everything it needs to decide staleness — occurred_at and expires_at — is
derived from the outbox row, never from send time.
"""

from __future__ import annotations

import json
from datetime import UTC, datetime, timedelta
from uuid import UUID, uuid4

import pytest

from acolyte.domain.notification import PendingNotification
from acolyte.gateway.datahub_notification_gw import DataHubNotificationGateway
from acolyte.gen.proto.services.datahub.v1 import datahub_pb2

_USER = UUID("77777777-7777-7777-7777-777777777777")
_OCCURRED_AT = datetime(2026, 8, 8, 9, 30, tzinfo=UTC)


class _FakeClient:
    def __init__(self, *, raise_with: Exception | None = None) -> None:
        self.requests: list[datahub_pb2.EnqueueNotificationRequest] = []
        self._raise_with = raise_with

    async def enqueue_notification(
        self,
        request: datahub_pb2.EnqueueNotificationRequest,
        **_: object,
    ) -> datahub_pb2.EnqueueNotificationResponse:
        self.requests.append(request)
        if self._raise_with is not None:
            raise self._raise_with
        return datahub_pb2.EnqueueNotificationResponse(delivery_count=1)


class _FakeFactory:
    def __init__(self, client: _FakeClient) -> None:
        self._client = client
        self.gets = 0

    def get(self) -> _FakeClient:
        self.gets += 1
        return self._client


def _notification() -> PendingNotification:
    return PendingNotification(
        notification_id=uuid4(),
        dedupe_key="acolyte:9a5f",
        user_id=_USER,
        kind="acolyte_report_ready",
        payload={"kind": "acolyte_report_ready", "url": "/acolyte/reports/abc"},
        occurred_at=_OCCURRED_AT,
        attempts=1,
    )


@pytest.mark.asyncio
async def test_forward_maps_the_row_onto_the_enqueue_request() -> None:
    client = _FakeClient()
    gw = DataHubNotificationGateway(_FakeFactory(client), ttl_seconds=86400)  # type: ignore[arg-type]
    notification = _notification()

    await gw.forward(notification)

    (request,) = client.requests
    assert request.dedupe_key == "acolyte:9a5f"
    assert request.user_id == str(_USER)
    assert request.kind == "acolyte_report_ready"
    assert json.loads(request.payload) == notification.payload
    assert request.occurred_at.ToDatetime(tzinfo=UTC) == _OCCURRED_AT


@pytest.mark.asyncio
async def test_expiry_is_derived_from_business_time_not_send_time() -> None:
    client = _FakeClient()
    gw = DataHubNotificationGateway(_FakeFactory(client), ttl_seconds=3600)  # type: ignore[arg-type]

    await gw.forward(_notification())

    (request,) = client.requests
    assert request.expires_at.ToDatetime(tzinfo=UTC) == _OCCURRED_AT + timedelta(seconds=3600)


@pytest.mark.asyncio
async def test_a_transport_failure_reaches_the_relay() -> None:
    client = _FakeClient(raise_with=RuntimeError("unavailable"))
    gw = DataHubNotificationGateway(_FakeFactory(client), ttl_seconds=86400)  # type: ignore[arg-type]

    # Swallowing here would mark the row forwarded and lose the notification.
    with pytest.raises(RuntimeError, match="unavailable"):
        await gw.forward(_notification())


@pytest.mark.asyncio
async def test_the_client_is_re_fetched_per_call_so_a_rotated_cert_is_picked_up() -> None:
    client = _FakeClient()
    factory = _FakeFactory(client)
    gw = DataHubNotificationGateway(factory, ttl_seconds=86400)  # type: ignore[arg-type]

    await gw.forward(_notification())
    await gw.forward(_notification())

    assert factory.gets == 2
