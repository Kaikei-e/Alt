#!/usr/bin/env python3
"""Alt System Health Analyzer.

Analyzes application logs and traces stored in ClickHouse to measure
the health and performance of the Alt platform.

Usage:
    uv run python analyze_clickhouse_performance.py
    uv run python analyze_clickhouse_performance.py --hours 48
    uv run python analyze_clickhouse_performance.py --output-dir ./reports
"""

from __future__ import annotations

import argparse
import contextlib
import logging
import os
import re
import sys
import traceback
from collections.abc import Callable
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

import clickhouse_connect
from clickhouse_connect.driver.client import Client
from clickhouse_connect.driver.exceptions import ClickHouseError

# =============================================================================
# Configuration
# =============================================================================

_IDENTIFIER_RE = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
logger = logging.getLogger(__name__)

# Health scoring and reporting thresholds
ERROR_RATE_CRITICAL = 10
ERROR_RATE_HIGH = 5
ERROR_RATE_LOW = 0.5
LATENCY_CRITICAL_MS = 10000
LATENCY_HIGH_MS = 5000
LATENCY_WARN_MS = 1000
LATENCY_LOW_MS = 500
LOG_GAP_CRITICAL_MIN = 10
LOG_GAP_WARN_MIN = 5
HEALTH_HEALTHY_MIN = 90
HEALTH_WARNING_MIN = 70
HEALTH_DEGRADED_MIN = 50
MIN_DEP_CALL_COUNT = 10
DEP_ERROR_RATE_THRESHOLD = 0.05
MIN_VOLUME_SAMPLES = 2
LOG_VOLUME_SPIKE_FACTOR = 2


def _validate_identifier(name: str) -> str:
    """Validate a ClickHouse database/table identifier against _IDENTIFIER_RE."""
    if not _IDENTIFIER_RE.match(name):
        msg = f"Invalid SQL identifier: {name!r}"
        raise ValueError(msg)
    return name


@dataclass
class ClickHouseConfig:
    """ClickHouse connection configuration."""

    host: str = "localhost"
    port: int = 8123
    user: str = "default"
    password: str = ""
    database: str = "rask_logs"

    @classmethod
    def from_env(cls) -> ClickHouseConfig:
        """Load configuration from environment variables."""

        def get_env_or_file(name: str, default: str = "") -> str:
            file_path = os.getenv(f"{name}_FILE")
            if file_path and Path(file_path).exists():
                return Path(file_path).read_text().strip()
            return os.getenv(name, default)

        database = os.getenv("APP_CLICKHOUSE_DATABASE", "rask_logs")
        if not _IDENTIFIER_RE.match(database):
            msg = f"Invalid APP_CLICKHOUSE_DATABASE identifier: {database!r}"
            raise ValueError(msg)

        return cls(
            host=os.getenv("APP_CLICKHOUSE_HOST", "localhost"),
            port=int(os.getenv("APP_CLICKHOUSE_PORT", "8123")),
            user=os.getenv("APP_CLICKHOUSE_USER", "default"),
            password=get_env_or_file("APP_CLICKHOUSE_PASSWORD", ""),
            database=database,
        )


@dataclass
class ServiceHealth:
    """Health data for a single service."""

    name: str
    total_logs: int = 0
    error_count: int = 0
    error_rate: float = 0.0
    last_seen: datetime | None = None
    p95_latency_ms: float = 0.0
    health_score: int = 100


