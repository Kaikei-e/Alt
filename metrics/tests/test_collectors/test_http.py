"""collectors/http.py のテスト"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from alt_metrics.collectors.http import (
    collect_http_endpoint_stats,
    collect_http_status_distribution,
)
from alt_metrics.exceptions import CollectorError


class TestCollectHttpEndpointStats:
    """collect_http_endpoint_stats関数のテスト"""

    def test_returns_endpoint_stats(self) -> None:
        """エンドポイント統計を返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "route",
            "request_count",
            "avg_duration_ms",
            "p95_duration_ms",
            "avg_response_size",
            "error_rate",
            "status_2xx",
            "status_4xx",
            "status_5xx",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", "/api/feeds", 1000, 50.0, 100.0, 1024, 0.5, 990, 5, 5),
            ("alt-backend", "/api/health", 500, 10.0, 20.0, 256, 0.0, 500, 0, 0),
        ]

        result = collect_http_endpoint_stats(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["route"] == "/api/feeds"
        assert result[0]["request_count"] == 1000
        assert result[0]["p95_duration_ms"] == 100.0
        assert result[1]["error_rate"] == 0.0

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_http_endpoint_stats(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_http_endpoint_stats(mock_client, "rask_logs", 24)

        assert "http_endpoint_stats" in str(exc_info.value)

    def test_query_uses_correct_parameters(self) -> None:
        """クエリが正しいパラメータを使用"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        collect_http_endpoint_stats(mock_client, "custom_db", 48)

        call_args = mock_client.query.call_args[0][0]
        assert "custom_db.otel_http_requests" in call_args
        assert "INTERVAL 48 HOUR" in call_args


class TestCollectHttpStatusDistribution:
    """collect_http_status_distribution関数のテスト"""

    def test_returns_status_distribution(self) -> None:
        """ステータス分布を返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "total_requests",
            "status_2xx",
            "status_3xx",
            "status_4xx",
            "status_5xx",
            "error_5xx_rate",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", 10000, 9500, 100, 300, 100, 1.0),
            ("auth-hub", 5000, 4800, 50, 100, 50, 1.0),
        ]

        result = collect_http_status_distribution(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["service"] == "alt-backend"
        assert result[0]["total_requests"] == 10000
        assert result[0]["status_5xx"] == 100
        assert result[0]["error_5xx_rate"] == 1.0

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_http_status_distribution(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_http_status_distribution(mock_client, "rask_logs", 24)

        assert "http_status_distribution" in str(exc_info.value)
