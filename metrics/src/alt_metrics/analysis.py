"""健全性分析とスコアリングロジック

設定可能な閾値を使用してサービスの健全性スコアを計算します。
"""

from typing import Literal

from alt_metrics.config import HealthThresholds
from alt_metrics.models import (
    AnalysisResult,
    ApiPerformanceStats,
    ErrorBudgetResult,
    ErrorTypeStat,
    LogVolumeTrend,
    ServiceHealth,
    ServiceStat,
)

HEALTH_SCORE_HEALTHY = 90
HEALTH_SCORE_WARNING = 70
HEALTH_SCORE_DEGRADED = 50

_BUDGET_CRITICAL_PCT = 80
_BUDGET_WARNING_PCT = 50
_DEP_MIN_CALLS = 10
_DEP_ERROR_RATE_THRESHOLD = 0.05
_MIN_TREND_POINTS = 2
_LOG_SPIKE_MULTIPLIER = 2


def calculate_health_score(
    error_rate: float,
    p95_ms: float,
    log_gap_minutes: float,
    thresholds: HealthThresholds | None = None,
) -> int:
    """ヘルススコア (0-100) を計算

    以下の要素に基づいて減点:
    - エラー率: 高いほど減点
    - レイテンシ: p95が高いほど減点
    - ログ欠落: 最新ログがないほど減点

    Args:
        error_rate: エラー率 (%)
        p95_ms: p95レイテンシ (ms)
        log_gap_minutes: 最終ログからの経過時間 (分)
        thresholds: 閾値設定 (Noneの場合はデフォルト)

    Returns:
        0-100のヘルススコア
    """
    if thresholds is None:
        thresholds = HealthThresholds()

    score = 100

    # エラー率による減点
    if error_rate > thresholds.error_rate_critical:
        score -= 40
    elif error_rate > thresholds.error_rate_high:
        score -= 25
    elif error_rate > thresholds.error_rate_warning:
        score -= 10
    elif error_rate > thresholds.error_rate_minor:
        score -= 5

    # レイテンシによる減点
    if p95_ms > thresholds.latency_critical_ms:
        score -= 30
    elif p95_ms > thresholds.latency_high_ms:
        score -= 20
    elif p95_ms > thresholds.latency_warning_ms:
        score -= 10
    elif p95_ms > thresholds.latency_minor_ms:
        score -= 5

    # ログ欠落による減点
    if log_gap_minutes > thresholds.log_gap_critical_min:
        score -= 30
    elif log_gap_minutes > thresholds.log_gap_warning_min:
        score -= 15

    return max(0, score)


def get_health_status(score: int) -> str:
    """スコアから日本語ステータスラベルを取得"""
    if score >= HEALTH_SCORE_HEALTHY:
        return "正常"
    if score >= HEALTH_SCORE_WARNING:
        return "警告"
    if score >= HEALTH_SCORE_DEGRADED:
        return "劣化"
    return "危険"


def get_health_status_emoji(status: str) -> str:
    """ステータスに対応する絵文字を取得"""
    emoji_map = {
        "正常": "✅",
        "警告": "⚠️",
        "劣化": "🔶",
        "危険": "🔴",
    }
    return emoji_map.get(status, "")


def calculate_error_budget(
    error_rate: float,
    slo_target: float,
    hours_analyzed: int,
) -> ErrorBudgetResult:
    """エラーバジェットを計算

    Google SREのエラーバジェット概念に基づいて計算します。
    エラーバジェット = 100% - SLO目標
    消費率 = 実際のエラー率 / エラーバジェット * 100

    Args:
        error_rate: 実際のエラー率 (%)
        slo_target: SLO目標 (例: 99.9)
        hours_analyzed: 分析期間（時間）

    Returns:
        エラーバジェット計算結果
    """
    budget_total = 100.0 - slo_target
    budget_consumed = error_rate
    budget_remaining = max(0.0, budget_total - budget_consumed)
    is_exceeded = budget_consumed > budget_total

    # 消費率を計算（ゼロ除算防止）
    if budget_total > 0:
        consumption_pct = round((budget_consumed / budget_total) * 100, 1)
    else:
        consumption_pct = 100.0 if budget_consumed > 0 else 0.0

    # ステータスを決定（超過時は消費率に関わらず exceeded）
    status: Literal["healthy", "warning", "critical", "exceeded"]
    if is_exceeded:
        status = "exceeded"
    elif consumption_pct >= _BUDGET_CRITICAL_PCT:
        status = "critical"
    elif consumption_pct >= _BUDGET_WARNING_PCT:
        status = "warning"
    else:
        status = "healthy"

    return ErrorBudgetResult(
        slo_target=slo_target,
        budget_total=budget_total,
        budget_consumed=budget_consumed,
        budget_remaining=budget_remaining,
        consumption_pct=consumption_pct,
        is_exceeded=is_exceeded,
        status=status,
        hours_analyzed=hours_analyzed,
    )


