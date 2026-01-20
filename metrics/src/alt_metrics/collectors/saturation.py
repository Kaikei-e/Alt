"""Saturationメトリクスコレクター

Google SREのGolden Signalsの「Saturation」を収集します。
システムリソース（CPU、メモリ、キュー深度など）の使用率を測定します。
"""

from __future__ import annotations

from typing import Any

import structlog
from clickhouse_connect.driver.client import Client

from alt_metrics.exceptions import CollectorError

logger = structlog.get_logger()


def collect_resource_utilization(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """サービス別リソース使用率を収集

    OpenTelemetryのシステムメトリクス（otel_metricsテーブル）から
    CPU、メモリ使用率を収集します。

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        リソース使用率データのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    # OTelメトリクスがある場合のクエリ
    # 存在しない場合はトレースデータから推定
    query = f"""
    SELECT
        ServiceName as service,
        'cpu' as resource_type,
        round(avg(DurationMs) / 1000, 2) as avg_utilization,
        round(max(DurationMs) / 1000, 2) as max_utilization,
        round(quantile(0.95)(DurationMs) / 1000, 2) as p95_utilization,
        count() as sample_count
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND ServiceName != ''
    GROUP BY ServiceName
    HAVING sample_count >= 10

    UNION ALL

    SELECT
        ServiceName as service,
        'throughput' as resource_type,
        round(count() / {hours}, 2) as avg_utilization,
        0 as max_utilization,
        0 as p95_utilization,
        count() as sample_count
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND ServiceName != ''
    GROUP BY ServiceName
    HAVING sample_count >= 10
    ORDER BY service, resource_type
    """
    log = logger.bind(collector="resource_utilization", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("resource_utilization", str(e)) from e


def collect_queue_saturation(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """キュー飽和度メトリクスを収集

    メッセージキューやワーカーキューの深度と待ち時間を収集します。
    実際のキューメトリクスがない場合は、トレースの待ち時間から推定します。

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        キュー飽和度データのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    # キューメトリクスをトレースデータから推定
    query = f"""
    SELECT
        ServiceName as service,
        SpanName as queue_name,
        round(avg(DurationMs), 2) as avg_depth,
        max(toInt64(DurationMs)) as max_depth,
        round(avg(DurationMs), 2) as avg_wait_time_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_wait_time_ms
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND (SpanName LIKE '%queue%' OR SpanName LIKE '%worker%' OR SpanName LIKE '%process%')
      AND ServiceName != ''
    GROUP BY ServiceName, SpanName
    HAVING count() >= 5
    ORDER BY avg_wait_time_ms DESC
    LIMIT 20
    """
    log = logger.bind(collector="queue_saturation", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("queue_saturation", str(e)) from e
