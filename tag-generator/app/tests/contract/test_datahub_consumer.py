"""Consumer-Driven Contract for tag-generator -> alt-data-hub (alt.datahub.v1).

tag-generator reaches alt_db only through Connect-RPC. Four procedures carry
the whole relationship:

  ListUntaggedArticles   the work queue (ConnectArticleFetcher.fetch_articles /
                         .count_untagged_articles)
  GetArticleContent      single-article re-fetch (ConnectArticleFetcher
                         .fetch_article_by_id)
  UpsertArticleTags      single-article write (ConnectTagInserter.upsert_tags)
  BatchUpsertArticleTags batch write (ConnectTagInserter
                         .batch_upsert_tags_no_commit)

ADR-000954 D7 moves that surface from ``services.backend.v1
.BackendInternalService`` to ``alt.datahub.v1.DataHubService``. The RPC names,
the message fields and the field numbers are identical on both sides — the
only thing that changes on the wire is the **URL path prefix** each request is
POSTed to. A rename that touches nothing but a path is exactly the kind of
change a unit test with a ``MagicMock`` client cannot see: every driver test in
tests/unit/driver/ mocks the client away and would stay green against either
namespace. So the path is what this file pins, and it is pinned by driving the
real drivers through a real Connect client rather than by asserting on a
string constant.

Provider pacticipant stays ``alt-backend``. ADR-000954 D7 defers the
pacticipant rename to a later step with a dual-publish window, because
renaming it drops the ``can-i-deploy`` verification history on the floor and
makes every existing pair look never-verified. The compose service is
``alt-data-hub``; the broker name is not, and that gap is deliberate for the
length of Wave 2.

Only the fields the drivers actually read are pinned. ``tags`` / ``language``
/ ``publishedAt`` ride on ArticleWithTags and are deliberately absent below:
tag-generator never reads them, and pinning a field a consumer merely receives
is how a provider ends up obliged to keep shipping data nobody uses.

Run with:
    cd tag-generator/app && uv run pytest tests/contract/test_datahub_consumer.py -v --no-cov
"""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import TYPE_CHECKING, Any
from unittest.mock import patch

import pytest
from pact import Pact, match

from tag_generator.driver import connect_client_factory
from tag_generator.gen.proto.alt.datahub.v1.datahub_connect import DataHubServiceClientSync

if TYPE_CHECKING:
    from tag_generator.ports import ArticleFetcherPort, TagInserterPort

PACT_DIR = Path(__file__).resolve().parent.parent.parent.parent.parent / "pacts"

# The one thing this file exists to pin. ADR-000954 D7.
DATAHUB_PREFIX = "/alt.datahub.v1.DataHubService"

# ConnectArticleFetcher._FIRST_PAGE_SENTINEL — "no cursor yet".
_FIRST_PAGE = "9999-12-31T23:59:59Z"


PACT_FILE = PACT_DIR / "tag-generator-alt-backend.json"


@pytest.fixture(scope="module", autouse=True)
def _fresh_pact_file() -> None:
    """Drop the pact file once, before the first test in this module.

    Each test below writes in merge mode so that all five interactions end up
    in one file rather than the last test clobbering the other four. Merge on
    its own never *removes* anything, though, so an interaction that gets
    renamed or deleted here would linger in the committed pact and keep being
    verified against the provider forever. Truncating first makes the file a
    function of the current test module and nothing else.
    """
    PACT_FILE.unlink(missing_ok=True)


def _new_pact() -> Pact:
    return Pact("tag-generator", "alt-backend")


class _UncompressedDataHubClient(DataHubServiceClientSync):
    """The real generated client with request gzip turned off.

    connect-python defaults ``send_compression`` to gzip, which is right in
    production — alt-data-hub is connect-go and decompresses it — but the pact
    mock server matches request bodies as text and reports a gzipped body as
    unparseable, which would mask every real mismatch behind the same error.

    Subclassing rather than hand-rolling a client is deliberate: the RPC paths
    under test are inherited from the generated ``DataHubServiceClientSync``,
    so they are the paths production sends, not a string this file made up.
    """

    def __init__(self, *args: Any, **kwargs: Any) -> None:
        kwargs.setdefault("send_compression", None)
        super().__init__(*args, **kwargs)