def analyze_health(
    result: AnalysisResult,
    thresholds: HealthThresholds | None = None,
) -> AnalysisResult:
    """データを分析してヘルススコアと推奨事項を生成する純関数

    入力の AnalysisResult は変更せず、分析結果を反映した新しいインスタンスを返す。

    Args:
        result: 分析結果コンテナ（変更されない）
        thresholds: 閾値設定

    Returns:
        ヘルススコア・推奨事項付きの新しい AnalysisResult
    """
    if thresholds is None:
        thresholds = HealthThresholds()

    # 同一サービスに複数エンドポイントがありうるため、ヘルススコアには
    # 最悪ケース（最大p95）を採用する。単純な内包表記だと後続行が
    # 上書きし、api_performanceの並び順（p95降順）依存の値になってしまう。
    service_latencies: dict[str, float] = {}
    for s in result.api_performance:
        service_latencies[s.service] = max(service_latencies.get(s.service, 0.0), s.p95_ms)

    service_health: list[ServiceHealth] = []
    for stats in result.service_stats:
        p95_latency_ms = service_latencies.get(stats.service_name, 0.0)
        health_score = calculate_health_score(
            stats.error_rate,
            p95_latency_ms,
            stats.minutes_since_last_log,
            thresholds,
        )
        service_health.append(
            ServiceHealth(
                name=stats.service_name,
                total_logs=stats.total_logs,
                error_count=stats.error_count,
                error_rate=stats.error_rate,
                last_seen=stats.last_seen,
                p95_latency_ms=p95_latency_ms,
                health_score=health_score,
            )
        )

    overall_health_score = result.overall_health_score
    error_budget = result.error_budget
    if service_health:
        overall_health_score = sum(s.health_score for s in service_health) // len(service_health)

        total_logs = sum(s.total_logs for s in service_health)
        total_errors = sum(s.error_count for s in service_health)
        if total_logs > 0:
            overall_error_rate = (total_errors / total_logs) * 100
            error_budget = calculate_error_budget(
                error_rate=overall_error_rate,
                slo_target=thresholds.slo_availability_target,
                hours_analyzed=result.hours_analyzed,
            )

    critical_issues = list(result.critical_issues)
    warnings = list(result.warnings)
    recommendations = list(result.recommendations)

    critical_issues.extend(
        f"**{svc.name}** が危険な状態です (スコア: {svc.health_score})。"
        f"エラー率: {svc.error_rate}%、p95レイテンシ: {svc.p95_latency_ms}ms"
        for svc in service_health
        if svc.health_score < HEALTH_SCORE_DEGRADED
    )

    _collect_warnings(
        result=result,
        service_health=service_health,
        thresholds=thresholds,
        critical_issues=critical_issues,
        warnings=warnings,
    )
    _collect_recommendations(
        api_performance=result.api_performance,
        error_types=result.error_types,
        service_stats=result.service_stats,
        thresholds=thresholds,
        recommendations=recommendations,
    )

    return result.model_copy(
        update={
            "service_health": service_health,
            "overall_health_score": overall_health_score,
            "error_budget": error_budget,
            "critical_issues": critical_issues,
            "warnings": warnings,
            "recommendations": recommendations,
        }
    )


