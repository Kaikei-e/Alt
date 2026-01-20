"""collectors/logs.py のテスト"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from alt_metrics.collectors.logs import (
    collect_error_types,
    collect_log_severity_distribution,
    collect_log_volume_trends,
    collect_recent_errors,
)
from alt_metrics.exceptions import CollectorError


class TestCollectErrorTypes:
    """collect_error_types関数のテスト"""

    def test_returns_error_type_data(self) -> None:
        """エラー種類データを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "error_type",
            "error_count",
            "sample_message",
        ]
        mock_client.query.return_value.result_rows = [
            ("auth-hub", "AuthenticationError", 100, "Invalid token"),
            ("alt-backend", "DatabaseError", 50, "Connection refused"),
        ]

        result = collect_error_types(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["error_type"] == "AuthenticationError"
        assert result[0]["error_count"] == 100
        assert result[1]["service"] == "alt-backend"

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_error_types(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_error_types(mock_client, "rask_logs", 24)

        assert "error_types" in str(exc_info.value)


class TestCollectRecentErrors:
    """collect_recent_errors関数のテスト"""

    def test_returns_recent_errors(self) -> None:
        """最新エラーを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "level",
            "message",
            "error_type",
            "timestamp",
        ]
        mock_client.query.return_value.result_rows = [
            ("auth-hub", "ERROR", "Login failed", "AuthError", "2026-01-19 12:00:00"),
            ("alt-backend", "ERROR", "DB timeout", "DBError", "2026-01-19 11:55:00"),
        ]

        result = collect_recent_errors(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["service"] == "auth-hub"
        assert result[0]["level"] == "ERROR"
        assert "2026-01-19" in result[0]["timestamp"]

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_recent_errors(mock_client, "rask_logs", 24)

        assert "recent_errors" in str(exc_info.value)


class TestCollectLogSeverityDistribution:
    """collect_log_severity_distribution関数のテスト"""

    def test_returns_severity_distribution(self) -> None:
        """重要度分布を返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "total_logs",
            "debug_count",
            "info_count",
            "warn_count",
            "error_count",
            "fatal_count",
            "error_rate",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", 10000, 1000, 8000, 500, 400, 100, 5.0),
            ("auth-hub", 5000, 500, 4000, 200, 250, 50, 6.0),
        ]

        result = collect_log_severity_distribution(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["total_logs"] == 10000
        assert result[0]["error_count"] == 400
        assert result[0]["error_rate"] == 5.0

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_log_severity_distribution(mock_client, "rask_logs", 24)

        assert "log_severity_distribution" in str(exc_info.value)


class TestCollectLogVolumeTrends:
    """collect_log_volume_trends関数のテスト"""

    def test_returns_volume_trends(self) -> None:
        """ログ量トレンドを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "hour",
            "service",
            "log_count",
            "error_count",
            "error_rate",
        ]
        mock_client.query.return_value.result_rows = [
            ("2026-01-19 12:00:00", "alt-backend", 5000, 50, 1.0),
            ("2026-01-19 11:00:00", "alt-backend", 4500, 45, 1.0),
        ]

        result = collect_log_volume_trends(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["log_count"] == 5000
        assert result[0]["error_rate"] == 1.0

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_log_volume_trends(mock_client, "rask_logs", 24)

        assert "log_volume_trends" in str(exc_info.value)
