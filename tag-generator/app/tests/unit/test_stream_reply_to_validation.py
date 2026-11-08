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

    def __init__(
        self,
        batches: list[list[Any]] | None = None,
        *,
        xadd_errors: list[Exception | None] | None = None,
    ) -> None:
        self.batches = list(batches or [])
        self.acks: list[str] = []
        self.xadds: list[str] = []
        self.xadd_attempts = 0
        self.xadd_errors = list(xadd_errors or [])

    async def xreadgroup(self, **kwargs: Any) -> list[Any]:
        del kwargs
        return self.batches.pop(0) if self.batches else []

    async def xack(self, stream: str, group: str, message_id: str) -> None:
        del stream, group
        self.acks.append(message_id)

    async def xadd(self, stream: str, fields: dict[Any, Any], **kwargs: Any) -> str:
        del fields, kwargs
        self.xadd_attempts += 1
        error = self.xadd_errors.pop(0) if self.xadd_errors else None
        if error is not None:
            raise error
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


def test_reply_xadd_failure_leaves_request_unacked() -> None:
    """When XADD to the reply stream fails (e.g. Redis OOM under noeviction),
    the TagGenerationRequested message must stay unacked in the PEL for retry.
    """
    oom = redis.ResponseError("OOM command not allowed when used memory > 'maxmemory'.")
    service = _service()
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=cast(Any, None))
    consumer.handler = TagGeneratorEventHandler(service, consumer)
    client = _RecordingClient([_request("3-0", VALID_REPLY_TO)], xadd_errors=[oom])
    consumer.client = cast(redis.Redis, client)

    asyncio.run(consumer._read_and_process())

    assert client.xadd_attempts == 1
    assert client.xadds == []
    assert client.acks == [], "a failed reply XADD must not ACK the request message"


def test_unwired_reply_publisher_leaves_request_unacked() -> None:
    """When TagGeneratorEventHandler has stream_consumer=None, receiving a
    TagGenerationRequested with a valid reply_to must fail loudly so
    StreamConsumer leaves the request unacked in the PEL.
    """
    service = _service()
    handler = TagGeneratorEventHandler(service, stream_consumer=None)
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=handler)
    client = _RecordingClient([_request("4-0", VALID_REPLY_TO)])
    consumer.client = cast(redis.Redis, client)

    asyncio.run(consumer._read_and_process())

    assert client.xadds == []
    assert client.acks == [], "an unwired reply publisher must not ACK the request message"
    service.tag_extractor.extract_tags_with_metrics.assert_not_called()


def test_main_logs_tags_stream_consumer_disabled_when_disabled(monkeypatch) -> None:
    """When the dedicated tags stream consumer is disabled, main() must emit an
    explicit startup log of the disabled wiring state.
    """
    import main as main_mod

    logged_events: list[str] = []

    class _FakeLogger:
        def info(self, event: str, **_kwargs: Any) -> None:
            logged_events.append(event)

        def error(self, event: str, **_kwargs: Any) -> None:
            logged_events.append(event)

    class _FakeService:
        def __init__(self, _config: object = None) -> None:
            pass

        def run_service(self) -> None:
            pass

    monkeypatch.setattr(main_mod, "logger", _FakeLogger())
    monkeypatch.setattr(main_mod, "TagGeneratorService", _FakeService)
    monkeypatch.setattr(main_mod, "otel_shutdown", lambda: None)
    monkeypatch.setattr(
        main_mod.ConsumerConfig,
        "from_env",
        classmethod(lambda cls: ConsumerConfig(enabled=False)),
    )
    monkeypatch.setattr(
        main_mod.ConsumerConfig,
        "tags_stream_from_env",
        classmethod(lambda cls: ConsumerConfig(enabled=False, stream_key="alt:events:tags")),
    )

    rc = main_mod.main()

    assert rc == 0
    assert "redis_streams_consumer_disabled" in logged_events
    assert "tags_stream_consumer_disabled" in logged_events
