"""データモデル

Pydanticを使用した型安全なデータモデルを定義します。
"""

from datetime import UTC, datetime
from typing import Literal

from pydantic import BaseModel, ConfigDict, Field


def _utc_now() -> datetime:
    return datetime.now(UTC)


class ErrorBudgetResult(BaseModel):
    """エラーバジェット計算結果

    SLO目標に基づくエラーバジェットの状態を表します。
    Google SREのエラーバジェット概念に基づいています。
    """

    model_config = ConfigDict(strict=True, frozen=True)

    slo_target: float  # SLO目標 (例: 99.9%)
    budget_total: float  # 合計バジェット (100 - SLO)
    budget_consumed: float  # 消費済みバジェット (実際のエラー率)
    budget_remaining: float  # 残りバジェット
    consumption_pct: float  # 消費率 (%)
    is_exceeded: bool  # バジェット超過フラグ
    status: Literal["healthy", "warning", "critical", "exceeded"]  # ステータス
    hours_analyzed: int  # 分析期間


class ServiceHealth(BaseModel):
    """サービス単位の健全性データ"""

    model_config = ConfigDict(strict=True, frozen=True)

    name: str
    total_logs: int = 0
    error_count: int = 0
    error_rate: float = 0.0
    last_seen: datetime | None = None
    p95_latency_ms: float = 0.0
    health_score: int = 100


class ApiPerformanceStats(BaseModel):
    """APIエンドポイント単位のパフォーマンス統計（`collect_api_performance`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    endpoint: str
    request_count: int
    avg_ms: float
    p50_ms: float
    p95_ms: float
    p99_ms: float
    max_ms: float = 0.0
    error_spans: int = 0


class ServiceStat(BaseModel):
    """レガシーlogsテーブル由来のサービス統計（`collect_service_stats`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service_name: str
    total_logs: int
    error_count: int
    warn_count: int = 0
    error_rate: float
    last_seen: datetime | None = None
    minutes_since_last_log: int


class ErrorTrend(BaseModel):
    """時間別エラートレンド（`collect_error_trends`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    hour: datetime
    service_name: str
    error_count: int
    total_count: int
    error_rate: float


class Bottleneck(BaseModel):
    """パフォーマンスボトルネック（`collect_bottlenecks`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    operation: str
    occurrences: int
    avg_ms: float
    p95_ms: float
    total_time_sec: float


class ErrorTypeStat(BaseModel):
    """エラー種類別統計（`collect_error_types`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    error_type: str
    error_count: int
    sample_message: str


class RecentError(BaseModel):
    """最新エラーログ（`collect_recent_errors`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    level: str
    message: str
    error_type: str
    timestamp: str


class HttpEndpointStat(BaseModel):
    """HTTPエンドポイント統計（`collect_http_endpoint_stats`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    route: str
    request_count: int
    avg_duration_ms: float
    p95_duration_ms: float
    avg_response_size: float
    error_rate: float
    status_2xx: int
    status_4xx: int
    status_5xx: int


class HttpStatusDistribution(BaseModel):
    """サービス別HTTPステータス分布（`collect_http_status_distribution`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    total_requests: int
    status_2xx: int
    status_3xx: int
    status_4xx: int
    status_5xx: int
    error_5xx_rate: float


class SpanTypeStat(BaseModel):
    """スパン種類別統計（`collect_span_type_stats`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    span_kind: str
    span_count: int
    avg_duration_ms: float
    p95_duration_ms: float
    error_count: int


class ErrorSpan(BaseModel):
    """エラースパン詳細（`collect_error_spans`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    operation: str
    error_message: str
    error_count: int
    avg_duration_ms: float
    last_occurrence: str


class ServiceDependency(BaseModel):
    """サービス間呼び出し依存関係（`collect_service_dependencies`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    caller: str
    callee: str
    call_count: int
    avg_duration_ms: float
    p95_duration_ms: float
    error_count: int


class LogSeverityDistribution(BaseModel):
    """サービス別ログ重要度分布（`collect_log_severity_distribution`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    total_logs: int
    debug_count: int
    info_count: int
    warn_count: int
    error_count: int
    fatal_count: int
    error_rate: float


class LogVolumeTrend(BaseModel):
    """時間別ログ量トレンド（`collect_log_volume_trends`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    hour: datetime
    service: str
    log_count: int
    error_count: int
    error_rate: float


class SliTrend(BaseModel):
    """SLIメトリクストレンド（`collect_sli_trends`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    time_bucket: datetime
    service: str
    metric: str
    value: float


class SloViolation(BaseModel):
    """SLO違反（`collect_slo_violations`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    time_bucket: datetime
    error_rate_pct: float
    sample_count: int


class ResourceUtilization(BaseModel):
    """サービス別リソース使用率（`collect_resource_utilization`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    resource_type: str
    avg_utilization: float
    max_utilization: float
    p95_utilization: float
    sample_count: int


class QueueSaturation(BaseModel):
    """キュー飽和度（`collect_queue_saturation`の結果）"""

    model_config = ConfigDict(strict=True, frozen=True)

    service: str
    queue_name: str
    avg_wait_time_ms: float
    max_wait_time_ms: int
    p95_wait_time_ms: float


class AnalysisResult(BaseModel):
    """分析結果コンテナ

    すべての収集データと分析結果を保持します。
    """

    model_config = ConfigDict(strict=True, frozen=True)

    generated_at: datetime = Field(default_factory=_utc_now)
    hours_analyzed: int = 24

    # システム健全性
    overall_health_score: int = 100
    service_health: list[ServiceHealth] = Field(default_factory=list)

    # サービスログ
    service_stats: list[ServiceStat] = Field(default_factory=list)
    error_trends: list[ErrorTrend] = Field(default_factory=list)

    # APIパフォーマンス
    api_performance: list[ApiPerformanceStats] = Field(default_factory=list)
    bottlenecks: list[Bottleneck] = Field(default_factory=list)

    # エラー分析
    error_types: list[ErrorTypeStat] = Field(default_factory=list)
    recent_errors: list[RecentError] = Field(default_factory=list)

    # HTTP詳細分析
    http_endpoint_stats: list[HttpEndpointStat] = Field(default_factory=list)
    http_status_distribution: list[HttpStatusDistribution] = Field(default_factory=list)

    # トレース詳細分析
    span_type_stats: list[SpanTypeStat] = Field(default_factory=list)
    error_spans: list[ErrorSpan] = Field(default_factory=list)
    service_dependencies: list[ServiceDependency] = Field(default_factory=list)

    # ログ詳細分析
    log_severity_distribution: list[LogSeverityDistribution] = Field(default_factory=list)
    log_volume_trends: list[LogVolumeTrend] = Field(default_factory=list)

    # SLI/SLO分析
    sli_trends: list[SliTrend] = Field(default_factory=list)
    slo_violations: list[SloViolation] = Field(default_factory=list)

    # Saturation (Golden Signals)
    resource_utilization: list[ResourceUtilization] = Field(default_factory=list)
    queue_saturation: list[QueueSaturation] = Field(default_factory=list)

    # エラーバジェット
    error_budget: ErrorBudgetResult | None = None

    # 推奨事項
    critical_issues: list[str] = Field(default_factory=list)
    warnings: list[str] = Field(default_factory=list)
    recommendations: list[str] = Field(default_factory=list)
