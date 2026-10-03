"""A forged reply_to must never reach XADD.

The reply stream name arrives in the event metadata, so a forged
TagGenerationRequested could name alt:events:articles and have tag-generator
XADD into it with MAXLEN ~1000, trimming the live stream. Both the handler and
StreamConsumer.publish_reply refuse anything mq-hub could not have generated;
the consumer still ACKs the delivery so the poison message leaves the PEL.
"""

import asyncio
import json
from typing import Any, cast
from unittest.mock import MagicMock

import redis.asyncio as redis

from tag_generator.stream_consumer import ConsumerConfig, StreamConsumer
from tag_generator.stream_event_handler import TagGeneratorEventHandler

VALID_REPLY_TO = "alt:replies:tags:0f8e3c1a-5b2d-4c7e-9a10-3d4f5e6a7b8c"


class _RecordingClient:
    """Replays one XREADGROUP batch and records XACK / XADD calls."""

    def __init__(self, batches: list[list[Any]] | None = None) -> None:
        self.batches = list(batches or [])
        self.acks: list[str] = []
        self.xadds: list[str] = []

    async def xreadgroup(self, **kwargs: Any) -> list[Any]:
        del kwargs
        return self.batches.pop(0) if self.batches else []

    async def xack(self, stream: str, group: str, message_id: str) -> None:
        del stream, group
        self.acks.append(message_id)

    async def xadd(self, stream: str, fields: dict[Any, Any], **kwargs: Any) -> str:
        del fields, kwargs
        self.xadds.append(stream)
        return "9-0"


def _request(message_id: str, reply_to: str) -> list[Any]:
    return [
        (
            "alt:events:tags",
            [
                (
                    message_id,
                    {
                        "event_id": f"evt-{message_id}",
                        "event_type": "TagGenerationRequested",
                        "source": "mq-hub",
                        "payload": json.dumps({"article_id": "art-1", "title": "Title", "content": "Content"}),
                        "metadata": json.dumps({"correlation_id": "corr-1", "reply_to": reply_to}),
                    },
                )
            ],
        )
    ]


def _service() -> MagicMock:
    service = MagicMock()
    outcome = MagicMock()
    outcome.tags = ["rust"]
    outcome.tag_confidences = {"rust": 0.9}
    service.tag_extractor.extract_tags_with_metrics.return_value = outcome
    return service


def _deliver(reply_to: str, message_id: str) -> tuple[_RecordingClient, MagicMock]:
    service = _service()
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=cast(Any, None))
    consumer.handler = TagGeneratorEventHandler(service, consumer)
    client = _RecordingClient([_request(message_id, reply_to)])
    consumer.client = cast(redis.Redis, client)
    asyncio.run(consumer._read_and_process())
    return client, service


def test_forged_reply_to_is_acked_without_any_xadd() -> None:
    client, service = _deliver("alt:events:articles", "1-0")

    assert client.xadds == [], "a forged reply_to must never be written to"
    assert client.acks == ["1-0"], "the poison request must leave the PEL"
    service.tag_extractor.extract_tags_with_metrics.assert_not_called()


def test_mq_hub_reply_stream_gets_the_reply() -> None:
    client, _ = _deliver(VALID_REPLY_TO, "2-0")

    assert client.xadds == [VALID_REPLY_TO]
    assert client.acks == ["2-0"]


def test_publish_reply_refuses_a_non_reply_stream_key() -> None:
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=cast(Any, None))
    client = _RecordingClient()
    consumer.client = cast(redis.Redis, client)

    result = asyncio.run(consumer.publish_reply("alt:events:articles", {"payload": {}}))

    assert result is None
    assert client.xadds == []


def test_publish_reply_writes_to_an_mq_hub_reply_stream() -> None:
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=cast(Any, None))
    client = _RecordingClient()
    consumer.client = cast(redis.Redis, client)

    result = asyncio.run(consumer.publish_reply(VALID_REPLY_TO, {"payload": {}}))

    assert result == "9-0"
    assert client.xadds == [VALID_REPLY_TO]
