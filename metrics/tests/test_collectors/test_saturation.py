"""collectors/saturation.py のテスト"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from alt_metrics.collectors.saturation import (
    collect_resource_utilization,
    collect_queue_saturation,
)
from alt_metrics.exceptions import CollectorError


class TestCollectResourceUtilization:
    """collect_resource_utilization関数のテスト"""

    def test_returns_resource_utilization_data(self) -> None:
        """リソース使用率データを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "resource_type",
            "avg_utilization",
            "max_utilization",
            "p95_utilization",
            "sample_count",
        ]
        mock_client.query.return_value.result_rows = [
            ("alt-backend", "cpu", 45.5, 85.0, 75.0, 1000),
            ("alt-backend", "memory", 60.0, 78.0, 72.0, 1000),
            ("auth-hub", "cpu", 30.0, 65.0, 55.0, 800),
        ]

        result = collect_resource_utilization(mock_client, "rask_logs", 24)

        assert len(result) == 3
        assert result[0]["service"] == "alt-backend"
        assert result[0]["resource_type"] == "cpu"
        assert result[0]["avg_utilization"] == 45.5
        assert result[1]["resource_type"] == "memory"

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_resource_utilization(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_resource_utilization(mock_client, "rask_logs", 24)

        assert "resource_utilization" in str(exc_info.value)

    def test_query_uses_correct_parameters(self) -> None:
        """クエリが正しいパラメータを使用"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        collect_resource_utilization(mock_client, "custom_db", 48)

        call_args = mock_client.query.call_args[0][0]
        assert "custom_db" in call_args
        assert "INTERVAL 48 HOUR" in call_args


class TestCollectQueueSaturation:
    """collect_queue_saturation関数のテスト"""

    def test_returns_queue_saturation_data(self) -> None:
        """キュー飽和度データを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "queue_name",
            "avg_depth",
            "max_depth",
            "avg_wait_time_ms",
            "p95_wait_time_ms",
        ]
        mock_client.query.return_value.result_rows = [
            ("pre-processor", "feed_queue", 100.0, 500, 50.0, 200.0),
            ("tag-generator", "tag_queue", 50.0, 200, 30.0, 100.0),
        ]

        result = collect_queue_saturation(mock_client, "rask_logs", 24)

        assert len(result) == 2
        assert result[0]["queue_name"] == "feed_queue"
        assert result[0]["avg_depth"] == 100.0
        assert result[1]["p95_wait_time_ms"] == 100.0

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_queue_saturation(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_queue_saturation(mock_client, "rask_logs", 24)

        assert "queue_saturation" in str(exc_info.value)
