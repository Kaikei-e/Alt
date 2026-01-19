"""SLI/SLOメトリクスコレクター

sli_metricsテーブルからSLI/SLOデータを収集します。
"""

from __future__ import annotations

from typing import Any

import structlog
from clickhouse_connect.driver.client import Client

from alt_metrics.exceptions import CollectorError

logger = structlog.get_logger()


def collect_sli_trends(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """SLIメトリクストレンド（error_rate, log_throughput）を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        SLIトレンドデータのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        toStartOfFiveMinutes(Timestamp) as time_bucket,
        ServiceName as service,
        Metric as metric,
        round(avg(Value), 4) as value
    FROM {database}.sli_metrics
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND Metric IN ('error_rate', 'log_throughput')
    GROUP BY time_bucket, ServiceName, Metric
    ORDER BY time_bucket DESC, ServiceName, Metric
    LIMIT 500
    """
    log = logger.bind(collector="sli_trends", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("sli_trends", str(e)) from e


def collect_slo_violations(
    client: Client, database: str, hours: int, error_rate_threshold: float = 1.0
) -> list[dict[str, Any]]:
    """SLO違反（エラー率が閾値を超過）を検出

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）
        error_rate_threshold: エラー率閾値（%）

    Returns:
        SLO違反データのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = f"""
    SELECT
        ServiceName as service,
        toStartOfFiveMinutes(Timestamp) as time_bucket,
        round(avg(Value) * 100, 2) as error_rate_pct,
        count() as sample_count
    FROM {database}.sli_metrics
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND Metric = 'error_rate'
    GROUP BY ServiceName, time_bucket
    HAVING avg(Value) > {error_rate_threshold / 100}
    ORDER BY time_bucket DESC, error_rate_pct DESC
    LIMIT 50
    """
    log = logger.bind(
        collector="slo_violations",
        database=database,
        hours=hours,
        threshold=error_rate_threshold,
    )
    log.debug("クエリ実行開始")

    try:
        result = client.query(query)
        data = [dict(zip(result.column_names, row)) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except Exception as e:
        log.error("クエリ実行エラー", error=str(e), query=query[:200])
        raise CollectorError("slo_violations", str(e)) from e
