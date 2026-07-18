"""CLIコマンド処理

analyzeとvalidateコマンドを提供します。
"""

import argparse
import sys
from contextlib import closing
from datetime import UTC, datetime
from pathlib import Path

import clickhouse_connect
import structlog
from clickhouse_connect.driver.exceptions import ClickHouseError

from alt_metrics.analysis import analyze_health
from alt_metrics.collectors import (
    collect_api_performance,
    collect_bottlenecks,
    collect_error_spans,
    collect_error_trends,
    collect_error_types,
    collect_http_endpoint_stats,
    collect_http_status_distribution,
    collect_log_severity_distribution,
    collect_log_volume_trends,
    collect_queue_saturation,
    collect_recent_errors,
    collect_resource_utilization,
    collect_service_dependencies,
    collect_service_stats,
    collect_sli_trends,
    collect_slo_violations,
    collect_span_type_stats,
)
from alt_metrics.config import AppConfig, ClickHouseConfig, HealthThresholds
from alt_metrics.exceptions import ClickHouseConnectionError, CollectorError, MetricsError
from alt_metrics.models import AnalysisResult
from alt_metrics.reports import generate_japanese_report

logger = structlog.get_logger()


def configure_logging(verbose: bool = False) -> None:
    """structlogの設定"""
    structlog.configure(
        processors=[
            structlog.stdlib.add_log_level,
            structlog.stdlib.PositionalArgumentsFormatter(),
            structlog.processors.TimeStamper(fmt="iso"),
            structlog.processors.StackInfoRenderer(),
            structlog.processors.format_exc_info,
            structlog.dev.ConsoleRenderer() if verbose else structlog.processors.JSONRenderer(),
        ],
        wrapper_class=structlog.stdlib.BoundLogger,
        context_class=dict,
        logger_factory=structlog.PrintLoggerFactory(),
        cache_logger_on_first_use=True,
    )


def run_analysis(
    config: ClickHouseConfig,
    thresholds: HealthThresholds,
    hours: int,
    verbose: bool = False,
) -> AnalysisResult:
    """分析を実行して結果を返す

    Args:
        config: ClickHouse接続設定
        thresholds: 健全性閾値
        hours: 分析期間（時間）
        verbose: 詳細出力フラグ

    Returns:
        分析結果

    Raises:
        ClickHouseConnectionError: 接続に失敗した場合
        CollectorError: データ収集に失敗した場合
    """
    log = logger.bind(host=config.host, port=config.port, database=config.database)

    if verbose:
        log.info("ClickHouseに接続中", host=config.host, port=config.port)

    try:
        client = clickhouse_connect.get_client(
            host=config.host,
            port=config.port,
            username=config.user,
            password=config.password,
        )
    except ClickHouseError as e:
        log.exception("ClickHouse接続エラー")
        raise ClickHouseConnectionError(f"接続失敗: {e}") from e

    with closing(client):
        result = AnalysisResult(hours_analyzed=hours)
        db = config.database

        # コレクター定義: (名前, 属性名, コレクター関数)
        collector_defs = [
            ("サービス統計", "service_stats", collect_service_stats),
            ("エラートレンド", "error_trends", collect_error_trends),
            ("APIパフォーマンス", "api_performance", collect_api_performance),
            ("ボトルネック", "bottlenecks", collect_bottlenecks),
            ("エラー種類", "error_types", collect_error_types),
            ("最新エラー", "recent_errors", collect_recent_errors),
            ("HTTPエンドポイント統計", "http_endpoint_stats", collect_http_endpoint_stats),
            ("HTTPステータス分布", "http_status_distribution", collect_http_status_distribution),
            ("トレーススパン統計", "span_type_stats", collect_span_type_stats),
            ("エラースパン", "error_spans", collect_error_spans),
            ("サービス依存関係", "service_dependencies", collect_service_dependencies),
            ("ログ重要度分布", "log_severity_distribution", collect_log_severity_distribution),
            ("ログ量トレンド", "log_volume_trends", collect_log_volume_trends),
            ("SLIトレンド", "sli_trends", collect_sli_trends),
            ("リソース使用率", "resource_utilization", collect_resource_utilization),
            ("キュー飽和度", "queue_saturation", collect_queue_saturation),
        ]

        for name, attr, fn in collector_defs:
            if verbose:
                log.info("コレクター実行中", collector=name)
            try:
                result = result.model_copy(update={attr: fn(client, db, hours)})
            except CollectorError as e:
                log.warning("コレクターエラー（続行）", collector=name, error=str(e))
                if verbose:
                    log.info("コレクター収集失敗", collector=name, error=str(e))

        # SLO違反は追加パラメータが必要
        if verbose:
            log.info("コレクター実行中", collector="SLO違反")
        try:
            result = result.model_copy(
                update={
                    "slo_violations": collect_slo_violations(client, db, hours, thresholds.slo_error_rate_threshold)
                }
            )
        except CollectorError as e:
            log.warning("コレクターエラー（続行）", collector="SLO違反", error=str(e))
            if verbose:
                log.info("コレクター収集失敗", collector="SLO違反", error=str(e))

        if verbose:
            log.info("健全性分析と推奨事項を生成中")
        return analyze_health(result, thresholds)


