"""collectors/traces.py のテスト"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from alt_metrics.collectors.traces import (
    collect_api_performance,
    collect_bottlenecks,
    collect_error_spans,
    collect_service_dependencies,
    collect_service_latency,
    collect_span_type_stats,
)
from alt_metrics.exceptions import CollectorError


class TestCollectApiPerformance:
    """collect_api_performance関数のテスト"""

    def test_returns_list_of_dicts(self) -> None:
        """結果を辞書のリストとして返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "endpoint",
            "request_count",
            "avg_ms",
            "p50_ms",
            "p95_ms",
            "p99_ms",
            "max_ms",
            "error_spans",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", "GET /api/health", 100, 50.0, 40.0, 95.0, 99.0, 150.0, 0),
            ("alt-frontend", "GET /dashboard", 500, 200.0, 180.0, 400.0, 500.0, 800.0, 2),
        ]

        result = collect_api_performance(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["service"] == "alt-backend"
        assert result[0]["p95_ms"] == 95.0
        assert result[1]["endpoint"] == "GET /dashboard"

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_api_performance(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_api_performance(mock_client, "rask_logs", 24)

        assert "api_performance" in str(exc_info.value)


class TestCollectBottlenecks:
    """collect_bottlenecks関数のテスト"""

    def test_returns_bottleneck_data(self) -> None:
        """ボトルネックデータを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "operation",
            "occurrences",
            "avg_ms",
            "p95_ms",
            "total_time_sec",
        ]
        mock_client.query.return_value.result_rows = [
            ("auth-hub", "authenticate", 50, 2000.0, 3500.0, 100.0),
        ]

        result = collect_bottlenecks(mock_client, "rask_logs", 24)

        assert len(result) == 1
        assert result[0]["service"] == "auth-hub"
        assert result[0]["operation"] == "authenticate"
        assert result[0]["p95_ms"] == 3500.0

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_bottlenecks(mock_client, "rask_logs", 24)

        assert "bottlenecks" in str(exc_info.value)


class TestCollectServiceLatency:
    """collect_service_latency関数のテスト"""

    def test_returns_dict_of_latencies(self) -> None:
        """サービスごとのp95レイテンシ辞書を返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.result_rows = [
            ("alt-backend", 95.0),
            ("auth-hub", 500.0),
        ]

        result = collect_service_latency(mock_client, "rask_logs", 24)

        assert isinstance(result, dict)
        assert result["alt-backend"] == 95.0
        assert result["auth-hub"] == 500.0

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_service_latency(mock_client, "rask_logs", 24)

        assert "service_latency" in str(exc_info.value)


class TestCollectSpanTypeStats:
    """collect_span_type_stats関数のテスト"""

    def test_returns_span_stats(self) -> None:
        """スパン種類別統計を返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "span_kind",
            "span_count",
            "avg_duration_ms",
            "p95_duration_ms",
            "error_count",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", "SERVER", 1000, 50.0, 100.0, 5),
            ("alt-backend", "CLIENT", 500, 30.0, 80.0, 2),
        ]

        result = collect_span_type_stats(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["span_kind"] == "SERVER"
        assert result[0]["span_count"] == 1000

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_span_type_stats(mock_client, "rask_logs", 24)

        assert "span_type_stats" in str(exc_info.value)


class TestCollectErrorSpans:
    """collect_error_spans関数のテスト"""

    def test_returns_error_span_data(self) -> None:
        """エラースパンデータを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "operation",
            "error_message",
            "error_count",
            "avg_duration_ms",
            "last_occurrence",
        ]
        mock_client.query.return_value.result_rows = [
            ("auth-hub", "login", "Invalid credentials", 50, 100.0, "2026-01-19 12:00:00"),
        ]

        result = collect_error_spans(mock_client, "rask_logs", 24)

        assert len(result) == 1
        assert result[0]["operation"] == "login"
        assert result[0]["error_count"] == 50

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_error_spans(mock_client, "rask_logs", 24)

        assert "error_spans" in str(exc_info.value)


class TestCollectServiceDependencies:
    """collect_service_dependencies関数のテスト"""

    def test_returns_dependency_data(self) -> None:
        """サービス依存関係データを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "caller",
            "callee",
            "call_count",
            "avg_duration_ms",
            "p95_duration_ms",
            "error_count",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", "auth-hub", 500, 50.0, 100.0, 5),
            ("alt-frontend", "alt-backend", 1000, 100.0, 300.0, 10),
        ]

        result = collect_service_dependencies(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["caller"] == "alt-backend"
        assert result[0]["callee"] == "auth-hub"
        assert result[1]["call_count"] == 1000

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_service_dependencies(mock_client, "rask_logs", 24)

        assert "service_dependencies" in str(exc_info.value)
