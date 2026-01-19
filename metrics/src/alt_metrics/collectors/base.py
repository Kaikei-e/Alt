"""ベースコレクター: レガシーlogsテーブルからのサービス統計

構造化ログを使用したエラーハンドリングを実装しています。
"""

from __future__ import annotations

from typing import Any

import structlog
from clickhouse_connect.driver.client import Client

from alt_metrics.exceptions import CollectorError

logger = structlog.get_logger()


def collect_service_stats(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """レガシーlogsテーブルからサービス統計を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        サービス統計のリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        service_name,
        count() as total_logs,
        countIf(level IN ('Error', 'Fatal')) as error_count,
        countIf(level = 'Warn') as warn_count,
        round(countIf(level IN ('Error', 'Fatal')) / count() * 100, 3) as error_rate,
        max(timestamp) as last_seen,
        dateDiff('minute', max(timestamp), now()) as minutes_since_last_log
    FROM {database}.logs
    WHERE timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY service_name
    ORDER BY error_rate DESC, total_logs DESC
    """
    log = logger.bind(collector="service_stats", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("service_stats", str(e)) from e


def collect_error_trends(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """レガシーlogsテーブルから時間別エラートレンドを収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        エラートレンドのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        toStartOfHour(timestamp) as hour,
        service_name,
        countIf(level IN ('Error', 'Fatal')) as error_count,
        count() as total_count,
        round(countIf(level IN ('Error', 'Fatal')) / count() * 100, 2) as error_rate
    FROM {database}.logs
    WHERE timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY hour, service_name
    HAVING total_count > 0
    ORDER BY hour DESC, error_count DESC
    """
    log = logger.bind(collector="error_trends", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("error_trends", str(e)) from e
