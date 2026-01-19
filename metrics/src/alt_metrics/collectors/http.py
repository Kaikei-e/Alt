"""HTTPリクエストメトリクスコレクター

otel_http_requestsテーブルからHTTPパフォーマンスデータを収集します。
"""

from __future__ import annotations

from typing import Any

import structlog
from clickhouse_connect.driver.client import Client

from alt_metrics.exceptions import CollectorError

logger = structlog.get_logger()


def collect_http_endpoint_stats(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """HTTPエンドポイント詳細統計を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        HTTPエンドポイント統計のリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        ServiceName as service,
        HttpRoute as route,
        count() as request_count,
        round(avg(RequestDuration), 2) as avg_duration_ms,
        round(quantile(0.95)(RequestDuration), 2) as p95_duration_ms,
        round(avg(ResponseSize), 0) as avg_response_size,
        round(countIf(HttpStatusCode >= 400) / count() * 100, 2) as error_rate,
        countIf(HttpStatusCode >= 200 AND HttpStatusCode < 300) as status_2xx,
        countIf(HttpStatusCode >= 400 AND HttpStatusCode < 500) as status_4xx,
        countIf(HttpStatusCode >= 500) as status_5xx
    FROM {database}.otel_http_requests
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND HttpRoute != ''
    GROUP BY ServiceName, HttpRoute
    ORDER BY request_count DESC
    LIMIT 30
    """
    log = logger.bind(collector="http_endpoint_stats", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("http_endpoint_stats", str(e)) from e


def collect_http_status_distribution(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """サービス別HTTPステータスコード分布を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        HTTPステータス分布データのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        ServiceName as service,
        count() as total_requests,
        countIf(HttpStatusCode >= 200 AND HttpStatusCode < 300) as status_2xx,
        countIf(HttpStatusCode >= 300 AND HttpStatusCode < 400) as status_3xx,
        countIf(HttpStatusCode >= 400 AND HttpStatusCode < 500) as status_4xx,
        countIf(HttpStatusCode >= 500) as status_5xx,
        round(countIf(HttpStatusCode >= 500) / count() * 100, 2) as error_5xx_rate
    FROM {database}.otel_http_requests
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY ServiceName
    ORDER BY total_requests DESC
    """
    log = logger.bind(collector="http_status_distribution", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("http_status_distribution", str(e)) from e
