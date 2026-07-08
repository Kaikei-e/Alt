"""analysis.py のテスト"""

from __future__ import annotations

import pytest

from alt_metrics.analysis import (
    analyze_health,
    calculate_health_score,
    get_health_status,
    get_health_status_emoji,
)
from alt_metrics.config import HealthThresholds
from alt_metrics.models import AnalysisResult, ApiPerformanceStats


class TestCalculateHealthScore:
    """calculate_health_score関数のテスト"""

    @pytest.fixture
    def thresholds(self) -> HealthThresholds:
        return HealthThresholds()

    def test_perfect_health_returns_100(self, thresholds: HealthThresholds) -> None:
        """完璧な状態では100を返す"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 100

    def test_critical_error_rate_reduces_score_by_40(self, thresholds: HealthThresholds) -> None:
        """エラー率が10%超の場合、40点減点"""
        score = calculate_health_score(
            error_rate=15.0,  # > 10%
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 60

    def test_high_error_rate_reduces_score_by_25(self, thresholds: HealthThresholds) -> None:
        """エラー率が5%超の場合、25点減点"""
        score = calculate_health_score(
            error_rate=7.0,  # > 5%
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 75

    def test_warning_error_rate_reduces_score_by_10(self, thresholds: HealthThresholds) -> None:
        """エラー率が1%超の場合、10点減点"""
        score = calculate_health_score(
            error_rate=2.0,  # > 1%
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 90

    def test_minor_error_rate_reduces_score_by_5(self, thresholds: HealthThresholds) -> None:
        """エラー率が0.5%超の場合、5点減点"""
        score = calculate_health_score(
            error_rate=0.7,  # > 0.5%
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 95

    def test_critical_latency_reduces_score_by_30(self, thresholds: HealthThresholds) -> None:
        """レイテンシが10秒超の場合、30点減点"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=15000.0,  # > 10000ms
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 70

    def test_high_latency_reduces_score_by_20(self, thresholds: HealthThresholds) -> None:
        """レイテンシが5秒超の場合、20点減点"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=7000.0,  # > 5000ms
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 80

    def test_warning_latency_reduces_score_by_10(self, thresholds: HealthThresholds) -> None:
        """レイテンシが1秒超の場合、10点減点"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=2000.0,  # > 1000ms
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 90

    def test_minor_latency_reduces_score_by_5(self, thresholds: HealthThresholds) -> None:
        """レイテンシが500ms超の場合、5点減点"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=700.0,  # > 500ms
            log_gap_minutes=0.0,
            thresholds=thresholds,
        )
        assert score == 95

    def test_critical_log_gap_reduces_score_by_30(self, thresholds: HealthThresholds) -> None:
        """ログ欠落が10分超の場合、30点減点"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=100.0,
            log_gap_minutes=15.0,  # > 10分
            thresholds=thresholds,
        )
        assert score == 70

    def test_warning_log_gap_reduces_score_by_15(self, thresholds: HealthThresholds) -> None:
        """ログ欠落が5分超の場合、15点減点"""
        score = calculate_health_score(
            error_rate=0.0,
            p95_ms=100.0,
            log_gap_minutes=7.0,  # > 5分
            thresholds=thresholds,
        )
        assert score == 85

    def test_multiple_issues_accumulate(self, thresholds: HealthThresholds) -> None:
        """複数の問題は累積して減点"""
        score = calculate_health_score(
            error_rate=15.0,  # -40
            p95_ms=15000.0,  # -30
            log_gap_minutes=15.0,  # -30
            thresholds=thresholds,
        )
        assert score == 0  # 0にクランプ

    def test_score_never_goes_below_zero(self, thresholds: HealthThresholds) -> None:
        """スコアは0未満にならない"""
        score = calculate_health_score(
            error_rate=100.0,
            p95_ms=100000.0,
            log_gap_minutes=100.0,
            thresholds=thresholds,
        )
        assert score == 0

    def test_custom_thresholds_are_used(self) -> None:
        """カスタム閾値が使用される"""
        custom = HealthThresholds(
            error_rate_critical=50.0,  # より緩い閾値
            error_rate_high=40.0,
            error_rate_warning=30.0,
            error_rate_minor=20.0,
        )
        # 通常なら10%で-40点だが、カスタム閾値では減点なし
        score = calculate_health_score(
            error_rate=15.0,
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=custom,
        )
        assert score == 100

    def test_default_thresholds_when_none(self) -> None:
        """閾値がNoneの場合はデフォルト値を使用"""
        score = calculate_health_score(
            error_rate=15.0,  # > 10% (default critical)
            p95_ms=100.0,
            log_gap_minutes=0.0,
            thresholds=None,
        )
        assert score == 60