def _drivers(base_url: str) -> tuple[ArticleFetcherPort, TagInserterPort]:
    """Build the production drivers pointed at the pact mock server.

    Goes through the composition root rather than constructing the drivers
    directly, so a factory still wired to the legacy namespace fails here: the
    ``patch.object`` below resolves ``DataHubServiceClientSync`` by name inside
    the factory module and raises AttributeError if it is not what the factory
    imports.

    ``clear=True`` drops MTLS_ENFORCE (true by default in compose since Wave
    2-A) so the factory takes its plain-HTTP branch — the mock server speaks
    HTTP, and mTLS is not what is under test here.
    """
    with (
        patch.dict(os.environ, {"BACKEND_API_URL": base_url}, clear=True),
        patch.object(
            connect_client_factory,
            "DataHubServiceClientSync",
            _UncompressedDataHubClient,
        ),
    ):
        return connect_client_factory.create_article_fetcher_and_inserter()


def _article_body() -> dict[str, Any]:
    """ArticleWithTags, restricted to the fields ConnectArticleFetcher reads."""
    return {
        "id": match.string("art-001"),
        "title": match.string("Rust Memory Safety"),
        "content": match.string("An article about memory safety in Rust."),
        "createdAt": match.string("2026-03-26T00:00:00Z"),
        "userId": match.string("user-001"),
        "feedId": match.string("feed-001"),
    }


def test_list_untagged_articles_goes_to_the_datahub_namespace() -> None:
    """The work queue: POST {DATAHUB_PREFIX}/ListUntaggedArticles."""
    pact = _new_pact()
    (
        pact.upon_receiving("a ListUntaggedArticles request for the first page")
        .given("untagged articles exist")
        .with_request("POST", f"{DATAHUB_PREFIX}/ListUntaggedArticles")
        .with_body(json.dumps({"limit": 75}), "application/json")
        .will_respond_with(200)
        .with_body(
            {
                "articles": match.each_like(_article_body(), min=1),
                "totalCount": match.integer(42),
                "nextId": match.string("art-002"),
            },
            "application/json",
        )
    )

    with pact.serve() as srv:
        fetcher, _ = _drivers(str(srv.url))
        articles = fetcher.fetch_new_articles(None, _FIRST_PAGE, "")

    assert articles, "an empty list here means the RPC never reached the provider"
    first = articles[0]
    assert first["id"] == "art-001"
    assert first["title"] == "Rust Memory Safety"
    assert first["content"] == "An article about memory safety in Rust."
    assert first["created_at"] == "2026-03-26T00:00:00Z", "createdAt must survive the Timestamp round-trip"
    assert first["feed_id"] == "feed-001"
    assert first["user_id"] == "user-001"

    pact.write_file(PACT_DIR)


def test_count_untagged_articles_reads_total_count() -> None:
    """The scheduler's backlog probe sends limit=1 and binds totalCount only."""
    pact = _new_pact()
    (
        pact.upon_receiving("a ListUntaggedArticles backlog probe")
        .given("untagged articles exist")
        .with_request("POST", f"{DATAHUB_PREFIX}/ListUntaggedArticles")
        .with_body(json.dumps({"limit": 1}), "application/json")
        .will_respond_with(200)
        .with_body(
            {
                "articles": match.each_like(_article_body(), min=1),
                "totalCount": match.integer(42),
            },
            "application/json",
        )
    )

    with pact.serve() as srv:
        fetcher, _ = _drivers(str(srv.url))
        total = fetcher.count_untagged_articles(None)

    assert total == 42

    pact.write_file(PACT_DIR)


