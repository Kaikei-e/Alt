"""OpenTelemetryトレースデータコレクター

otel_tracesテーブルからパフォーマンス・エラーデータを収集します。
"""

import structlog
from clickhouse_connect.driver.client import Client
from clickhouse_connect.driver.exceptions import ClickHouseError

from alt_metrics.exceptions import CollectorError
from alt_metrics.models import (
    ApiPerformanceStats,
    Bottleneck,
    ErrorSpan,
    ServiceDependency,
    SpanTypeStat,
)

logger = structlog.get_logger()


def collect_api_performance(client: Client, database: str, hours: int) -> list[ApiPerformanceStats]:
    """トレースからAPIエンドポイントパフォーマンスを収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        APIパフォーマンスデータのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = """
    SELECT
        ServiceName as service,
        SpanName as endpoint,
        count() as request_count,
        round(avg(DurationMs), 2) as avg_ms,
        round(quantile(0.50)(DurationMs), 2) as p50_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_ms,
        round(quantile(0.99)(DurationMs), 2) as p99_ms,
        round(max(DurationMs), 2) as max_ms,
        countIf(StatusCode = 'ERROR') as error_spans
    FROM {database:Identifier}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours:UInt32} HOUR
      AND SpanName != ''
    GROUP BY ServiceName, SpanName
    HAVING request_count >= 5
    ORDER BY p95_ms DESC
    LIMIT 30
    """
    log = logger.bind(collector="api_performance", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query, parameters={"database": database, "hours": hours})
        data = [ApiPerformanceStats(**dict(zip(result.column_names, row))) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except ClickHouseError as e:
        log.exception("クエリ実行エラー", query=query[:200])
        raise CollectorError("api_performance", str(e)) from e


def collect_bottlenecks(client: Client, database: str, hours: int) -> list[Bottleneck]:
    """パフォーマンスボトルネック（1秒超の操作）を特定

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        ボトルネックデータのリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = """
    SELECT
        ServiceName as service,
        SpanName as operation,
        count() as occurrences,
        round(avg(DurationMs), 2) as avg_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_ms,
        round(sum(DurationMs) / 1000, 2) as total_time_sec
    FROM {database:Identifier}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours:UInt32} HOUR
      AND DurationMs > 1000
    GROUP BY ServiceName, SpanName
    HAVING occurrences >= 3
    ORDER BY total_time_sec DESC
    LIMIT 15
    """
    log = logger.bind(collector="bottlenecks", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query, parameters={"database": database, "hours": hours})
        data = [Bottleneck(**dict(zip(result.column_names, row))) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except ClickHouseError as e:
        log.exception("クエリ実行エラー", query=query[:200])
        raise CollectorError("bottlenecks", str(e)) from e


def collect_span_type_stats(client: Client, database: str, hours: int) -> list[SpanTypeStat]:
    """トレーススパン種類別統計を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        スパン種類別統計のリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = """
    SELECT
        ServiceName as service,
        SpanKind as span_kind,
        count() as span_count,
        round(avg(DurationMs), 2) as avg_duration_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_duration_ms,
        countIf(StatusCode = 'ERROR') as error_count
    FROM {database:Identifier}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours:UInt32} HOUR
    GROUP BY ServiceName, SpanKind
    ORDER BY span_count DESC
    """
    log = logger.bind(collector="span_type_stats", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query, parameters={"database": database, "hours": hours})
        data = [SpanTypeStat(**dict(zip(result.column_names, row))) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except ClickHouseError as e:
        log.exception("クエリ実行エラー", query=query[:200])
        raise CollectorError("span_type_stats", str(e)) from e


def collect_error_spans(client: Client, database: str, hours: int) -> list[ErrorSpan]:
    """エラースパンの詳細情報を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        エラースパン情報のリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = """
    SELECT
        ServiceName as service,
        SpanName as operation,
        StatusMessage as error_message,
        count() as error_count,
        round(avg(DurationMs), 2) as avg_duration_ms,
        formatDateTime(max(Timestamp), '%Y-%m-%d %H:%M:%S') as last_occurrence
    FROM {database:Identifier}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours:UInt32} HOUR
      AND StatusCode = 'ERROR'
    GROUP BY ServiceName, SpanName, StatusMessage
    ORDER BY error_count DESC
    LIMIT 20
    """
    log = logger.bind(collector="error_spans", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query, parameters={"database": database, "hours": hours})
        data = [ErrorSpan(**dict(zip(result.column_names, row))) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except ClickHouseError as e:
        log.exception("クエリ実行エラー", query=query[:200])
        raise CollectorError("error_spans", str(e)) from e


def collect_service_dependencies(client: Client, database: str, hours: int) -> list[ServiceDependency]:
    """サービス間の呼び出し依存関係を収集

    Args:
        client: ClickHouseクライアント
        database: データベース名
        hours: 分析対象期間（時間）

    Returns:
        サービス依存関係のリスト

    Raises:
        CollectorError: クエリ実行に失敗した場合
    """
    query = """
    SELECT
        s1.ServiceName as caller,
        s2.ServiceName as callee,
        count() as call_count,
        round(avg(s1.DurationMs), 2) as avg_duration_ms,
        round(quantile(0.95)(s1.DurationMs), 2) as p95_duration_ms,
        countIf(s1.StatusCode = 'ERROR') as error_count
    FROM {database:Identifier}.otel_traces s1
    JOIN {database:Identifier}.otel_traces s2
        ON s1.TraceId = s2.TraceId AND s1.SpanId = s2.ParentSpanId
    WHERE s1.Timestamp >= now() - INTERVAL {hours:UInt32} HOUR
      AND s1.ServiceName != s2.ServiceName
    GROUP BY s1.ServiceName, s2.ServiceName
    ORDER BY call_count DESC
    LIMIT 20
    """
    log = logger.bind(collector="service_dependencies", database=database, hours=hours)
    log.debug("クエリ実行開始")

    try:
        result = client.query(query, parameters={"database": database, "hours": hours})
        data = [ServiceDependency(**dict(zip(result.column_names, row))) for row in result.result_rows]
        log.info("データ収集完了", count=len(data))
        return data
    except ClickHouseError as e:
        log.exception("クエリ実行エラー", query=query[:200])
        raise CollectorError("service_dependencies", str(e)) from e
