"""An ArticleCreated delivery has three outcomes, not two.

`_process_single_article` collapses "this article has no extractable tags"
(a short body, or one the sanitizer strips to nothing) and "the upsert RPC
failed" into the same `False`. The stream path then treated both as a failure
and withheld the XACK, so an article that will never yield tags was re-run
through the whole ML pipeline on every reclaim pass and dead-lettered on the
fifth -- while the batch path has always treated an empty extraction as a
normal, terminal outcome (`max_empty_extraction_retries`).

These tests drive the real consumer and the real handler, faking only the
ports, so they pin what a caller observes: which deliveries leave the PEL.
"""

import asyncio
from typing import Any, cast

import redis.asyncio as redis

from tag_extractor.extract import TagExtractionOutcome
from tag_generator.service import TagGeneratorService
from tag_generator.stream_consumer import ConsumerConfig, StreamConsumer
from tag_generator.stream_event_handler import TagGeneratorEventHandler

ARTICLE = {
    "id": "article-123",
    "title": "Test Article",
    "content": "Some content",
    "feed_id": "feed-456",
}


class _FakeFetcher:
    """Serves one article by id, the way GetArticleContent does."""

    def __init__(self, article: dict[str, Any] | None) -> None:
        self.article = article

    def fetch_article_by_id(self, conn: Any, article_id: str) -> dict[str, Any] | None:
        del conn, article_id
        return dict(self.article) if self.article is not None else None


class _FakeExtractor:
    """Returns a scripted set of tags and counts how often it was asked."""

    def __init__(self, tags: list[str]) -> None:
        self.tags = tags
        self.calls = 0

    def extract_tags_with_metrics(self, title: str, content: str) -> TagExtractionOutcome:
        self.calls += 1
        return TagExtractionOutcome(
            tags=list(self.tags),
            confidence=0.9,
            tag_count=len(self.tags),
            inference_ms=1.0,
            language="en",
            model_name="fake",
            sanitized_length=len(content) + len(title),
            tag_confidences=dict.fromkeys(self.tags, 0.9),
        )


class _FakeInserter:
    """Records upsert attempts and replays a scripted RPC outcome."""

    def __init__(self, *, success: bool = True) -> None:
        self.success = success
        self.calls: list[tuple[str, list[str], str]] = []

    def upsert_tags(
        self,
        conn: Any,
        article_id: str,
        tags: list[str],
        feed_id: str,
        tag_confidences: dict[str, float] | None = None,
    ) -> dict[str, Any]:
        del conn, tag_confidences
        self.calls.append((article_id, list(tags), feed_id))
        return {"success": self.success}


class _ScriptedReadClient:
    """Replays one XREADGROUP batch and records XACK calls."""

    def __init__(self, batches: list[list[Any]]) -> None:
        self.batches = list(batches)
        self.acks: list[str] = []

    async def xreadgroup(self, **kwargs: Any) -> list[Any]:
        del kwargs
        return self.batches.pop(0) if self.batches else []

    async def xack(self, stream: str, group: str, message_id: str) -> None:
        del stream, group
        self.acks.append(message_id)


def _service(extractor: _FakeExtractor, inserter: _FakeInserter, article: dict[str, Any] | None = ARTICLE):
    """Build a TagGeneratorService with fake ports and no ML warmup.

    `__init__` loads the real extractor, so the instance is created directly and
    only the three collaborators the article path touches are injected. The
    service's own logic stays real -- that is the point of the test.
    """
    service = object.__new__(TagGeneratorService)
    service.article_fetcher = cast(Any, _FakeFetcher(article))
    service.tag_extractor = cast(Any, extractor)
    service.tag_inserter = cast(Any, inserter)
    return service


def _delivery(message_id: str) -> list[Any]:
    return [
        (
            "alt:events:articles",
            [
                (
                    message_id,
                    {
                        "event_id": f"evt-{message_id}",
                        "event_type": "ArticleCreated",
                        "source": "alt-backend",
                        "payload": '{"article_id": "article-123", "title": "Test Article"}',
                    },
                )
            ],
        )
    ]


def _run(extractor: _FakeExtractor, inserter: _FakeInserter, message_id: str) -> _ScriptedReadClient:
    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=cast(Any, None))
    consumer.handler = TagGeneratorEventHandler(_service(extractor, inserter), consumer)
    client = _ScriptedReadClient([_delivery(message_id)])
    consumer.client = cast(redis.Redis, client)

    asyncio.run(consumer._read_and_process())
    return client


def test_article_with_no_extractable_tags_is_acked_and_never_upserted() -> None:
    """Zero tags is a terminal answer, not a failure. Withholding the XACK here
    re-ran the ML pipeline every reclaim interval and dead-lettered an article
    whose only fault is having nothing to tag.
    """
    extractor = _FakeExtractor(tags=[])
    inserter = _FakeInserter()

    client = _run(extractor, inserter, "1-0")

    assert extractor.calls == 1
    assert inserter.calls == [], "nothing to upsert when nothing was extracted"
    assert client.acks == ["1-0"], "an untaggable article must leave the PEL"


def test_article_whose_upsert_fails_is_left_pending_for_retry() -> None:
    """The other half of the split: a failed UpsertArticleTags RPC is transient,
    so the entry must keep its PEL slot for the reclaim loop.
    """
    extractor = _FakeExtractor(tags=["technology"])
    inserter = _FakeInserter(success=False)

    client = _run(extractor, inserter, "2-0")

    assert inserter.calls == [("article-123", ["technology"], "feed-456")]
    assert client.acks == [], "a failed upsert must be retried, not dropped"


def test_successfully_tagged_article_is_acked() -> None:
    extractor = _FakeExtractor(tags=["technology"])
    inserter = _FakeInserter()

    client = _run(extractor, inserter, "3-0")

    assert inserter.calls == [("article-123", ["technology"], "feed-456")]
    assert client.acks == ["3-0"]


def test_missing_article_is_acked() -> None:
    """A GetArticleContent miss was already terminal; keep it that way."""
    extractor = _FakeExtractor(tags=["technology"])
    inserter = _FakeInserter()

    consumer = StreamConsumer(ConsumerConfig(enabled=True), handler=cast(Any, None))
    consumer.handler = TagGeneratorEventHandler(_service(extractor, inserter, article=None), consumer)
    client = _ScriptedReadClient([_delivery("4-0")])
    consumer.client = cast(redis.Redis, client)

    asyncio.run(consumer._read_and_process())

    assert extractor.calls == 0
    assert client.acks == ["4-0"]
