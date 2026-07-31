"""Consumer-Driven Contract for tag-generator <- mq-hub on alt:events:tags.

mq-hub writes this stream (usecase/generate_tags_usecase.go publishes
TagGenerationRequested to domain.StreamKeyTags and then blocks on a reply
stream), and tag-generator reads it (ConsumerConfig.tags_stream_from_env,
consumer group tag-generator-tags-group). The contract therefore belongs to
tag-generator as consumer and mq-hub as provider.

Unlike alt:events:articles, this stream still carries the article body on
purpose: ADR-000953 left it embedded because the exchange is a latency-critical
synchronous request-reply and mq-hub's caller already holds the text. The
interaction below pins the shape as it is today; it must not be thinned to the
id-only shape used for alt:events:articles.

Only the fields tag-generator reads are pinned. Pinning a field the consumer
merely binds is how a producer ends up obliged to keep shipping data nobody
uses -- the same reasoning as pre-processor's alt:events:articles pacts.
"""

import asyncio
import json
from pathlib import Path
from typing import Any
from unittest.mock import MagicMock

from pact import Pact, match

from tag_generator.handler.event_payload import TagGenerationRequestPayload
from tag_generator.stream_consumer import ConsumerConfig, StreamConsumer
from tag_generator.stream_event_handler import TagGeneratorEventHandler

PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent.parent / "pacts"

MESSAGE_ID = "1742947200000-0"


def _new_pact() -> Pact:
    return Pact("tag-generator", "mq-hub")


def _stub_service() -> MagicMock:
    """A TagGeneratorService whose extractor records what the payload gave it."""
    service = MagicMock()
    outcome = MagicMock()
    outcome.tags = ["memory-safety"]
    outcome.tag_confidences = {"memory-safety": 0.91}
    service.tag_extractor.extract_tags_with_metrics.return_value = outcome
    return service


def _redis_fields(envelope: dict[str, Any]) -> dict[str, str]:
    """Turn the logical event envelope into the Redis field map mq-hub XADDs.

    RedisDriver.eventToValues() writes payload and metadata as JSON strings and
    leaves the scalar envelope fields as plain strings; StreamConsumer._parse_event
    reads exactly that shape back. The pact body is the logical envelope, so the
    conversion has to happen here rather than in production code.
    """
    fields = {k: v for k, v in envelope.items() if k not in ("payload", "metadata")}
    for nested in ("payload", "metadata"):
        if nested in envelope:
            fields[nested] = json.dumps(envelope[nested])
    return fields


def test_consume_tag_generation_requested_event():
    """Pin the request mq-hub puts on alt:events:tags and answer it for real."""
    pact = _new_pact()
    service = _stub_service()
    replies: list[tuple[str, dict[str, Any]]] = []

    stream_consumer = MagicMock()

    async def _capture_reply(stream: str, event_data: dict[str, Any]) -> str:
        replies.append((stream, event_data))
        return MESSAGE_ID

    stream_consumer.publish_reply = _capture_reply

    handler = TagGeneratorEventHandler(service, stream_consumer)
    parser = StreamConsumer(ConsumerConfig.tags_stream_from_env(), handler)

    (
        pact.upon_receiving("a TagGenerationRequested event on alt:events:tags", "Async")
        .given("the tags stream exists")
        .with_body(
            {
                "event_id": match.string("evt-uuid-tag-001"),
                "event_type": "TagGenerationRequested",
                "source": match.string("mq-hub"),
                "created_at": match.string("2026-03-26T00:00:00.000Z"),
                "payload": {
                    "article_id": match.string("art-002"),
                    "title": match.string("Rust Memory Safety"),
                    "content": match.string("An article about memory safety in Rust programming language."),
                },
                "metadata": {
                    "reply_to": match.string("alt:replies:tags:corr-001"),
                    "correlation_id": match.string("corr-001"),
                },
            },
            "application/json",
        )
        .with_metadata({"contentType": "application/json"})
    )

    def _consume(body: str | bytes | None, _metadata: dict[str, object]) -> None:
        assert body is not None
        envelope = json.loads(body)
        event = parser._parse_event(MESSAGE_ID, _redis_fields(envelope))

        assert event.event_type == "TagGenerationRequested"
        assert event.event_id, "event_id is logged on every handled event"
        assert event.created_at is not None, "created_at must survive datetime.fromisoformat"

        asyncio.run(handler.handle_event(event))

    pact.verify(_consume, "Async")

    service.tag_extractor.extract_tags_with_metrics.assert_called_once_with(
        "Rust Memory Safety",
        "An article about memory safety in Rust programming language.",
    )

    assert len(replies) == 1, "mq-hub blocks on the reply stream; no reply is a 60s hang"
    reply_to, reply = replies[0]
    assert reply_to == "alt:replies:tags:corr-001"
    assert reply["event_type"] == "TagGenerationCompleted"
    assert reply["metadata"]["correlation_id"] == "corr-001"
    assert reply["payload"]["success"] is True
    assert reply["payload"]["article_id"] == "art-002"

    pact.write_file(PACT_DIR)


def test_tags_consumer_reads_the_stream_mq_hub_publishes_to():
    """The pact names a stream; this is what binds that name to the consumer.

    mq-hub publishes to domain.StreamKeyTags ("alt:events:tags"). Nothing else
    in tag-generator ties the contract's topic to the config the consumer
    actually starts with.
    """
    config = ConsumerConfig.tags_stream_from_env()

    assert config.stream_key == "alt:events:tags"
    assert config.group_name == "tag-generator-tags-group"


def test_tag_generation_request_payload_binds_only_contracted_fields():
    """Consumer-side half of the guarantee the pact above encodes.

    The pact says what mq-hub must send; this says what tag-generator is allowed
    to require. Two directions matter:

    - `content` must stay required. Thinning this stream to the id-only shape
      ADR-000953 applied to alt:events:articles makes every request fail
      validation and reply success=false, which is why that ADR left
      alt:events:tags alone.
    - `feed_id` must stay optional. It is bound here and read at
      stream_event_handler.py:173, but `_generate_tags_inline` never uses it
      (extraction takes title and content only), so it is deliberately absent
      from the pact. Pinning a field nobody uses obliges mq-hub to keep
      shipping it forever.
    """
    fields = TagGenerationRequestPayload.model_fields

    assert list(fields) == ["article_id", "title", "content", "feed_id"], (
        "payload bound off alt:events:tags changed; the pact in this file must be revisited"
    )
    assert [name for name, field in fields.items() if field.is_required()] == [
        "article_id",
        "title",
        "content",
    ]
