"""Tests for ConnectTagInserter driver."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError

from tag_generator.driver.connect_tag_inserter import ConnectTagInserter
from tag_generator.gen.proto.services.backend.v1 import internal_pb2


@pytest.fixture
def mock_client() -> MagicMock:
    return MagicMock()


@pytest.fixture
def auth_headers() -> dict[str, str]:
    return {"X-Service-Token": "test-token"}


@pytest.fixture
def inserter(mock_client, auth_headers) -> ConnectTagInserter:
    return ConnectTagInserter(mock_client, auth_headers)


class TestUpsertTags:
    """Tests for upsert_tags method."""

    def test_calls_upsert_article_tags_with_correct_request(self, inserter, mock_client, auth_headers) -> None:
        """Calls upsert_article_tags RPC with correct TagItems."""
        mock_client.upsert_article_tags.return_value = internal_pb2.UpsertArticleTagsResponse(
            success=True, upserted_count=3
        )

        inserter.upsert_tags(
            None,
            "art-1",
            ["python", "rust", "go"],
            "feed-1",
            tag_confidences={"python": 0.9, "rust": 0.8},
        )

        mock_client.upsert_article_tags.assert_called_once()
        call_args = mock_client.upsert_article_tags.call_args
        req = call_args[0][0]

        assert req.article_id == "art-1"
        assert req.feed_id == "feed-1"
        assert len(req.tags) == 3
        # Check tag items
        tag_map = {t.name: t.confidence for t in req.tags}
        assert tag_map["python"] == pytest.approx(0.9)
        assert tag_map["rust"] == pytest.approx(0.8)
        assert tag_map["go"] == pytest.approx(0.5)  # default confidence

        assert call_args[1]["headers"] == auth_headers
        assert call_args[1]["timeout_ms"] == 30000

    def test_maps_success_and_upserted_count(self, inserter, mock_client) -> None:
        """Maps response success and upserted_count correctly."""
        mock_client.upsert_article_tags.return_value = internal_pb2.UpsertArticleTagsResponse(
            success=True, upserted_count=5
        )

        result = inserter.upsert_tags(None, "art-1", ["tag"], "feed-1")

        assert result["success"] is True
        assert result["upserted_count"] == 5

    def test_returns_error_on_connect_error(self, inserter, mock_client) -> None:
        """Returns error dict when ConnectError is raised."""
        mock_client.upsert_article_tags.side_effect = ConnectError(Code.INTERNAL, "DB error")

        result = inserter.upsert_tags(None, "art-1", ["tag"], "feed-1")

        assert result["success"] is False
        assert "error" in result


class TestBatchUpsertTagsNoCommit:
    """Tests for batch_upsert_tags_no_commit method."""

    def test_calls_batch_upsert_with_correct_items(self, inserter, mock_client, auth_headers) -> None:
        """Calls batch_upsert_article_tags RPC with correct batch items."""
        mock_client.batch_upsert_article_tags.return_value = internal_pb2.BatchUpsertArticleTagsResponse(
            success=True, total_upserted=4
        )

        article_tags = [
            {
                "article_id": "art-1",
                "feed_id": "feed-1",
                "tags": ["python", "go"],
                "tag_confidences": {"python": 0.9},
            },
            {
                "article_id": "art-2",
                "feed_id": "feed-2",
                "tags": ["rust"],
                "tag_confidences": {},
            },
        ]

        inserter.batch_upsert_tags_no_commit(None, article_tags)

        mock_client.batch_upsert_article_tags.assert_called_once()
        call_args = mock_client.batch_upsert_article_tags.call_args
        req = call_args[0][0]

        assert len(req.items) == 2
        assert req.items[0].article_id == "art-1"
        assert req.items[0].feed_id == "feed-1"
        assert len(req.items[0].tags) == 2
        assert req.items[1].article_id == "art-2"

        assert call_args[1]["headers"] == auth_headers

    def test_returns_batch_result_on_success(self, inserter, mock_client) -> None:
        """Returns BatchResult with correct fields on success."""
        mock_client.batch_upsert_article_tags.return_value = internal_pb2.BatchUpsertArticleTagsResponse(
            success=True, total_upserted=2
        )

        article_tags = [
            {"article_id": "a1", "feed_id": "f1", "tags": ["t1"], "tag_confidences": {}},
            {"article_id": "a2", "feed_id": "f2", "tags": ["t2"], "tag_confidences": {}},
        ]

        result = inserter.batch_upsert_tags_no_commit(None, article_tags)

        assert result["success"] is True
        assert result["processed_articles"] == 2
        assert result["failed_articles"] == 0
        assert result["errors"] == []

    def test_returns_error_batch_result_on_connect_error(self, inserter, mock_client) -> None:
        """Returns error BatchResult when ConnectError is raised."""
        mock_client.batch_upsert_article_tags.side_effect = ConnectError(Code.UNAVAILABLE, "Service unavailable")

        article_tags = [
            {"article_id": "a1", "feed_id": "f1", "tags": ["t1"], "tag_confidences": {}},
        ]

        result = inserter.batch_upsert_tags_no_commit(None, article_tags)

        assert result["success"] is False
        assert result["failed_articles"] == 1
        assert len(result["errors"]) == 1


class TestBatchUpsertTagsWithComparison:
    """Tests for batch_upsert_tags_with_comparison method."""

    def test_delegates_to_batch_upsert_no_commit(self, inserter, mock_client) -> None:
        """Delegates to batch_upsert_tags_no_commit in API mode."""
        mock_client.batch_upsert_article_tags.return_value = internal_pb2.BatchUpsertArticleTagsResponse(
            success=True, total_upserted=1
        )

        article_tags = [
            {"article_id": "a1", "feed_id": "f1", "tags": ["t1"], "tag_confidences": {}},
        ]

        result = inserter.batch_upsert_tags_with_comparison(None, article_tags)

        assert result["success"] is True
        mock_client.batch_upsert_article_tags.assert_called_once()