def cmd_analyze(args: argparse.Namespace) -> int:
    """analyzeコマンドを実行"""
    configure_logging(args.verbose)
    config = AppConfig.from_env()

    try:
        result = run_analysis(
            config.clickhouse,
            config.thresholds,
            args.hours,
            args.verbose,
        )

        # レポート生成（日本語のみ対応）
        report = generate_japanese_report(result)

        # ファイル出力
        output_dir = args.output_dir or config.report.output_dir
        output_dir.mkdir(parents=True, exist_ok=True)
        timestamp = datetime.now(UTC).strftime("%Y%m%d_%H%M%S")
        output_file = output_dir / f"system_health_{timestamp}.md"
        output_file.write_text(report, encoding="utf-8")

        logger.info("レポート生成完了", output_file=str(output_file))

        if args.verbose:
            logger.info("レポート本文", report=report)

        return 0

    except MetricsError as e:
        print(f"エラー: {e}", file=sys.stderr)
        return 1
    except Exception as e:
        logger.exception("予期せぬエラー", error=str(e))
        print(f"予期せぬエラー: {e}", file=sys.stderr)
        return 1


def cmd_validate(args: argparse.Namespace) -> int:
    """validateコマンドを実行（ClickHouse接続テスト）"""
    configure_logging(args.verbose)
    config = ClickHouseConfig.from_env()
    log = logger.bind(host=config.host, port=config.port, database=config.database)

    log.info("ClickHouse接続テスト開始")

    try:
        client = clickhouse_connect.get_client(
            host=config.host,
            port=config.port,
            username=config.user,
            password=config.password,
        )

        # 接続テスト
        result = client.query("SELECT 1")
        if result.result_rows:
            log.info("接続成功")
        else:
            log.error("接続失敗: クエリ結果が空")
            return 1

        # テーブル存在確認（ホスト/ユーザー名はログに出さない）
        tables = ["logs", "otel_logs", "otel_traces", "otel_http_requests", "otel_error_logs", "sli_metrics"]
        for table in tables:
            try:
                result = client.query(
                    "SELECT count() FROM {database:Identifier}.{table:Identifier} LIMIT 1",
                    parameters={"database": config.database, "table": table},
                )
                count = result.result_rows[0][0] if result.result_rows else 0
                log.info("テーブル確認", table=table, row_count=count)
            except ClickHouseError as e:
                log.warning("テーブルアクセス不可", table=table, error=str(e))

        return 0

    except ClickHouseError as e:
        log.exception("接続失敗")
        print(f"❌ 接続失敗: {e}", file=sys.stderr)
        return 1


def create_parser() -> argparse.ArgumentParser:
    """CLIパーサーを作成"""
    parser = argparse.ArgumentParser(
        prog="alt-metrics",
        description="Alt システム健全性アナライザー - ClickHouseからメトリクスを分析してレポート生成",
    )
    subparsers = parser.add_subparsers(dest="command", help="利用可能なコマンド")

    # analyze コマンド
    analyze_parser = subparsers.add_parser(
        "analyze",
        help="システム健全性を分析してレポートを生成",
    )
    analyze_parser.add_argument(
        "--hours",
        type=int,
        default=24,
        help="分析期間（時間、デフォルト: 24）",
    )
    analyze_parser.add_argument(
        "--output-dir",
        type=Path,
        default=None,
        help="レポート出力先ディレクトリ",
    )
    analyze_parser.add_argument(
        "--lang",
        choices=["ja"],
        default="ja",
        help="レポート言語（現在は ja のみ対応）",
    )
    analyze_parser.add_argument(
        "--verbose",
        action="store_true",
        help="詳細出力を有効化",
    )

    # validate コマンド
    validate_parser = subparsers.add_parser(
        "validate",
        help="ClickHouse接続をテスト",
    )
    validate_parser.add_argument(
        "--verbose",
        action="store_true",
        help="詳細出力を有効化",
    )

    return parser