class TestGetHealthStatus:
    """get_health_status関数のテスト"""

    def test_score_90_or_above_is_healthy(self) -> None:
        """スコア90以上は「正常」"""
        assert get_health_status(90) == "正常"
        assert get_health_status(95) == "正常"
        assert get_health_status(100) == "正常"

    def test_score_70_to_89_is_warning(self) -> None:
        """スコア70-89は「警告」"""
        assert get_health_status(70) == "警告"
        assert get_health_status(80) == "警告"
        assert get_health_status(89) == "警告"

    def test_score_50_to_69_is_degraded(self) -> None:
        """スコア50-69は「劣化」"""
        assert get_health_status(50) == "劣化"
        assert get_health_status(60) == "劣化"
        assert get_health_status(69) == "劣化"

    def test_score_below_50_is_critical(self) -> None:
        """スコア50未満は「危険」"""
        assert get_health_status(49) == "危険"
        assert get_health_status(30) == "危険"
        assert get_health_status(0) == "危険"


class TestGetHealthStatusEmoji:
    """get_health_status_emoji関数のテスト"""

    def test_healthy_emoji(self) -> None:
        """正常状態の絵文字"""
        assert get_health_status_emoji("正常") == "✅"

    def test_warning_emoji(self) -> None:
        """警告状態の絵文字"""
        assert get_health_status_emoji("警告") == "⚠️"

    def test_degraded_emoji(self) -> None:
        """劣化状態の絵文字"""
        assert get_health_status_emoji("劣化") == "🔶"

    def test_critical_emoji(self) -> None:
        """危険状態の絵文字"""
        assert get_health_status_emoji("危険") == "🔴"

    def test_unknown_status_returns_empty(self) -> None:
        """不明なステータスは空文字"""
        assert get_health_status_emoji("unknown") == ""


class TestAnalyzeHealthServiceLatencyAggregation:
    """analyze_health関数のservice_latencies集約テスト

    api_performanceはサービス×エンドポイント単位・p95降順で格納されるため、
    同一サービスに複数エンドポイントがある場合の集約方法を検証する。
    """

    def _make_result(self, api_performance: list[ApiPerformanceStats]) -> AnalysisResult:
        return AnalysisResult(
            hours_analyzed=24,
            service_stats=[
                {
                    "service_name": "alt-backend",
                    "total_logs": 100,
                    "error_count": 0,
                    "error_rate": 0.0,
                    "minutes_since_last_log": 0,
                },
            ],
            api_performance=api_performance,
        )

    def test_multi_endpoint_service_uses_worst_case_p95_not_last_row(self) -> None:
        """同一サービスの複数エンドポイントがある場合、最後の行ではなく最大p95を採用する"""
        # ClickHouseはp95降順で返すため、後続行ほどp95が小さい。
        # 素朴なdict内包表記だと最後に処理された（=最小の）p95で上書きされてしまう。
        result = self._make_result(
            [
                ApiPerformanceStats(
                    service="alt-backend",
                    endpoint="GET /slow",
                    request_count=10,
                    avg_ms=100.0,
                    p50_ms=90.0,
                    p95_ms=9000.0,
                    p99_ms=9500.0,
                    max_ms=9800.0,
                    error_spans=0,
                ),
                ApiPerformanceStats(
                    service="alt-backend",
                    endpoint="GET /fast",
                    request_count=1000,
                    avg_ms=10.0,
                    p50_ms=8.0,
                    p95_ms=20.0,
                    p99_ms=25.0,
                    max_ms=30.0,
                    error_spans=0,
                ),
            ]
        )

        analyze_health(result)

        assert len(result.service_health) == 1
        assert result.service_health[0].p95_latency_ms == 9000.0

    def test_single_endpoint_service_uses_its_p95(self) -> None:
        """エンドポイントが1つの場合はそのままp95を使う"""
        result = self._make_result(
            [
                ApiPerformanceStats(
                    service="alt-backend",
                    endpoint="GET /api/health",
                    request_count=100,
                    avg_ms=50.0,
                    p50_ms=40.0,
                    p95_ms=200.0,
                    p99_ms=500.0,
                    max_ms=600.0,
                    error_spans=0,
                ),
            ]
        )

        analyze_health(result)

        assert result.service_health[0].p95_latency_ms == 200.0

    def test_no_api_performance_data_defaults_to_zero(self) -> None:
        """api_performanceが空の場合はp95レイテンシ0として扱う"""
        result = self._make_result([])

        analyze_health(result)

        assert result.service_health[0].p95_latency_ms == 0


