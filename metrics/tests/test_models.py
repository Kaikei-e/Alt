"""models.py の型安全性テスト"""

from datetime import datetime

import pytest
from pydantic import BaseModel, ValidationError

from alt_metrics.models import (
    AnalysisResult,
    ApiPerformanceStats,
    Bottleneck,
    ErrorSpan,
    ErrorTrend,
    ErrorTypeStat,
    HttpEndpointStat,
    HttpStatusDistribution,
    LogSeverityDistribution,
    LogVolumeTrend,
    QueueSaturation,
    RecentError,
    ResourceUtilization,
    ServiceDependency,
    ServiceStat,
    SliTrend,
    SloViolation,
    SpanTypeStat,
)

_DT = datetime(2026, 1, 19, 12, 0, 0)

_ROW_MODEL_SAMPLES: list[tuple[str, type[BaseModel], dict]] = [
    (
        "service_stats",
        ServiceStat,
        {
            "service_name": "alt-backend",
            "total_logs": 10000,
            "error_count": 50,
            "warn_count": 100,
            "error_rate": 0.5,
            "last_seen": _DT,
            "minutes_since_last_log": 1,
        },
    ),
    (
        "error_trends",
        ErrorTrend,
        {"hour": _DT, "service_name": "alt-backend", "error_count": 10, "total_count": 1000, "error_rate": 1.0},
    ),
    (
        "bottlenecks",
        Bottleneck,
        {
            "service": "auth-hub",
            "operation": "authenticate",
            "occurrences": 50,
            "avg_ms": 2000.0,
            "p95_ms": 3500.0,
            "total_time_sec": 100.0,
        },
    ),
    (
        "error_types",
        ErrorTypeStat,
        {
            "service": "auth-hub",
            "error_type": "AuthenticationError",
            "error_count": 100,
            "sample_message": "Invalid token",
        },
    ),
    (
        "recent_errors",
        RecentError,
        {
            "service": "auth-hub",
            "level": "ERROR",
            "message": "Login failed",
            "error_type": "AuthError",
            "timestamp": "2026-01-19 12:00:00",
        },
    ),
    (
        "http_endpoint_stats",
        HttpEndpointStat,
        {
            "service": "alt-backend",
            "route": "/api/feeds",
            "request_count": 1000,
            "avg_duration_ms": 50.0,
            "p95_duration_ms": 100.0,
            "avg_response_size": 1024.0,
            "error_rate": 0.5,
            "status_2xx": 990,
            "status_4xx": 5,
            "status_5xx": 5,
        },
    ),
    (
        "http_status_distribution",
        HttpStatusDistribution,
        {
            "service": "alt-backend",
            "total_requests": 10000,
            "status_2xx": 9500,
            "status_3xx": 100,
            "status_4xx": 300,
            "status_5xx": 100,
            "error_5xx_rate": 1.0,
        },
    ),
    (
        "span_type_stats",
        SpanTypeStat,
        {
            "service": "alt-backend",
            "span_kind": "SERVER",
            "span_count": 1000,
            "avg_duration_ms": 50.0,
            "p95_duration_ms": 100.0,
            "error_count": 5,
        },
    ),
    (
        "error_spans",
        ErrorSpan,
        {
            "service": "auth-hub",
            "operation": "login",
            "error_message": "Invalid credentials",
            "error_count": 50,
            "avg_duration_ms": 100.0,
            "last_occurrence": "2026-01-19 12:00:00",
        },
    ),
    (
        "service_dependencies",
        ServiceDependency,
        {
            "caller": "alt-backend",
            "callee": "auth-hub",
            "call_count": 500,
            "avg_duration_ms": 50.0,
            "p95_duration_ms": 100.0,
            "error_count": 5,
        },
    ),
    (
        "log_severity_distribution",
        LogSeverityDistribution,
        {
            "service": "alt-backend",
            "total_logs": 10000,
            "debug_count": 1000,
            "info_count": 8000,
            "warn_count": 500,
            "error_count": 400,
            "fatal_count": 100,
            "error_rate": 5.0,
        },
    ),
    (
        "log_volume_trends",
        LogVolumeTrend,
        {"hour": _DT, "service": "alt-backend", "log_count": 5000, "error_count": 50, "error_rate": 1.0},
    ),
    (
        "sli_trends",
        SliTrend,
        {"time_bucket": _DT, "service": "alt-backend", "metric": "error_rate", "value": 0.005},
    ),
    (
        "slo_violations",
        SloViolation,
        {"service": "auth-hub", "time_bucket": _DT, "error_rate_pct": 2.5, "sample_count": 100},
    ),
    (
        "resource_utilization",
        ResourceUtilization,
        {
            "service": "alt-backend",
            "resource_type": "trace_duration_sec",
            "avg_utilization": 45.5,
            "max_utilization": 85.0,
            "p95_utilization": 75.0,
            "sample_count": 1000,
        },
    ),
    (
        "queue_saturation",
        QueueSaturation,
        {
            "service": "pre-processor",
            "queue_name": "feed_queue",
            "avg_wait_time_ms": 50.0,
            "max_wait_time_ms": 500,
            "p95_wait_time_ms": 200.0,
        },
    ),
]


_row_model_params = pytest.mark.parametrize(
    ("field", "model", "payload"),
    _ROW_MODEL_SAMPLES,
    ids=[field for field, _, _ in _ROW_MODEL_SAMPLES],
)


class TestTypedRowModels:
    """AnalysisResultの行データモデルのテスト（DECREE §5）"""

    @_row_model_params
    def test_construction(self, field: str, model: type[BaseModel], payload: dict) -> None:
        """コレクター行の全フィールドで構築できる"""
        row = model(**payload)
        for key, value in payload.items():
            assert getattr(row, key) == value

    @_row_model_params
    def test_is_frozen(self, field: str, model: type[BaseModel], payload: dict) -> None:
        """構築後のフィールド変更は拒否される"""
        row = model(**payload)
        first_key = next(iter(payload))
        with pytest.raises(ValidationError):
            setattr(row, first_key, payload[first_key])

    @_row_model_params
    def test_analysis_result_holds_typed_rows(self, field: str, model: type[BaseModel], payload: dict) -> None:
        """AnalysisResultの各リストは型付き行モデルを保持する"""
        result = AnalysisResult(**{field: [model(**payload)]})
        assert isinstance(getattr(result, field)[0], model)

    @_row_model_params
    def test_analysis_result_coerces_dict_rows(self, field: str, model: type[BaseModel], payload: dict) -> None:
        """dict入力は型付き行モデルへ変換される（untyped dictのまま保持しない）"""
        result = AnalysisResult(**{field: [payload]})
        assert isinstance(getattr(result, field)[0], model)


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
