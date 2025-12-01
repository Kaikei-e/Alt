from unittest.mock import MagicMock, Mock

import pytest

from article_fetcher.fetch import ArticleFetcher


class TestArticleFetcherForward:
    def test_fetch_new_articles_uses_forward_cursor(self):
        fetcher = ArticleFetcher()
        mock_conn = MagicMock()
        mock_cursor = mock_conn.cursor.return_value.__enter__.return_value
        mock_cursor.fetchall.return_value = [
            {
                "id": "next-id",
                "title": "Title",
                "content": "Body",
                "created_at": "2024-01-02T00:00:00Z",
                "feed_id": None,
                "url": None,
            }
        ]

        articles = fetcher.fetch_new_articles(mock_conn, "2024-01-01T00:00:00Z", "base-id", 10)

        assert len(articles) == 1
        mock_conn.cursor.assert_called_once()
        mock_cursor.execute.assert_called_once()
        call_args = mock_cursor.execute.call_args[0]

        assert "created_at > %s" in call_args[0]
        assert call_args[1][0] == "2024-01-01T00:00:00Z"
        assert call_args[1][1] == "2024-01-01T00:00:00Z"
        assert call_args[1][2] == "base-id"
        assert call_args[1][3] == 10

    def test_fetch_new_articles_validates_inputs(self):
        fetcher = ArticleFetcher()
        mock_conn = Mock()

        with pytest.raises(ValueError):
            fetcher.fetch_new_articles(mock_conn, "", "id-1")