def test_get_article_content_goes_to_the_datahub_namespace() -> None:
    """Single-article re-fetch: POST {DATAHUB_PREFIX}/GetArticleContent."""
    pact = _new_pact()
    (
        pact.upon_receiving("a GetArticleContent request")
        .given("an article with body text exists")
        .with_request("POST", f"{DATAHUB_PREFIX}/GetArticleContent")
        .with_body(json.dumps({"articleId": "art-001"}), "application/json")
        .will_respond_with(200)
        .with_body(
            {
                "articleId": match.string("art-001"),
                "title": match.string("Rust Memory Safety"),
                "content": match.string("An article about memory safety in Rust."),
                "url": match.string("https://example.com/rust-memory-safety"),
            },
            "application/json",
        )
    )

    with pact.serve() as srv:
        fetcher, _ = _drivers(str(srv.url))
        article = fetcher.fetch_article_by_id(None, "art-001")

    # fetch_article_by_id swallows ConnectError and returns None, so `is not
    # None` is the assertion that a path mismatch cannot slip past.
    assert article is not None, "a None here means the RPC failed — check the URL path"
    assert article["id"] == "art-001"
    assert article["url"] == "https://example.com/rust-memory-safety"

    pact.write_file(PACT_DIR)


def test_upsert_article_tags_goes_to_the_datahub_namespace() -> None:
    """Single-article write: POST {DATAHUB_PREFIX}/UpsertArticleTags."""
    pact = _new_pact()
    # 0.75 / 0.5 are exactly representable in float32, so the protobuf
    # round-trip through MessageToJson reproduces them verbatim and the
    # request body below stays an exact match rather than a near-match.
    request_body = {
        "articleId": "art-001",
        "feedId": "feed-001",
        "tags": [
            {"name": "memory-safety", "confidence": 0.75},
            {"name": "rust", "confidence": 0.5},
        ],
    }
    (
        pact.upon_receiving("an UpsertArticleTags request for one article")
        .given("the article exists and has no tags")
        .with_request("POST", f"{DATAHUB_PREFIX}/UpsertArticleTags")
        .with_body(json.dumps(request_body), "application/json")
        .will_respond_with(200)
        .with_body(
            {
                "success": match.boolean(True),
                "upsertedCount": match.integer(2),
            },
            "application/json",
        )
    )

    with pact.serve() as srv:
        _, inserter = _drivers(str(srv.url))
        result = inserter.upsert_tags(
            None,
            "art-001",
            ["memory-safety", "rust"],
            "feed-001",
            {"memory-safety": 0.75, "rust": 0.5},
        )

    # upsert_tags swallows ConnectError into {"success": False, "error": ...},
    # so asserting success is what makes a path mismatch fail the test.
    assert result["success"] is True, f"upsert failed: {result.get('error')}"
    assert result["upserted_count"] == 2

    pact.write_file(PACT_DIR)


def test_batch_upsert_article_tags_goes_to_the_datahub_namespace() -> None:
    """Batch write: POST {DATAHUB_PREFIX}/BatchUpsertArticleTags."""
    pact = _new_pact()
    request_body = {
        "items": [
            {
                "articleId": "art-001",
                "feedId": "feed-001",
                "tags": [{"name": "memory-safety", "confidence": 0.75}],
            },
            {
                "articleId": "art-002",
                "feedId": "feed-002",
                "tags": [{"name": "rust", "confidence": 0.5}],
            },
        ]
    }
    (
        pact.upon_receiving("a BatchUpsertArticleTags request for two articles")
        .given("both articles exist and have no tags")
        .with_request("POST", f"{DATAHUB_PREFIX}/BatchUpsertArticleTags")
        .with_body(json.dumps(request_body), "application/json")
        .will_respond_with(200)
        .with_body(
            {
                "success": match.boolean(True),
                "totalUpserted": match.integer(2),
            },
            "application/json",
        )
    )

    with pact.serve() as srv:
        _, inserter = _drivers(str(srv.url))
        result = inserter.batch_upsert_tags_no_commit(
            None,
            [
                {
                    "article_id": "art-001",
                    "feed_id": "feed-001",
                    "tags": ["memory-safety"],
                    "tag_confidences": {"memory-safety": 0.75},
                },
                {
                    "article_id": "art-002",
                    "feed_id": "feed-002",
                    "tags": ["rust"],
                    "tag_confidences": {"rust": 0.5},
                },
            ],
        )

    # BatchResult is a TypedDict; batch_upsert_tags_no_commit turns a
    # ConnectError into success=False, so asserting success is what makes a
    # path mismatch fail the test.
    assert result["success"] is True, f"batch upsert failed: {result['errors']}"
    assert result["processed_articles"] == 2
    assert result["failed_articles"] == 0

    pact.write_file(PACT_DIR)
