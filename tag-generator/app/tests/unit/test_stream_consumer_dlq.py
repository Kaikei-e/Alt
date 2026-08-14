"""The reclaim sweep must survive a Redis that rejects the dead-letter write.

`_get_delivery_count` and `_move_to_dlq` ran outside the per-message try, so a
single failing entry unwound `_reclaim_idle_messages` and the healthy entries
behind it in the same batch waited another interval for a redelivery the sweep
exists to give them. The failure is not hypothetical: redis-streams runs at
maxmemory 1gb under noeviction and XADD is denyoom, so the DLQ write is exactly
the call that starts failing first -- which is also why the DLQ stream itself
has to be capped rather than growing until it locks every producer out.
"""

import asyncio
from typing import Any, cast

import redis.asyncio as redis

from tag_generator.stream_consumer import ConsumerConfig, EventHandler, StreamConsumer

OOM = redis.ResponseError("OOM command not allowed when used memory > 'maxmemory'.")


class _RecordingHandler(EventHandler):
    def __init__(self) -> None:
        self.events: list[Any] = []

    async def handle_event(self, event: Any) -> None:
        self.events.append(event)


class _DlqClient:
    """Serves one scripted XAUTOCLAIM batch per sweep and records DLQ traffic."""

    def __init__(
        self,
        entries: list[tuple[str, dict[str, str]]],
        times_delivered: dict[str, int],
        *,
        xadd_errors: list[Exception | None] | None = None,
        xack_errors: list[Exception | None] | None = None,
        xpending_errors: dict[str, Exception] | None = None,
    ) -> None:
        self.entries = entries
        self.times_delivered = times_delivered
        self.xadd_errors = list(xadd_errors or [])
        self.xack_errors = list(xack_errors or [])
        self.xpending_errors = xpending_errors or {}
        self.xadds: list[tuple[str, dict[str, Any], dict[str, Any]]] = []
        self.acks: list[str] = []

    async def xautoclaim(self, **kwargs: Any) -> tuple[str, list[Any], int]:
        del kwargs
        return ("0-0", list(self.entries), 0)

    async def xpending_range(self, *args: Any, **kwargs: Any) -> list[Any]:
        del args
        message_id = str(kwargs["min"])
        error = self.xpending_errors.get(message_id)
        if error is not None:
            raise error
        return [{"times_delivered": self.times_delivered.get(message_id, 1)}]

    async def xadd(self, name: str, fields: dict[str, Any], **kwargs: Any) -> str:
        error = self.xadd_errors.pop(0) if self.xadd_errors else None
        if error is not None:
            raise error
        self.xadds.append((name, fields, kwargs))
        return f"dlq-{len(self.xadds)}"

    async def xack(self, stream: str, group: str, message_id: str) -> None:
        del stream, group
        error = self.xack_errors.pop(0) if self.xack_errors else None
        if error is not None:
            raise error
        self.acks.append(message_id)


def _entry(message_id: str) -> tuple[str, dict[str, str]]:
    return (
        message_id,
        {
            "event_id": f"evt-{message_id}",
            "event_type": "ArticleCreated",
            "source": "alt-backend",
            "payload": '{"article_id": "article-123"}',
        },
    )


def _consumer(client: _DlqClient) -> tuple[StreamConsumer, _RecordingHandler]:
    handler = _RecordingHandler()
    consumer = StreamConsumer(ConsumerConfig(enabled=True, max_delivery_count=5), handler)
    consumer.client = cast(redis.Redis, client)
    return consumer, handler


def test_a_rejected_dlq_write_does_not_abort_the_sweep() -> None:
    """One entry Redis refuses to dead-letter must not cost the rest of the
    batch their redelivery -- and it must keep its own PEL slot so a later
    sweep can dead-letter it once Redis accepts writes again.
    """
    client = _DlqClient(
        entries=[_entry("1-0"), _entry("2-0")],
        times_delivered={"1-0": 99, "2-0": 1},
        xadd_errors=[OOM],
    )
    consumer, handler = _consumer(client)

    asyncio.run(consumer._reclaim_idle_messages())

    assert [e.message_id for e in handler.events] == ["2-0"], "the healthy entry must still be reprocessed"
    assert client.acks == ["2-0"], "only the reprocessed entry leaves the PEL"


def test_a_failing_pel_lookup_does_not_abort_the_sweep() -> None:
    """Same obligation one call earlier: XPENDING is Redis traffic too."""
    client = _DlqClient(
        entries=[_entry("1-0"), _entry("2-0")],
        times_delivered={"2-0": 1},
        xpending_errors={"1-0": redis.TimeoutError("Timeout reading from socket")},
    )
    consumer, handler = _consumer(client)

    asyncio.run(consumer._reclaim_idle_messages())

    assert [e.message_id for e in handler.events] == ["2-0"]
    assert client.acks == ["2-0"]


def test_dlq_copy_is_not_duplicated_when_the_ack_fails() -> None:
    """XADD landing and XACK failing leaves the entry in the PEL, so the next
    sweep dead-letters it again -- piling duplicate copies of the same payload
    into the stream every reclaim interval. The copy is durable after the first
    write; only the ACK is owed.
    """
    client = _DlqClient(
        entries=[_entry("1-0")],
        times_delivered={"1-0": 99},
        xack_errors=[OOM],
    )
    consumer, _handler = _consumer(client)

    asyncio.run(consumer._reclaim_idle_messages())
    assert len(client.xadds) == 1
    assert client.acks == []

    asyncio.run(consumer._reclaim_idle_messages())

    assert len(client.xadds) == 1, "the DLQ copy is already durable; do not write it again"
    assert client.acks == ["1-0"], "the retried ACK must free the PEL slot"


def test_dlq_stream_is_capped() -> None:
    """An uncapped DLQ is a slow-motion outage: redis-streams is maxmemory /
    noeviction and nothing consumes or trims these streams, so once one fills
    the instance every producer's XADD starts being refused, not just ours.
    The reply publisher already caps its own stream the same way.
    """
    client = _DlqClient(entries=[_entry("1-0")], times_delivered={"1-0": 99})
    consumer, _handler = _consumer(client)

    asyncio.run(consumer._reclaim_idle_messages())

    stream, _fields, kwargs = client.xadds[0]
    assert stream.endswith(":dlq")
    assert kwargs.get("approximate") is True, "an exact trim on every write is not worth the cost"
    maxlen = kwargs.get("maxlen")
    assert isinstance(maxlen, int) and 0 < maxlen <= 10_000, "the DLQ must be bounded"
