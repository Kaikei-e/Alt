"""OpenTelemetryログデータコレクター

otel_logsおよびotel_error_logsテーブルからログデータを収集します。
"""

from __future__ import annotations

from typing import Any

import structlog
from clickhouse_connect.driver.client import Client

from alt_metrics.exceptions import CollectorError

logger = structlog.get_logger()


def collect_error_types(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """エラーログからエラー種類と頻度を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        エラー種類データのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        ServiceName as service,
        if(ExceptionType = '', 'Unknown', ExceptionType) as error_type,
        count() as error_count,
        any(substring(Body, 1, 150)) as sample_message
    FROM {database}.otel_error_logs
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY ServiceName, ExceptionType
    ORDER BY error_count DESC
    LIMIT 20
    """
    log = logger.bind(collector="error_types", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("error_types", str(e)) from e


def collect_recent_errors(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """最新のエラーログを収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        最新エラーログのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        ServiceName as service,
        SeverityText as level,
        substring(Body, 1, 200) as message,
        if(ExceptionType = '', '-', ExceptionType) as error_type,
        formatDateTime(Timestamp, '%Y-%m-%d %H:%M:%S') as timestamp
    FROM {database}.otel_error_logs
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    ORDER BY Timestamp DESC
    LIMIT 25
    """
    log = logger.bind(collector="recent_errors", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("recent_errors", str(e)) from e


def collect_log_severity_distribution(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """サービス別ログ重要度分布を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        ログ重要度分布データのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        ServiceName as service,
        count() as total_logs,
        countIf(SeverityText = 'DEBUG' OR SeverityNumber <= 4) as debug_count,
        countIf(SeverityText = 'INFO' OR (SeverityNumber > 4 AND SeverityNumber <= 8)) as info_count,
        countIf(SeverityText IN ('WARN', 'WARNING') OR (SeverityNumber > 8 AND SeverityNumber <= 12)) as warn_count,
        countIf(SeverityText = 'ERROR' OR (SeverityNumber > 12 AND SeverityNumber <= 16)) as error_count,
        countIf(SeverityText IN ('FATAL', 'CRITICAL') OR SeverityNumber > 20) as fatal_count,
        round(countIf(SeverityNumber >= 17) / count() * 100, 2) as error_rate
    FROM {database}.otel_logs
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY ServiceName
    ORDER BY total_logs DESC
    """
    log = logger.bind(collector="log_severity_distribution", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("log_severity_distribution", str(e)) from e


def collect_log_volume_trends(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """時間別ログ量トレンドを収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        ログ量トレンドデータのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        toStartOfHour(Timestamp) as hour,
        ServiceName as service,
        count() as log_count,
        countIf(SeverityNumber >= 17) as error_count,
        round(countIf(SeverityNumber >= 17) / count() * 100, 2) as error_rate
    FROM {database}.otel_logs
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY hour, ServiceName
    ORDER BY hour DESC, log_count DESC
    """
    log = logger.bind(collector="log_volume_trends", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("log_volume_trends", str(e)) from e