@dataclass
class AnalysisResult:
    """Container for all analysis results."""

    generated_at: datetime = field(default_factory=datetime.now)
    hours_analyzed: int = 24

    # System health
    overall_health_score: int = 100
    service_health: list[ServiceHealth] = field(default_factory=list)

    # Service logs
    service_stats: list[dict[str, Any]] = field(default_factory=list)
    error_trends: list[dict[str, Any]] = field(default_factory=list)

    # API Performance
    api_performance: list[dict[str, Any]] = field(default_factory=list)
    bottlenecks: list[dict[str, Any]] = field(default_factory=list)

    # Error analysis
    error_types: list[dict[str, Any]] = field(default_factory=list)
    recent_errors: list[dict[str, Any]] = field(default_factory=list)

    # HTTP detailed analysis
    http_endpoint_stats: list[dict[str, Any]] = field(default_factory=list)
    http_status_distribution: list[dict[str, Any]] = field(default_factory=list)

    # Trace detailed analysis
    span_type_stats: list[dict[str, Any]] = field(default_factory=list)
    error_spans: list[dict[str, Any]] = field(default_factory=list)
    service_dependencies: list[dict[str, Any]] = field(default_factory=list)

    # Log detailed analysis
    log_severity_distribution: list[dict[str, Any]] = field(default_factory=list)
    log_volume_trends: list[dict[str, Any]] = field(default_factory=list)

    # SLI/SLO analysis
    sli_trends: list[dict[str, Any]] = field(default_factory=list)
    slo_violations: list[dict[str, Any]] = field(default_factory=list)

    # Recommendations
    critical_issues: list[str] = field(default_factory=list)
    warnings: list[str] = field(default_factory=list)
    recommendations: list[str] = field(default_factory=list)


# =============================================================================
# Health Score Calculation
# =============================================================================


def calculate_health_score(error_rate: float, p95_ms: float, log_gap_minutes: float) -> int:
    """Calculate health score (0-100) for a service."""
    score = 100

    # Error rate penalty
    if error_rate > ERROR_RATE_CRITICAL:
        score -= 40
    elif error_rate > ERROR_RATE_HIGH:
        score -= 25
    elif error_rate > 1:
        score -= 10
    elif error_rate > ERROR_RATE_LOW:
        score -= 5

    # Latency penalty
    if p95_ms > LATENCY_CRITICAL_MS:
        score -= 30
    elif p95_ms > LATENCY_HIGH_MS:
        score -= 20
    elif p95_ms > LATENCY_WARN_MS:
        score -= 10
    elif p95_ms > LATENCY_LOW_MS:
        score -= 5

    # Log gap penalty (service might be down)
    if log_gap_minutes > LOG_GAP_CRITICAL_MIN:
        score -= 30
    elif log_gap_minutes > LOG_GAP_WARN_MIN:
        score -= 15

    return max(0, score)


def get_health_status(score: int) -> str:
    """Get health status label from score."""
    if score >= HEALTH_HEALTHY_MIN:
        return "Healthy"
    if score >= HEALTH_WARNING_MIN:
        return "Warning"
    if score >= HEALTH_DEGRADED_MIN:
        return "Degraded"
    return "Critical"


# =============================================================================
# Data Collection
# =============================================================================


