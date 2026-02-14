"""Tests for ConnectArticleFetcher driver."""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest
from connectrpc.code import Code
from connectrpc.errors import ConnectError
from google.protobuf.timestamp_pb2 import Timestamp

from tag_generator.driver.connect_article_fetcher import ConnectArticleFetcher
from tag_generator.gen.proto.services.backend.v1 import internal_pb2


@pytest.fixture
def mock_client() -> MagicMock:
    return MagicMock()


@pytest.fixture
def auth_headers() -> dict[str, str]:
    return {"X-Service-Token": "test-token"}


@pytest.fixture
def fetcher(mock_client, auth_headers) -> ConnectArticleFetcher:
    return ConnectArticleFetcher(mock_client, auth_headers)


class TestFetchArticles:
    """Tests for fetch_articles method."""

    def test_calls_list_untagged_articles_with_correct_request(self, fetcher, mock_client, auth_headers) -> None:
        """Calls list_untagged_articles RPC with correct limit and offset."""
        ts = Timestamp()
        ts.FromJsonString("2024-01-15T10:00:00Z")
        article = internal_pb2.ArticleWithTags(
            id="art-1",
            title="Test",
            content="Body",
            user_id="user-1",
            created_at=ts,
        )
        mock_client.list_untagged_articles.return_value = internal_pb2.ListUntaggedArticlesResponse(
            articles=[article], total_count=1
        )

        fetcher.fetch_articles(None, "2024-01-01", "zzz", custom_batch_size=50)

        mock_client.list_untagged_articles.assert_called_once()
        call_args = mock_client.list_untagged_articles.call_args
        req = call_args[0][0]
        assert req.limit == 50
        assert req.offset == 0
        assert call_args[1]["headers"] == auth_headers
        assert call_args[1]["timeout_ms"] == 30000

    def test_converts_protobuf_article_to_dict(self, fetcher, mock_client) -> None:
        """Converts protobuf ArticleWithTags to dict matching old format."""
        ts = Timestamp()
        ts.FromJsonString("2024-06-15T12:30:00Z")
        article = internal_pb2.ArticleWithTags(
            id="art-42",
            title="Proto Article",
            content="Proto content",
            user_id="user-99",
            created_at=ts,
        )
        mock_client.list_untagged_articles.return_value = internal_pb2.ListUntaggedArticlesResponse(
            articles=[article], total_count=1
        )

        result = fetcher.fetch_articles(None, "", "")

        assert len(result) == 1
        a = result[0]
        assert a["id"] == "art-42"
        assert a["title"] == "Proto Article"
        assert a["content"] == "Proto content"
        assert a["user_id"] == "user-99"
        assert a["feed_id"] is None
        assert a["url"] == ""
        # created_at should be an ISO string from the timestamp
        assert "2024-06-15" in a["created_at"]

    def test_default_batch_size(self, fetcher, mock_client) -> None:
        """Uses default batch size of 75."""
        mock_client.list_untagged_articles.return_value = internal_pb2.ListUntaggedArticlesResponse(
            articles=[], total_count=0
        )

        fetcher.fetch_articles(None, "", "")

        req = mock_client.list_untagged_articles.call_args[0][0]
        assert req.limit == 75


class TestCountUntaggedArticles:
    """Tests for count_untagged_articles method."""

    def test_returns_total_count(self, fetcher, mock_client) -> None:
        """Returns total_count from RPC response."""
        mock_client.list_untagged_articles.return_value = internal_pb2.ListUntaggedArticlesResponse(
            articles=[], total_count=42
        )

        count = fetcher.count_untagged_articles(None)

        assert count == 42


class TestFetchArticleById:
    """Tests for fetch_article_by_id method."""

    def test_calls_get_article_content(self, fetcher, mock_client, auth_headers) -> None:
        """Calls get_article_content RPC with correct article_id."""
        mock_client.get_article_content.return_value = internal_pb2.GetArticleContentResponse(
            article_id="art-1",
            title="Title",
            content="Content",
            url="https://example.com",
        )

        result = fetcher.fetch_article_by_id(None, "art-1")

        mock_client.get_article_content.assert_called_once()
        req = mock_client.get_article_content.call_args[0][0]
        assert req.article_id == "art-1"
        assert result is not None
        assert result["id"] == "art-1"
        assert result["title"] == "Title"
        assert result["content"] == "Content"
        assert result["url"] == "https://example.com"

    def test_returns_none_on_connect_error(self, fetcher, mock_client) -> None:
        """Returns None when ConnectError is raised."""
        mock_client.get_article_content.side_effect = ConnectError(Code.NOT_FOUND, "Article not found")

        result = fetcher.fetch_article_by_id(None, "missing-id")

        assert result is None


class TestFetchArticlesByStatus:
    """Tests for fetch_articles_by_status method."""

    def test_returns_empty_for_has_tags_true(self, fetcher) -> None:
        """Returns empty list when has_tags=True."""
        result = fetcher.fetch_articles_by_status(None, has_tags=True)
        assert result == []

    def test_delegates_to_fetch_articles_for_untagged(self, fetcher, mock_client) -> None:
        """Delegates to fetch_articles when has_tags=False."""
        mock_client.list_untagged_articles.return_value = internal_pb2.ListUntaggedArticlesResponse(
            articles=[], total_count=0
        )

        fetcher.fetch_articles_by_status(None, has_tags=False, limit=10)

        mock_client.list_untagged_articles.assert_called_once()


class TestFetchLowConfidenceArticles:
    """Tests for fetch_low_confidence_articles method."""

    def test_returns_empty_list(self, fetcher) -> None:
        """Not available via API — returns empty list."""
        result = fetcher.fetch_low_confidence_articles(None)
        assert result == []
