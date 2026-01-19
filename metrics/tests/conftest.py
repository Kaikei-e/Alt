"""共通テストフィクスチャ"""

from __future__ import annotations

from datetime import datetime
from unittest.mock import MagicMock

import pytest

from alt_metrics.config import HealthThresholds
from alt_metrics.models import AnalysisResult, ServiceHealth


@pytest.fixture
def mock_clickhouse_client() -> MagicMock:
    """モックClickHouseクライアント"""
    client = MagicMock()
    return client


@pytest.fixture
def default_thresholds() -> HealthThresholds:
    """デフォルト閾値設定"""
    return HealthThresholds()


@pytest.fixture
def sample_service_health() -> list[ServiceHealth]:
    """サンプルサービス健全性データ"""
    return [
        ServiceHealth(
            name="alt-backend",
            total_logs=10000,
            error_count=50,
            error_rate=0.5,
            p95_latency_ms=200.0,
            health_score=95,
        ),
        ServiceHealth(
            name="auth-hub",
            total_logs=5000,
            error_count=500,
            error_rate=10.0,
            p95_latency_ms=8000.0,
            health_score=30,
        ),
        ServiceHealth(
            name="pre-processor",
            total_logs=8000,
            error_count=100,
            error_rate=1.25,
            p95_latency_ms=1500.0,
            health_score=70,
        ),
    ]


@pytest.fixture
def sample_analysis_result(sample_service_health: list[ServiceHealth]) -> AnalysisResult:
    """サンプル分析結果"""
    return AnalysisResult(
        generated_at=datetime(2026, 1, 19, 12, 0, 0),
        hours_analyzed=24,
        overall_health_score=65,
        service_health=sample_service_health,
        service_stats=[
            {
                "service_name": "alt-backend",
                "total_logs": 10000,
                "error_count": 50,
                "error_rate": 0.5,
                "minutes_since_last_log": 1,
            },
            {
                "service_name": "auth-hub",
                "total_logs": 5000,
                "error_count": 500,
                "error_rate": 10.0,
                "minutes_since_last_log": 2,
            },
        ],
        api_performance=[
            {
                "service": "alt-backend",
                "endpoint": "GET /api/health",
                "request_count": 1000,
                "avg_ms": 50.0,
                "p50_ms": 40.0,
                "p95_ms": 200.0,
                "p99_ms": 500.0,
            },
        ],
        bottlenecks=[
            {
                "service": "auth-hub",
                "operation": "authenticate",
                "occurrences": 100,
                "avg_ms": 2000.0,
                "p95_ms": 5000.0,
                "total_time_sec": 200.0,
            },
        ],
        error_types=[
            {
                "service": "auth-hub",
                "error_type": "AuthenticationError",
                "error_count": 200,
                "sample_message": "Invalid token",
            },
        ],
        critical_issues=["**auth-hub** が危険な状態です (スコア: 30)。"],
        warnings=["エラー率が高いサービス (>1%): auth-hub"],
        recommendations=["主要エラーの調査: auth-hubのAuthenticationError (200件発生)"],
    )


@pytest.fixture
def sample_trace_rows() -> list[tuple]:
    """サンプルトレースデータ行"""
    return [
        ("alt-backend", "GET /api/health", 100, 50.0, 95.0, 99.0, 150.0, 0),
        ("alt-frontend", "GET /dashboard", 500, 200.0, 400.0, 500.0, 800.0, 2),
    ]
