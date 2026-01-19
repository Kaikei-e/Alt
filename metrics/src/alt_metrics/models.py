"""データモデル

Pydanticを使用した型安全なデータモデルを定義します。
"""

from __future__ import annotations

from datetime import datetime
from typing import Any

from pydantic import BaseModel, Field


class ServiceHealth(BaseModel):
    """サービス単位の健全性データ"""

    name: str
    total_logs: int = 0
    error_count: int = 0
    error_rate: float = 0.0
    last_seen: datetime | None = None
    p95_latency_ms: float = 0.0
    health_score: int = 100


class HttpEndpointStats(BaseModel):
    """HTTPエンドポイントパフォーマンス統計"""

    service: str
    route: str
    request_count: int
    avg_duration_ms: float
    p95_duration_ms: float
    avg_response_size: int
    error_rate: float
    status_2xx: int
    status_4xx: int
    status_5xx: int


class SLITrend(BaseModel):
    """SLIメトリクストレンドデータ"""

    timestamp: datetime
    service: str
    metric: str
    value: float


class LogVolumeStats(BaseModel):
    """ログ量統計（重要度別）"""

    service: str
    total_logs: int
    debug_count: int
    info_count: int
    warn_count: int
    error_count: int
    fatal_count: int


class AnalysisResult(BaseModel):
    """分析結果コンテナ

    すべての収集データと分析結果を保持します。
    """

    generated_at: datetime = Field(default_factory=datetime.now)
    hours_analyzed: int = 24

    # システム健全性
    overall_health_score: int = 100
    service_health: list[ServiceHealth] = Field(default_factory=list)

    # サービスログ
    service_stats: list[dict[str, Any]] = Field(default_factory=list)
    error_trends: list[dict[str, Any]] = Field(default_factory=list)

    # APIパフォーマンス
    api_performance: list[dict[str, Any]] = Field(default_factory=list)
    bottlenecks: list[dict[str, Any]] = Field(default_factory=list)

    # エラー分析
    error_types: list[dict[str, Any]] = Field(default_factory=list)
    recent_errors: list[dict[str, Any]] = Field(default_factory=list)

    # HTTP詳細分析
    http_endpoint_stats: list[dict[str, Any]] = Field(default_factory=list)
    http_status_distribution: list[dict[str, Any]] = Field(default_factory=list)

    # トレース詳細分析
    span_type_stats: list[dict[str, Any]] = Field(default_factory=list)
    error_spans: list[dict[str, Any]] = Field(default_factory=list)
    service_dependencies: list[dict[str, Any]] = Field(default_factory=list)

    # ログ詳細分析
    log_severity_distribution: list[dict[str, Any]] = Field(default_factory=list)
    log_volume_trends: list[dict[str, Any]] = Field(default_factory=list)

    # SLI/SLO分析
    sli_trends: list[dict[str, Any]] = Field(default_factory=list)
    slo_violations: list[dict[str, Any]] = Field(default_factory=list)

    # 推奨事項
    critical_issues: list[str] = Field(default_factory=list)
    warnings: list[str] = Field(default_factory=list)
    recommendations: list[str] = Field(default_factory=list)

    model_config = {"arbitrary_types_allowed": True}
