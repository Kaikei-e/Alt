"""collectors/base.py のテスト"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from alt_metrics.collectors.base import collect_error_trends, collect_service_stats
from alt_metrics.exceptions import CollectorError


class TestCollectServiceStats:
    """collect_service_stats関数のテスト"""

    def test_returns_list_of_dicts(self) -> None:
        """結果を辞書のリストとして返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service_name",
            "total_logs",
            "error_count",
            "warn_count",
            "error_rate",
            "last_seen",
            "minutes_since_last_log",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", 10000, 50, 100, 0.5, "2026-01-19 12:00:00", 1),
            ("auth-hub", 5000, 500, 200, 10.0, "2026-01-19 11:58:00", 2),
        ]

        result = collect_service_stats(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["service_name"] == "alt-backend"
        assert result[0]["total_logs"] == 10000
        assert result[1]["service_name"] == "auth-hub"
        assert result[1]["error_rate"] == 10.0

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_service_stats(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_service_stats(mock_client, "rask_logs", 24)

        assert "service_stats" in str(exc_info.value)
        assert "Connection failed" in str(exc_info.value)

    def test_query_uses_correct_database_and_hours(self) -> None:
        """クエリが正しいデータベースと時間を使用"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        collect_service_stats(mock_client, "custom_db", 48)

        call_args = mock_client.query.call_args[0][0]
        assert "custom_db.logs" in call_args
        assert "INTERVAL 48 HOUR" in call_args


class TestCollectErrorTrends:
    """collect_error_trends関数のテスト"""

    def test_returns_list_of_dicts(self) -> None:
        """結果を辞書のリストとして返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "hour",
            "service_name",
            "error_count",
            "total_count",
            "error_rate",
        ]
        mock_client.query.return_value.result_rows = [
            ("2026-01-19 12:00:00", "alt-backend", 10, 1000, 1.0),
            ("2026-01-19 11:00:00", "alt-backend", 5, 800, 0.625),
        ]

        result = collect_error_trends(mock_client, "rask_logs", 6)

        assert len(result) == 2
        assert result[0]["error_count"] == 10
        assert result[1]["error_rate"] == 0.625

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_error_trends(mock_client, "rask_logs", 6)

        assert "error_trends" in str(exc_info.value)
        assert "Query timeout" in str(exc_info.value)