class TestAnalyzeHealthIssuesWarningsRecommendations:
    """analyze_health関数の critical_issues / warnings / recommendations 生成テスト"""

    def test_low_health_score_generates_critical_issue(self) -> None:
        """健全性スコアが50未満のサービスはcritical_issuesに追加される"""
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[
                {
                    "service_name": "auth-hub",
                    "total_logs": 1000,
                    "error_count": 300,
                    "error_rate": 30.0,
                    "minutes_since_last_log": 20,
                },
            ],
        )

        analyze_health(result)

        assert result.service_health[0].health_score < 50
        assert any("auth-hub" in issue for issue in result.critical_issues)

    def test_high_error_rate_generates_warning(self) -> None:
        """エラー率が閾値超のサービスはwarningsに追加される"""
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[
                {
                    "service_name": "alt-backend",
                    "total_logs": 1000,
                    "error_count": 20,
                    "error_rate": 2.0,
                    "minutes_since_last_log": 0,
                },
            ],
        )

        analyze_health(result)

        assert any("alt-backend" in w for w in result.warnings)

    def test_slo_violations_generate_critical_issue(self) -> None:
        """SLO違反はcritical_issuesに追加される"""
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[],
            slo_violations=[
                {"service": "alt-backend", "time_bucket": "2026-01-19 12:00:00", "error_rate_pct": 5.0},
            ],
        )

        analyze_health(result)

        assert any("SLO違反" in issue for issue in result.critical_issues)

    def test_bottlenecks_generate_warning(self) -> None:
        """ボトルネックはwarningsに追加される"""
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[],
            bottlenecks=[
                {"service": "auth-hub", "operation": "authenticate", "p95_ms": 3500.0, "total_time_sec": 100.0},
            ],
        )

        analyze_health(result)

        assert any("auth-hub" in w and "authenticate" in w for w in result.warnings)

    def test_slow_apis_generate_recommendation(self) -> None:
        """p95が閾値超のAPIは最適化推奨に追加される"""
        thresholds = HealthThresholds()
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[],
            api_performance=[
                ApiPerformanceStats(
                    service="alt-backend",
                    endpoint="GET /slow",
                    request_count=10,
                    avg_ms=1500.0,
                    p50_ms=1400.0,
                    p95_ms=thresholds.latency_warning_ms + 1,
                    p99_ms=2000.0,
                    max_ms=2500.0,
                    error_spans=0,
                ),
            ],
        )

        analyze_health(result, thresholds)

        assert any("遅いエンドポイント" in rec for rec in result.recommendations)

    def test_stale_services_generate_recommendation(self) -> None:
        """ログ停止サービスは確認推奨に追加される"""
        thresholds = HealthThresholds()
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[
                {
                    "service_name": "search-indexer",
                    "total_logs": 100,
                    "error_count": 0,
                    "error_rate": 0.0,
                    "minutes_since_last_log": thresholds.log_gap_warning_min + 1,
                },
            ],
        )

        analyze_health(result, thresholds)

        assert any("search-indexer" in rec for rec in result.recommendations)

    def test_no_issues_when_all_healthy(self) -> None:
        """全サービスが健全な場合はcritical_issuesもwarningsも空"""
        result = AnalysisResult(
            hours_analyzed=24,
            service_stats=[
                {
                    "service_name": "alt-backend",
                    "total_logs": 1000,
                    "error_count": 0,
                    "error_rate": 0.0,
                    "minutes_since_last_log": 0,
                },
            ],
        )

        analyze_health(result)

        assert result.critical_issues == []
        assert result.warnings == []
        assert result.overall_health_score == 100
