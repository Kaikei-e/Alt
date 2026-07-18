"""models.py の型安全性テスト"""

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
        """ネストモデルは dict から構築できる（strict でもモデル入力として許容）"""
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

    def test_rejects_type_coercion_under_strict(self) -> None:
        """strict=True のため文字列→数値などの暗黙変換は拒否される"""
        with pytest.raises(ValidationError):
            AnalysisResult(hours_analyzed="24")  # type: ignore[arg-type]

    def test_rejects_dict_missing_required_field(self) -> None:
        """必須フィールドが欠けたdictはAnalysisResult構築時に拒否される"""
        with pytest.raises(ValidationError):
            AnalysisResult(api_performance=[{"service": "alt-backend"}])

    def test_is_frozen(self) -> None:
        """AnalysisResult は frozen でフィールド再代入不可"""
        result = AnalysisResult(hours_analyzed=24)
        with pytest.raises(ValidationError):
            result.hours_analyzed = 48  # type: ignore[misc]

    def test_generated_at_is_timezone_aware(self) -> None:
        """generated_at のデフォルトは UTC aware"""
        result = AnalysisResult()
        assert result.generated_at.tzinfo is not None
        assert result.generated_at.utcoffset() is not None