def _collect_warnings(
    *,
    result: AnalysisResult,
    service_health: list[ServiceHealth],
    thresholds: HealthThresholds,
    critical_issues: list[str],
    warnings: list[str],
) -> None:
    """分析結果から警告メッセージを収集（リストへ append）"""
    high_error_services = [s for s in service_health if s.error_rate > thresholds.error_rate_warning]
    if high_error_services:
        names = ", ".join(s.name for s in high_error_services[:3])
        warnings.append(f"エラー率が高いサービス (>{thresholds.error_rate_warning}%): {names}")

    if result.bottlenecks:
        top_bottleneck = result.bottlenecks[0]
        warnings.append(
            f"パフォーマンスボトルネック: {top_bottleneck.service}/{top_bottleneck.operation} "
            f"(p95: {top_bottleneck.p95_ms}ms, 合計時間: {top_bottleneck.total_time_sec}秒)"
        )

    high_5xx_services = [s for s in result.http_status_distribution if s.error_5xx_rate > 1]
    warnings.extend(
        f"HTTP 5xxエラー率が高い: {svc.service} ({svc.error_5xx_rate}% / {svc.total_requests}リクエスト)"
        for svc in high_5xx_services[:3]
    )

    if result.slo_violations:
        violation_count = len(result.slo_violations)
        affected_services = {v.service for v in result.slo_violations}
        critical_issues.append(
            f"SLO違反を検出: {violation_count}期間でエラー率 >{thresholds.slo_error_rate_threshold}% "
            f"({len(affected_services)}サービスに影響)"
        )

    if result.error_spans:
        top_error_span = result.error_spans[0]
        warnings.append(
            f"トレースエラー検出: {top_error_span.service}の{top_error_span.operation} ({top_error_span.error_count}件)"
        )

    high_error_deps = [
        d
        for d in result.service_dependencies
        if d.call_count > _DEP_MIN_CALLS and (d.error_count / d.call_count) > _DEP_ERROR_RATE_THRESHOLD
    ]
    for dep in high_error_deps[:2]:
        error_pct = round(dep.error_count / dep.call_count * 100, 1)
        warnings.append(
            f"サービス間呼び出しエラー率が高い: {dep.caller} → {dep.callee} "
            f"({error_pct}%エラー、{dep.call_count}呼び出し)"
        )

    _collect_log_volume_spike_warnings(result.log_volume_trends, warnings)


def _collect_log_volume_spike_warnings(
    log_volume_trends: list[LogVolumeTrend],
    warnings: list[str],
) -> None:
    """ログ量トレンドから急増スパイク警告を収集"""
    if not log_volume_trends:
        return

    service_volumes: dict[str, list[int]] = {}
    for trend in log_volume_trends:
        if trend.service:
            service_volumes.setdefault(trend.service, []).append(trend.log_count)

    for svc, volumes in service_volumes.items():
        if len(volumes) >= _MIN_TREND_POINTS:
            recent = volumes[0]
            previous = volumes[1]
            if previous > 0 and recent > previous * _LOG_SPIKE_MULTIPLIER:
                warnings.append(
                    f"ログ量スパイク検出: {svc} ({recent}件 vs 前時間{previous}件、{round(recent / previous, 1)}倍増加)"
                )


def _collect_recommendations(
    *,
    api_performance: list[ApiPerformanceStats],
    error_types: list[ErrorTypeStat],
    service_stats: list[ServiceStat],
    thresholds: HealthThresholds,
    recommendations: list[str],
) -> None:
    """分析結果から推奨事項を収集（リストへ append）"""
    slow_apis = [a for a in api_performance if a.p95_ms > thresholds.latency_warning_ms]
    if slow_apis:
        recommendations.append(
            f"遅いエンドポイントの最適化: {len(slow_apis)}件のAPIがp95 > {thresholds.latency_warning_ms}ms。"
            "キャッシュ、クエリ最適化、非同期処理を検討してください。"
        )

    if error_types:
        top_error = error_types[0]
        recommendations.append(
            f"主要エラーの調査: {top_error.service}の{top_error.error_type} ({top_error.error_count}件発生)"
        )

    stale_services = [s for s in service_stats if s.minutes_since_last_log > thresholds.log_gap_warning_min]
    if stale_services:
        names = ", ".join(s.service_name for s in stale_services[:3])
        recommendations.append(f"ログ停止サービスの確認: {names}")
