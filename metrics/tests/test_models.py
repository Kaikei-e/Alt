"""models.py の型安全性テスト"""

from __future__ import annotations

import pytest
from pydantic import ValidationError

from alt_metrics.models import AnalysisResult, ApiPerformanceStats


class TestApiPerformanceStats:
    """ApiPerformanceStatsモデルのテスト"""

    def test_is_frozen(self) -> None:
        """構築後のフィールド変更は拒否される"""
        stats = ApiPerformanceStats(
            service="alt-backend",
            endpoint="GET /api/health",
            request_count=100,
            avg_ms=50.0,
            p50_ms=40.0,
            p95_ms=95.0,
            p99_ms=99.0,
        )

        with pytest.raises(ValidationError):
            stats.p95_ms = 1.0  # type: ignore[misc]

    def test_missing_required_field_raises(self) -> None:
        """必須フィールド欠落時はValidationError"""
        with pytest.raises(ValidationError):
            ApiPerformanceStats(service="alt-backend")  # type: ignore[call-arg]

    def test_optional_fields_default(self) -> None:
        """max_ms/error_spansはデフォルト値を持つ"""
        stats = ApiPerformanceStats(
            service="alt-backend",
            endpoint="GET /api/health",
            request_count=100,
            avg_ms=50.0,
            p50_ms=40.0,
            p95_ms=95.0,
            p99_ms=99.0,
        )

        assert stats.max_ms == 0.0
        assert stats.error_spans == 0


class TestAnalysisResultApiPerformanceTyping:
    """AnalysisResult.api_performanceの型付けテスト（DECREE §5）"""

    def test_accepts_typed_api_performance_stats(self) -> None:
        """ApiPerformanceStatsのリストをそのまま保持する"""
        stats = ApiPerformanceStats(
            service="alt-backend",
            endpoint="GET /api/health",
            request_count=100,
            avg_ms=50.0,
            p50_ms=40.0,
            p95_ms=95.0,
            p99_ms=99.0,
        )
        result = AnalysisResult(api_performance=[stats])

        assert isinstance(result.api_performance[0], ApiPerformanceStats)
        assert result.api_performance[0].service == "alt-backend"

    def test_coerces_dict_input_into_typed_model(self) -> None:
        """dictを渡してもApiPerformanceStatsへ変換される（既存呼び出し側の後方互換）"""
        result = AnalysisResult(
            api_performance=[
                {
                    "service": "alt-backend",
                    "endpoint": "GET /api/health",
                    "request_count": 100,
                    "avg_ms": 50.0,
                    "p50_ms": 40.0,
                    "p95_ms": 95.0,
                    "p99_ms": 99.0,
                }
            ]
        )

        assert isinstance(result.api_performance[0], ApiPerformanceStats)
        assert result.api_performance[0].p95_ms == 95.0

    def test_rejects_dict_missing_required_field(self) -> None:
        """必須フィールドが欠けたdictはAnalysisResult構築時に拒否される"""
        with pytest.raises(ValidationError):
            AnalysisResult(api_performance=[{"service": "alt-backend"}])
