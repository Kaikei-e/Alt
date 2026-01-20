"""健全性分析とスコアリングロジック

設定可能な閾値を使用してサービスの健全性スコアを計算します。
"""

from __future__ import annotations

from typing import Literal

from alt_metrics.config import HealthThresholds
from alt_metrics.models import AnalysisResult, ErrorBudgetResult, ServiceHealth


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
    if score >= 90:
        return "正常"
    elif score >= 70:
        return "警告"
    elif score >= 50:
        return "劣化"
    else:
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

    # ステータスを決定
    status: Literal["healthy", "warning", "critical", "exceeded"]
    if consumption_pct > 100:
        status = "exceeded"
    elif consumption_pct >= 80:
        status = "critical"
    elif consumption_pct >= 50:
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
) -> None:
    """データを分析してヘルススコアと推奨事項を生成

    resultを直接変更し、以下を設定:
    - service_health: サービスごとの健全性データ
    - overall_health_score: 全体のスコア
    - critical_issues, warnings, recommendations: 問題点と推奨事項

    Args:
        result: 分析結果コンテナ (変更される)
        thresholds: 閾値設定
    """
    if thresholds is None:
        thresholds = HealthThresholds()

    service_latencies = {s["service"]: s.get("p95_ms", 0) for s in result.api_performance}

    # サービスごとの健全性を計算
    for stats in result.service_stats:
        service = ServiceHealth(
            name=stats["service_name"],
            total_logs=stats["total_logs"],
            error_count=stats["error_count"],
            error_rate=stats["error_rate"],
            last_seen=stats.get("last_seen"),
            p95_latency_ms=service_latencies.get(stats["service_name"], 0),
        )
        service.health_score = calculate_health_score(
            service.error_rate,
            service.p95_latency_ms,
            stats.get("minutes_since_last_log", 0),
            thresholds,
        )
        result.service_health.append(service)

    # 全体の健全性スコアを計算
    if result.service_health:
        result.overall_health_score = sum(s.health_score for s in result.service_health) // len(result.service_health)

        # 全体のエラー率を計算してエラーバジェットを算出
        total_logs = sum(s.total_logs for s in result.service_health)
        total_errors = sum(s.error_count for s in result.service_health)
        if total_logs > 0:
            overall_error_rate = (total_errors / total_logs) * 100
            result.error_budget = calculate_error_budget(
                error_rate=overall_error_rate,
                slo_target=thresholds.slo_availability_target,
                hours_analyzed=result.hours_analyzed,
            )

    # 重大な問題を生成
    for svc in result.service_health:
        if svc.health_score < 50:
            result.critical_issues.append(
                f"**{svc.name}** が危険な状態です (スコア: {svc.health_score})。"
                f"エラー率: {svc.error_rate}%、p95レイテンシ: {svc.p95_latency_ms}ms"
            )

    # 警告を生成
    _generate_warnings(result, thresholds)

    # 推奨事項を生成
    _generate_recommendations(result, thresholds)


def _generate_warnings(result: AnalysisResult, thresholds: HealthThresholds) -> None:
    """分析結果から警告メッセージを生成"""
    # エラー率が高いサービス
    high_error_services = [s for s in result.service_health if s.error_rate > thresholds.error_rate_warning]
    if high_error_services:
        names = ", ".join(s.name for s in high_error_services[:3])
        result.warnings.append(f"エラー率が高いサービス (>{thresholds.error_rate_warning}%): {names}")

    # ボトルネック
    if result.bottlenecks:
        top_bottleneck = result.bottlenecks[0]
        result.warnings.append(
            f"パフォーマンスボトルネック: {top_bottleneck['service']}/{top_bottleneck['operation']} "
            f"(p95: {top_bottleneck['p95_ms']}ms, 合計時間: {top_bottleneck['total_time_sec']}秒)"
        )

    # HTTP 5xxエラー率
    high_5xx_services = [s for s in result.http_status_distribution if s.get("error_5xx_rate", 0) > 1]
    if high_5xx_services:
        for svc in high_5xx_services[:3]:
            result.warnings.append(
                f"HTTP 5xxエラー率が高い: {svc['service']} "
                f"({svc['error_5xx_rate']}% / {svc['total_requests']}リクエスト)"
            )

    # SLO違反
    if result.slo_violations:
        violation_count = len(result.slo_violations)
        affected_services = {v["service"] for v in result.slo_violations}
        result.critical_issues.append(
            f"SLO違反を検出: {violation_count}期間でエラー率 >{thresholds.slo_error_rate_threshold}% "
            f"({len(affected_services)}サービスに影響)"
        )

    # エラースパン
    if result.error_spans:
        top_error_span = result.error_spans[0]
        result.warnings.append(
            f"トレースエラー検出: {top_error_span['service']}の{top_error_span['operation']} "
            f"({top_error_span['error_count']}件)"
        )

    # サービス間依存関係のエラー率
    high_error_deps = [
        d
        for d in result.service_dependencies
        if d.get("call_count", 0) > 10 and d.get("error_count", 0) > 0 and (d["error_count"] / d["call_count"]) > 0.05
    ]
    if high_error_deps:
        for dep in high_error_deps[:2]:
            error_pct = round(dep["error_count"] / dep["call_count"] * 100, 1)
            result.warnings.append(
                f"サービス間呼び出しエラー率が高い: {dep['caller']} → {dep['callee']} "
                f"({error_pct}%エラー、{dep['call_count']}呼び出し)"
            )

    # ログ量の異常
    if result.log_volume_trends:
        service_volumes: dict[str, list[int]] = {}
        for trend in result.log_volume_trends:
            svc = trend.get("service", "")
            if svc:
                service_volumes.setdefault(svc, []).append(trend.get("log_count", 0))

        for svc, volumes in service_volumes.items():
            if len(volumes) >= 2:
                recent = volumes[0]
                previous = volumes[1]
                if previous > 0 and recent > previous * 2:
                    result.warnings.append(
                        f"ログ量スパイク検出: {svc} "
                        f"({recent}件 vs 前時間{previous}件、{round(recent / previous, 1)}倍増加)"
                    )


def _generate_recommendations(result: AnalysisResult, thresholds: HealthThresholds) -> None:
    """分析結果から推奨事項を生成"""
    # 遅いAPI
    slow_apis = [a for a in result.api_performance if a.get("p95_ms", 0) > thresholds.latency_warning_ms]
    if slow_apis:
        result.recommendations.append(
            f"遅いエンドポイントの最適化: {len(slow_apis)}件のAPIがp95 > {thresholds.latency_warning_ms}ms。"
            "キャッシュ、クエリ最適化、非同期処理を検討してください。"
        )

    # トップエラー
    if result.error_types:
        top_error = result.error_types[0]
        result.recommendations.append(
            f"主要エラーの調査: {top_error['service']}の{top_error['error_type']} ({top_error['error_count']}件発生)"
        )

    # ログ停止サービス
    stale_services = [
        s for s in result.service_stats if s.get("minutes_since_last_log", 0) > thresholds.log_gap_warning_min
    ]
    if stale_services:
        names = ", ".join(s["service_name"] for s in stale_services[:3])
        result.recommendations.append(f"ログ停止サービスの確認: {names}")
