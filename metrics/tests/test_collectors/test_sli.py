"""collectors/sli.py のテスト"""

from __future__ import annotations

from unittest.mock import MagicMock

import pytest

from alt_metrics.collectors.sli import (
    collect_sli_trends,
    collect_slo_violations,
)
from alt_metrics.exceptions import CollectorError


class TestCollectSliTrends:
    """collect_sli_trends関数のテスト"""

    def test_returns_sli_trend_data(self) -> None:
        """SLIトレンドデータを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "time_bucket",
            "service",
            "metric",
            "value",
        ]
        mock_client.query.return_value.result_rows = [
            ("2026-01-19 12:00:00", "alt-backend", "error_rate", 0.005),
            ("2026-01-19 12:00:00", "alt-backend", "log_throughput", 1000.0),
            ("2026-01-19 11:55:00", "auth-hub", "error_rate", 0.01),
        ]

        result = collect_sli_trends(mock_client, "rask_logs", 24)

        assert len(result) == 3
        assert result[0]["metric"] == "error_rate"
        assert result[0]["value"] == 0.005
        assert result[1]["metric"] == "log_throughput"

    def test_empty_result_returns_empty_list(self) -> None:
        """空の結果は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_sli_trends(mock_client, "rask_logs", 24)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Connection failed")

        with pytest.raises(CollectorError) as exc_info:
            collect_sli_trends(mock_client, "rask_logs", 24)

        assert "sli_trends" in str(exc_info.value)

    def test_query_uses_correct_parameters(self) -> None:
        """クエリが正しいパラメータを使用"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        collect_sli_trends(mock_client, "custom_db", 48)

        call_args = mock_client.query.call_args[0][0]
        assert "custom_db.sli_metrics" in call_args
        assert "INTERVAL 48 HOUR" in call_args


class TestCollectSloViolations:
    """collect_slo_violations関数のテスト"""

    def test_returns_slo_violation_data(self) -> None:
        """SLO違反データを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = [
            "service",
            "time_bucket",
            "error_rate_pct",
            "sample_count",
        ]
        mock_client.query.return_value.result_rows = [
            ("auth-hub", "2026-01-19 12:00:00", 2.5, 100),
            ("alt-backend", "2026-01-19 11:55:00", 1.5, 200),
        ]

        result = collect_slo_violations(mock_client, "rask_logs", 24, 1.0)

        assert len(result) == 2
        assert result[0]["service"] == "auth-hub"
        assert result[0]["error_rate_pct"] == 2.5
        assert result[1]["sample_count"] == 200

    def test_empty_result_when_no_violations(self) -> None:
        """違反がない場合は空リストを返す"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        result = collect_slo_violations(mock_client, "rask_logs", 24, 1.0)

        assert result == []

    def test_raises_collector_error_on_exception(self) -> None:
        """例外発生時はCollectorErrorを投げる"""
        mock_client = MagicMock()
        mock_client.query.side_effect = Exception("Query timeout")

        with pytest.raises(CollectorError) as exc_info:
            collect_slo_violations(mock_client, "rask_logs", 24, 1.0)

        assert "slo_violations" in str(exc_info.value)

    def test_uses_custom_threshold(self) -> None:
        """カスタム閾値を使用"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        collect_slo_violations(mock_client, "rask_logs", 24, 5.0)

        call_args = mock_client.query.call_args[0][0]
        # 5.0% threshold = 0.05 in the query
        assert "0.05" in call_args

    def test_default_threshold_is_1_percent(self) -> None:
        """デフォルト閾値は1%"""
        mock_client = MagicMock()
        mock_client.query.return_value.column_names = []
        mock_client.query.return_value.result_rows = []

        collect_slo_violations(mock_client, "rask_logs", 24)

        call_args = mock_client.query.call_args[0][0]
        # 1.0% threshold = 0.01 in the query
        assert "0.01" in call_args