def collect_service_stats(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect service-level statistics."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        service_name,
        count() as total_logs,
        countIf(level IN ('Error', 'Fatal')) as error_count,
        countIf(level = 'Warn') as warn_count,
        round(countIf(level IN ('Error', 'Fatal')) / count() * 100, 3) as error_rate,
        max(timestamp) as last_seen,
        dateDiff('minute', max(timestamp), now()) as minutes_since_last_log
    FROM {db}.logs
    WHERE timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY service_name
    ORDER BY error_rate DESC, total_logs DESC
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_service_stats", exc)
        return []


def collect_error_trends(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect hourly error trends by service."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        toStartOfHour(timestamp) as hour,
        service_name,
        countIf(level IN ('Error', 'Fatal')) as error_count,
        count() as total_count,
        round(countIf(level IN ('Error', 'Fatal')) / count() * 100, 2) as error_rate
    FROM {db}.logs
    WHERE timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY hour, service_name
    HAVING total_count > 0
    ORDER BY hour DESC, error_count DESC
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_error_trends", exc)
        return []


def collect_api_performance(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect API endpoint performance metrics."""
    db = _validate_identifier(database)
    query = f"""
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
    FROM {db}.otel_traces
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND SpanName != ''
    GROUP BY ServiceName, SpanName
    HAVING request_count >= 5
    ORDER BY p95_ms DESC
    LIMIT 30
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_api_performance", exc)
        return []


def collect_bottlenecks(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Identify performance bottlenecks (slow operations)."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        SpanName as operation,
        count() as occurrences,
        round(avg(DurationMs), 2) as avg_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_ms,
        round(sum(DurationMs) / 1000, 2) as total_time_sec
    FROM {db}.otel_traces
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND DurationMs > 1000
    GROUP BY ServiceName, SpanName
    HAVING occurrences >= 3
    ORDER BY total_time_sec DESC
    LIMIT 15
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_bottlenecks", exc)
        return []


def collect_error_types(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect error types and their frequency."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        if(ExceptionType = '', 'Unknown', ExceptionType) as error_type,
        count() as error_count,
        any(substring(Body, 1, 150)) as sample_message
    FROM {db}.otel_error_logs
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY ServiceName, ExceptionType
    ORDER BY error_count DESC
    LIMIT 20
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_error_types", exc)
        return []


def collect_recent_errors(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect most recent error logs."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        SeverityText as level,
        substring(Body, 1, 200) as message,
        if(ExceptionType = '', '-', ExceptionType) as error_type,
        formatDateTime(Timestamp, '%%Y-%%m-%%d %%H:%%M:%%S') as timestamp
    FROM {db}.otel_error_logs
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
    ORDER BY Timestamp DESC
    LIMIT 25
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_recent_errors", exc)
        return []


# =============================================================================
# Enhanced Metrics Collection (Alt Service Details)
# =============================================================================


def collect_http_endpoint_stats(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect detailed HTTP endpoint performance statistics."""
    db = _validate_identifier(database)
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
    FROM {db}.otel_http_requests
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND HttpRoute != ''
    GROUP BY ServiceName, HttpRoute
    ORDER BY request_count DESC
    LIMIT 30
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_http_endpoint_stats", exc)
        return []


def collect_http_status_distribution(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect HTTP status code distribution by service."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        count() as total_requests,
        countIf(HttpStatusCode >= 200 AND HttpStatusCode < 300) as status_2xx,
        countIf(HttpStatusCode >= 300 AND HttpStatusCode < 400) as status_3xx,
        countIf(HttpStatusCode >= 400 AND HttpStatusCode < 500) as status_4xx,
        countIf(HttpStatusCode >= 500) as status_5xx,
        round(countIf(HttpStatusCode >= 500) / count() * 100, 2) as error_5xx_rate
    FROM {db}.otel_http_requests
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY ServiceName
    ORDER BY total_requests DESC
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_http_status_distribution", exc)
        return []


def collect_span_type_stats(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect trace span type statistics."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        SpanKind as span_kind,
        count() as span_count,
        round(avg(DurationMs), 2) as avg_duration_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_duration_ms,
        countIf(StatusCode = 'ERROR') as error_count
    FROM {db}.otel_traces
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY ServiceName, SpanKind
    ORDER BY span_count DESC
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_span_type_stats", exc)
        return []


def collect_error_spans(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect detailed error span information."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        SpanName as operation,
        StatusMessage as error_message,
        count() as error_count,
        round(avg(DurationMs), 2) as avg_duration_ms,
        formatDateTime(max(Timestamp), '%%Y-%%m-%%d %%H:%%M:%%S') as last_occurrence
    FROM {db}.otel_traces
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND StatusCode = 'ERROR'
    GROUP BY ServiceName, SpanName, StatusMessage
    ORDER BY error_count DESC
    LIMIT 20
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_error_spans", exc)
        return []


def collect_service_dependencies(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect service-to-service call dependencies."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        s1.ServiceName as caller,
        s2.ServiceName as callee,
        count() as call_count,
        round(avg(s1.DurationMs), 2) as avg_duration_ms,
        round(quantile(0.95)(s1.DurationMs), 2) as p95_duration_ms,
        countIf(s1.StatusCode = 'ERROR') as error_count
    FROM {db}.otel_traces s1
    JOIN {db}.otel_traces s2
        ON s1.TraceId = s2.TraceId AND s1.SpanId = s2.ParentSpanId
    WHERE s1.Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND s1.ServiceName != s2.ServiceName
    GROUP BY s1.ServiceName, s2.ServiceName
    ORDER BY call_count DESC
    LIMIT 20
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_service_dependencies", exc)
        return []


def collect_log_severity_distribution(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect log severity distribution by service."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        count() as total_logs,
        countIf(SeverityText = 'DEBUG' OR SeverityNumber <= 4) as debug_count,
        countIf(SeverityText = 'INFO' OR (SeverityNumber > 4 AND SeverityNumber <= 8)) as info_count,
        countIf(SeverityText = 'WARN' OR SeverityText = 'WARNING' OR (SeverityNumber > 8 AND SeverityNumber <= 12)) as warn_count,
        countIf(SeverityText = 'ERROR' OR (SeverityNumber > 12 AND SeverityNumber <= 16)) as error_count,
        countIf(SeverityText = 'FATAL' OR SeverityText = 'CRITICAL' OR SeverityNumber > 20) as fatal_count,
        round(countIf(SeverityNumber >= 17) / count() * 100, 2) as error_rate
    FROM {db}.otel_logs
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY ServiceName
    ORDER BY total_logs DESC
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_log_severity_distribution", exc)
        return []


def collect_log_volume_trends(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect hourly log volume trends."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        toStartOfHour(Timestamp) as hour,
        ServiceName as service,
        count() as log_count,
        countIf(SeverityNumber >= 17) as error_count,
        round(countIf(SeverityNumber >= 17) / count() * 100, 2) as error_rate
    FROM {db}.otel_logs
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
    GROUP BY hour, ServiceName
    ORDER BY hour DESC, log_count DESC
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_log_volume_trends", exc)
        return []


def collect_sli_trends(client: Client, database: str, hours: int) -> list[dict[str, Any]]:
    """Collect SLI metric trends (error_rate, log_throughput)."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        toStartOfFiveMinutes(Timestamp) as time_bucket,
        ServiceName as service,
        Metric as metric,
        round(avg(Value), 4) as value
    FROM {db}.sli_metrics
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND Metric IN ('error_rate', 'log_throughput')
    GROUP BY time_bucket, ServiceName, Metric
    ORDER BY time_bucket DESC, ServiceName, Metric
    LIMIT 500
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(query, parameters={"hours": hours})
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_sli_trends", exc)
        return []


def collect_slo_violations(
    client: Client, database: str, hours: int, error_rate_threshold: float = 1.0
) -> list[dict[str, Any]]:
    """Detect SLO violations where error rate exceeds threshold."""
    db = _validate_identifier(database)
    query = f"""
    SELECT
        ServiceName as service,
        toStartOfFiveMinutes(Timestamp) as time_bucket,
        round(avg(Value) * 100, 2) as error_rate_pct,
        count() as sample_count
    FROM {db}.sli_metrics
    WHERE Timestamp >= now() - INTERVAL %(hours)s HOUR
      AND Metric = 'error_rate'
    GROUP BY ServiceName, time_bucket
    HAVING avg(Value) > %(error_rate_threshold)s
    ORDER BY time_bucket DESC, error_rate_pct DESC
    LIMIT 50
    """  # noqa: S608 — identifier (trusted allowlist validated against _IDENTIFIER_RE above)
    try:
        result = client.query(
            query, parameters={"hours": hours, "error_rate_threshold": error_rate_threshold / 100}
        )
        return [dict(zip(result.column_names, row, strict=True)) for row in result.result_rows]
    except ClickHouseError as exc:
        logger.warning("%s query failed: %s", "collect_slo_violations", exc)
        return []


# =============================================================================
# Analysis & Recommendations
# =============================================================================


def _populate_service_health(result: AnalysisResult) -> None:
    """Populate per-service health scores and compute overall health score."""
    service_latencies = {s["service"]: s.get("p95_ms", 0) for s in result.api_performance}

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
        )
        result.service_health.append(service)

    if result.service_health:
        result.overall_health_score = sum(s.health_score for s in result.service_health) // len(
            result.service_health
        )


def _collect_health_alerts(result: AnalysisResult) -> None:
    """Record critical health issues and core service warnings."""
    for svc in result.service_health:
        if svc.health_score < HEALTH_DEGRADED_MIN:
            result.critical_issues.append(
                f"**{svc.name}** is in critical state (score: {svc.health_score}). "
                f"Error rate: {svc.error_rate}%, p95: {svc.p95_latency_ms}ms"
            )

    high_error_services = [s for s in result.service_health if s.error_rate > 1]
    if high_error_services:
        names = ", ".join(s.name for s in high_error_services[:3])
        result.warnings.append(f"Services with elevated error rates (>1%): {names}")

    if result.bottlenecks:
        top_bottleneck = result.bottlenecks[0]
        result.warnings.append(
            f"Performance bottleneck detected: {top_bottleneck['service']}/{top_bottleneck['operation']} "
            f"(p95: {top_bottleneck['p95_ms']}ms, total time: {top_bottleneck['total_time_sec']}s)"
        )


def _collect_recommendations(result: AnalysisResult) -> None:
    """Generate actionable recommendations from API latency, errors, and stale logs."""
    slow_apis = [a for a in result.api_performance if a.get("p95_ms", 0) > LATENCY_WARN_MS]
    if slow_apis:
        result.recommendations.append(
            f"Optimize slow endpoints: {len(slow_apis)} APIs have p95 > 1s. "
            "Consider caching, query optimization, or async processing."
        )

    if result.error_types:
        top_error = result.error_types[0]
        result.recommendations.append(
            f"Investigate top error: {top_error['error_type']} in {top_error['service']} "
            f"({top_error['error_count']} occurrences)"
        )

    stale_services = [
        s for s in result.service_stats if s.get("minutes_since_last_log", 0) > LOG_GAP_WARN_MIN
    ]
    if stale_services:
        names = ", ".join(s["service_name"] for s in stale_services[:3])
        result.recommendations.append(f"Check services with no recent logs: {names}")


def _collect_enhanced_alerts(result: AnalysisResult) -> None:
    """Record alerts for HTTP 5xx rates, SLO violations, error spans, and dependencies."""
    high_5xx_services = [
        s for s in result.http_status_distribution if s.get("error_5xx_rate", 0) > 1
    ]
    if high_5xx_services:
        for svc in high_5xx_services[:3]:
            result.warnings.append(
                f"HTTP 5xx error rate elevated: {svc['service']} "
                f"({svc['error_5xx_rate']}% of {svc['total_requests']} requests)"
            )

    if result.slo_violations:
        violation_count = len(result.slo_violations)
        affected_services = {v["service"] for v in result.slo_violations}
        result.critical_issues.append(
            f"SLO violations detected: {violation_count} periods with error rate >1% "
            f"across {len(affected_services)} services"
        )

    if result.error_spans:
        top_error_span = result.error_spans[0]
        result.warnings.append(
            f"Trace errors detected: {top_error_span['operation']} in {top_error_span['service']} "
            f"({top_error_span['error_count']} errors)"
        )

    high_error_deps = [
        d
        for d in result.service_dependencies
        if d.get("call_count", 0) > MIN_DEP_CALL_COUNT
        and d.get("error_count", 0) > 0
        and (d["error_count"] / d["call_count"]) > DEP_ERROR_RATE_THRESHOLD
    ]
    if high_error_deps:
        for dep in high_error_deps[:2]:
            error_pct = round(dep["error_count"] / dep["call_count"] * 100, 1)
            result.warnings.append(
                f"High error rate in service call: {dep['caller']} -> {dep['callee']} "
                f"({error_pct}% errors, {dep['call_count']} calls)"
            )


def _collect_log_volume_anomalies(result: AnalysisResult) -> None:
    """Detect sudden hourly log volume spikes per service."""
    if not result.log_volume_trends:
        return

    service_volumes: dict[str, list[int]] = {}
    for trend in result.log_volume_trends:
        svc = trend.get("service", "")
        if svc:
            service_volumes.setdefault(svc, []).append(trend.get("log_count", 0))

    for svc, volumes in service_volumes.items():
        if len(volumes) >= MIN_VOLUME_SAMPLES:
            recent = volumes[0]
            previous = volumes[1]
            if previous > 0 and recent > previous * LOG_VOLUME_SPIKE_FACTOR:
                result.warnings.append(
                    f"Log volume spike detected: {svc} "
                    f"({recent} logs vs {previous} previous hour, {round(recent / previous, 1)}x increase)"
                )


def analyze_health(result: AnalysisResult) -> None:
    """Analyze data and generate health scores and recommendations."""
    _populate_service_health(result)
    _collect_health_alerts(result)
    _collect_recommendations(result)
    _collect_enhanced_alerts(result)
    _collect_log_volume_anomalies(result)


# =============================================================================
# Report Generation
# =============================================================================


def format_table(data: list[dict[str, Any]], columns: list[str] | None = None) -> str:
    """Format data as Markdown table."""
    if not data:
        return "_No data available_\n"

    cols = columns or list(data[0].keys())
    header = "| " + " | ".join(cols) + " |"
    separator = "|" + "|".join("---" for _ in cols) + "|"
    rows = []
    for row in data:
        values = [str(row.get(c, ""))[:60] for c in cols]
        rows.append("| " + " | ".join(values) + " |")

    return "\n".join([header, separator, *rows]) + "\n"


def _append_report_summary(report: list[str], result: AnalysisResult) -> None:
    """Append header, overall health score, and executive summary to report."""
    report.append("# Alt System Health Report")
    report.append("")
    report.append(f"**Generated**: {result.generated_at.strftime('%Y-%m-%d %H:%M:%S')}")
    report.append(f"**Analysis Period**: Last {result.hours_analyzed} hours")
    report.append("")

    status = get_health_status(result.overall_health_score)
    status_emoji = {"Healthy": "+", "Warning": "!", "Degraded": "!!", "Critical": "!!!"}
    report.append(
        f"## Overall System Health: {result.overall_health_score}/100 [{status}] {status_emoji.get(status, '')}"
    )
    report.append("")

    total_logs = sum(s.total_logs for s in result.service_health)
    total_errors = sum(s.error_count for s in result.service_health)
    healthy_count = len([s for s in result.service_health if s.health_score >= HEALTH_HEALTHY_MIN])
    degraded_count = len([s for s in result.service_health if s.health_score < HEALTH_WARNING_MIN])

    report.append("### Summary")
    report.append(f"- **Total Log Entries**: {total_logs:,}")
    report.append(f"- **Total Errors**: {total_errors:,}")
    report.append(f"- **Services Monitored**: {len(result.service_health)}")
    report.append(f"- **Healthy Services**: {healthy_count}")
    report.append(f"- **Degraded/Critical Services**: {degraded_count}")
    report.append("")


def _append_alerts_and_dashboard(report: list[str], result: AnalysisResult) -> None:
    """Append critical issues, warnings, recommendations, and service health table."""
    if result.critical_issues:
        report.append("## Critical Issues")
        report.append("")
        report.extend(f"- {issue}" for issue in result.critical_issues)
        report.append("")

    if result.warnings:
        report.append("## Warnings")
        report.append("")
        report.extend(f"- {warning}" for warning in result.warnings)
        report.append("")

    if result.recommendations:
        report.append("## Recommendations")
        report.append("")
        report.extend(f"- {rec}" for rec in result.recommendations)
        report.append("")

    report.append("## Service Health Dashboard")
    report.append("")
    health_data = [
        {
            "service": s.name,
            "score": s.health_score,
            "status": get_health_status(s.health_score),
            "error_rate": f"{s.error_rate}%",
            "p95_ms": s.p95_latency_ms,
            "logs": s.total_logs,
        }
        for s in sorted(result.service_health, key=lambda x: x.health_score)
    ]
    report.append(
        format_table(health_data, ["service", "score", "status", "error_rate", "p95_ms", "logs"])
    )


def _append_table_section(
    report: list[str],
    heading: str,
    rows: list[dict[str, Any]],
    columns: list[str],
    empty_message: str,
) -> None:
    """Append a Markdown section containing a formatted table or fallback line."""
    report.append(heading)
    report.append("")
    if rows:
        report.append(format_table(rows, columns))
    else:
        report.append(empty_message)


def _append_core_sections(report: list[str], result: AnalysisResult) -> None:
    """Append core performance, bottleneck, and error report sections."""
    _append_table_section(
        report,
        "## API Performance (Top by p95 Latency)",
        result.api_performance[:15],
        ["service", "endpoint", "request_count", "avg_ms", "p95_ms", "p99_ms"],
        "_No trace data available_\n",
    )
    _append_table_section(
        report,
        "## Performance Bottlenecks (>1s operations)",
        result.bottlenecks,
        ["service", "operation", "occurrences", "avg_ms", "p95_ms", "total_time_sec"],
        "_No significant bottlenecks detected_\n",
    )
    _append_table_section(
        report,
        "## Error Analysis by Type",
        result.error_types,
        ["service", "error_type", "error_count", "sample_message"],
        "_No error data available_\n",
    )
    _append_table_section(
        report,
        "## Recent Errors (Last 25)",
        result.recent_errors,
        ["timestamp", "service", "level", "error_type", "message"],
        "_No recent errors_\n",
    )
    recent_trends = [t for t in result.error_trends if t.get("error_count", 0) > 0][:30]
    _append_table_section(
        report,
        "## Error Trends (Last 6 Hours)",
        recent_trends,
        ["hour", "service_name", "error_count", "total_count", "error_rate"],
        "_No error trends available_\n",
    )


def _append_enhanced_sections(report: list[str], result: AnalysisResult) -> None:
    """Append HTTP, trace span, log, and SLO report sections."""
    _append_table_section(
        report,
        "## HTTP Endpoint Performance",
        result.http_endpoint_stats[:20],
        [
            "service",
            "route",
            "request_count",
            "avg_duration_ms",
            "p95_duration_ms",
            "error_rate",
        ],
        "_No HTTP endpoint data available_\n",
    )
    _append_table_section(
        report,
        "## HTTP Status Distribution by Service",
        result.http_status_distribution,
        [
            "service",
            "total_requests",
            "status_2xx",
            "status_4xx",
            "status_5xx",
            "error_5xx_rate",
        ],
        "_No HTTP status data available_\n",
    )
    _append_table_section(
        report,
        "## Trace Span Analysis",
        result.span_type_stats[:20],
        [
            "service",
            "span_kind",
            "span_count",
            "avg_duration_ms",
            "p95_duration_ms",
            "error_count",
        ],
        "_No trace span data available_\n",
    )
    _append_table_section(
        report,
        "## Error Spans (Trace Errors)",
        result.error_spans[:15],
        ["service", "operation", "error_count", "avg_duration_ms", "last_occurrence"],
        "_No error spans detected_\n",
    )
    _append_table_section(
        report,
        "## Service Dependencies",
        result.service_dependencies,
        [
            "caller",
            "callee",
            "call_count",
            "avg_duration_ms",
            "p95_duration_ms",
            "error_count",
        ],
        "_No service dependencies detected_\n",
    )
    _append_table_section(
        report,
        "## Log Severity Distribution",
        result.log_severity_distribution,
        [
            "service",
            "total_logs",
            "debug_count",
            "info_count",
            "warn_count",
            "error_count",
            "error_rate",
        ],
        "_No log severity data available_\n",
    )
    _append_table_section(
        report,
        "## Log Volume Trends (Hourly)",
        result.log_volume_trends[:24],
        ["hour", "service", "log_count", "error_count", "error_rate"],
        "_No log volume trend data available_\n",
    )
    _append_table_section(
        report,
        "## SLO Violations (Error Rate > 1%)",
        result.slo_violations[:20],
        ["service", "time_bucket", "error_rate_pct", "sample_count"],
        "_No SLO violations detected_\n",
    )


def _append_sli_and_footer(report: list[str], result: AnalysisResult) -> None:
    """Append SLI trends summary and report footer."""
    report.append("## SLI Trends Summary")
    report.append("")
    if result.sli_trends:
        latest_sli: dict[str, dict[str, Any]] = {}
        for trend in result.sli_trends:
            key = f"{trend.get('service', '')}-{trend.get('metric', '')}"
            if key not in latest_sli:
                latest_sli[key] = trend
        sli_summary = list(latest_sli.values())[:20]
        report.append(format_table(sli_summary, ["service", "metric", "value", "time_bucket"]))
    else:
        report.append("_No SLI trend data available_\n")

    report.append("---")
    report.append("")
    report.append("*Report generated by Alt System Health Analyzer (Enhanced)*")


def generate_report(result: AnalysisResult) -> str:
    """Generate Markdown report from analysis results."""
    report: list[str] = []
    _append_report_summary(report, result)
    _append_alerts_and_dashboard(report, result)
    _append_core_sections(report, result)
    _append_enhanced_sections(report, result)
    _append_sli_and_footer(report, result)
    return "\n".join(report)


# =============================================================================
# Main
# =============================================================================


_COLLECTION_STEPS: list[tuple[str, str, Callable[[Client, str, int], list[dict[str, Any]]]]] = [
    ("service_stats", "Collecting service statistics...", collect_service_stats),
    ("error_trends", "Collecting error trends...", collect_error_trends),
    ("api_performance", "Collecting API performance...", collect_api_performance),
    ("bottlenecks", "Identifying bottlenecks...", collect_bottlenecks),
    ("error_types", "Collecting error types...", collect_error_types),
    ("recent_errors", "Collecting recent errors...", collect_recent_errors),
    ("http_endpoint_stats", "Collecting HTTP endpoint statistics...", collect_http_endpoint_stats),
    (
        "http_status_distribution",
        "Collecting HTTP status distribution...",
        collect_http_status_distribution,
    ),
    ("span_type_stats", "Collecting trace span statistics...", collect_span_type_stats),
    ("error_spans", "Collecting error spans...", collect_error_spans),
    ("service_dependencies", "Collecting service dependencies...", collect_service_dependencies),
    (
        "log_severity_distribution",
        "Collecting log severity distribution...",
        collect_log_severity_distribution,
    ),
    ("log_volume_trends", "Collecting log volume trends...", collect_log_volume_trends),
    ("sli_trends", "Collecting SLI trends...", collect_sli_trends),
    ("slo_violations", "Detecting SLO violations...", collect_slo_violations),
]


def run_analysis(config: ClickHouseConfig, hours: int, verbose: bool = False) -> AnalysisResult:
    """Run complete analysis and return results."""
    if verbose:
        sys.stdout.write(f"Connecting to ClickHouse at {config.host}:{config.port}...\n")

    client = clickhouse_connect.get_client(
        host=config.host,
        port=config.port,
        username=config.user,
        password=config.password,
        connect_timeout=10,
        send_receive_timeout=60,
    )

    with contextlib.closing(client):
        result = AnalysisResult(hours_analyzed=hours)

        for attr, message, collector in _COLLECTION_STEPS:
            if verbose:
                sys.stdout.write(f"{message}\n")
            setattr(result, attr, collector(client, config.database, hours))

        if verbose:
            sys.stdout.write("Analyzing health and generating recommendations...\n")
        analyze_health(result)

        return result


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Analyze Alt system health using ClickHouse logs and traces"
    )
    parser.add_argument(
        "--hours",
        type=int,
        default=24,
        help="Analysis period in hours (default: 24)",
    )
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(__file__).parent / "reports",
        help="Output directory for reports (default: ./reports)",
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Enable verbose output",
    )

    args = parser.parse_args()
    config = ClickHouseConfig.from_env()

    try:
        result = run_analysis(config, args.hours, args.verbose)
        report = generate_report(result)

        args.output_dir.mkdir(parents=True, exist_ok=True)
        timestamp = datetime.now().astimezone().strftime("%Y%m%d_%H%M%S")
        output_file = args.output_dir / f"system_health_{timestamp}.md"
        output_file.write_text(report)

        sys.stdout.write(f"Report generated: {output_file}\n")

        if args.verbose:
            sys.stdout.write("\n" + "=" * 80 + "\n")
            sys.stdout.write(f"{report}\n")

    except Exception as e:  # noqa: BLE001 — top-level CLI boundary logs error and exits non-zero
        sys.stderr.write(f"Error: {e}\n")
        if args.verbose:
            traceback.print_exc()
        return 1
    else:
        return 0


if __name__ == "__main__":
    sys.exit(main())
