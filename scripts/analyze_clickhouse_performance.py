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
import os
import sys
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

import clickhouse_connect
from clickhouse_connect.driver.client import Client


# =============================================================================
# Configuration
# =============================================================================


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

        return cls(
            host=os.getenv("APP_CLICKHOUSE_HOST", "localhost"),
            port=int(os.getenv("APP_CLICKHOUSE_PORT", "8123")),
            user=os.getenv("APP_CLICKHOUSE_USER", "default"),
            password=get_env_or_file("APP_CLICKHOUSE_PASSWORD", ""),
            database=os.getenv("APP_CLICKHOUSE_DATABASE", "rask_logs"),
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
class HttpEndpointStats:
    """HTTP endpoint performance statistics."""

    service: str
    route: str
    request_count: int
    avg_duration_ms: float
    p95_duration_ms: float
    avg_response_size: int
    error_rate: float
    status_2xx: int
    status_4xx: int
    status_5xx: int


@dataclass
class SLITrend:
    """SLI metric trend data point."""

    timestamp: datetime
    service: str
    metric: str
    value: float


@dataclass
class LogVolumeStats:
    """Log volume statistics by severity."""

    service: str
    total_logs: int
    debug_count: int
    info_count: int
    warn_count: int
    error_count: int
    fatal_count: int


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


def calculate_health_score(
    error_rate: float, p95_ms: float, log_gap_minutes: float
) -> int:
    """Calculate health score (0-100) for a service."""
    score = 100

    # Error rate penalty
    if error_rate > 10:
        score -= 40
    elif error_rate > 5:
        score -= 25
    elif error_rate > 1:
        score -= 10
    elif error_rate > 0.5:
        score -= 5

    # Latency penalty
    if p95_ms > 10000:
        score -= 30
    elif p95_ms > 5000:
        score -= 20
    elif p95_ms > 1000:
        score -= 10
    elif p95_ms > 500:
        score -= 5

    # Log gap penalty (service might be down)
    if log_gap_minutes > 10:
        score -= 30
    elif log_gap_minutes > 5:
        score -= 15

    return max(0, score)


def get_health_status(score: int) -> str:
    """Get health status label from score."""
    if score >= 90:
        return "Healthy"
    elif score >= 70:
        return "Warning"
    elif score >= 50:
        return "Degraded"
    else:
        return "Critical"


# =============================================================================
# Data Collection
# =============================================================================


def collect_service_stats(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect service-level statistics."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_error_trends(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect hourly error trends by service."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_api_performance(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect API endpoint performance metrics."""
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
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND SpanName != ''
    GROUP BY ServiceName, SpanName
    HAVING request_count >= 5
    ORDER BY p95_ms DESC
    LIMIT 30
    """
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_bottlenecks(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Identify performance bottlenecks (slow operations)."""
    query = f"""
    SELECT
        ServiceName as service,
        SpanName as operation,
        count() as occurrences,
        round(avg(DurationMs), 2) as avg_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_ms,
        round(sum(DurationMs) / 1000, 2) as total_time_sec
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND DurationMs > 1000
    GROUP BY ServiceName, SpanName
    HAVING occurrences >= 3
    ORDER BY total_time_sec DESC
    LIMIT 15
    """
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_error_types(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect error types and their frequency."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_recent_errors(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect most recent error logs."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_service_latency(
    client: Client, database: str, hours: int
) -> dict[str, float]:
    """Collect p95 latency per service."""
    query = f"""
    SELECT
        ServiceName,
        round(quantile(0.95)(DurationMs), 2) as p95_ms
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY ServiceName
    """
    try:
        result = client.query(query)
        return {row[0]: row[1] for row in result.result_rows}
    except Exception:
        return {}


# =============================================================================
# Enhanced Metrics Collection (Alt Service Details)
# =============================================================================


def collect_http_endpoint_stats(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect detailed HTTP endpoint performance statistics."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_http_status_distribution(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect HTTP status code distribution by service."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_span_type_stats(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect trace span type statistics."""
    query = f"""
    SELECT
        ServiceName as service,
        SpanKind as span_kind,
        count() as span_count,
        round(avg(DurationMs), 2) as avg_duration_ms,
        round(quantile(0.95)(DurationMs), 2) as p95_duration_ms,
        countIf(StatusCode = 'ERROR') as error_count
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY ServiceName, SpanKind
    ORDER BY span_count DESC
    """
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_error_spans(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect detailed error span information."""
    query = f"""
    SELECT
        ServiceName as service,
        SpanName as operation,
        StatusMessage as error_message,
        count() as error_count,
        round(avg(DurationMs), 2) as avg_duration_ms,
        formatDateTime(max(Timestamp), '%Y-%m-%d %H:%M:%S') as last_occurrence
    FROM {database}.otel_traces
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
      AND StatusCode = 'ERROR'
    GROUP BY ServiceName, SpanName, StatusMessage
    ORDER BY error_count DESC
    LIMIT 20
    """
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_service_dependencies(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect service-to-service call dependencies."""
    query = f"""
    SELECT
        s1.ServiceName as caller,
        s2.ServiceName as callee,
        count() as call_count,
        round(avg(s1.DurationMs), 2) as avg_duration_ms,
        round(quantile(0.95)(s1.DurationMs), 2) as p95_duration_ms,
        countIf(s1.StatusCode = 'ERROR') as error_count
    FROM {database}.otel_traces s1
    JOIN {database}.otel_traces s2
        ON s1.TraceId = s2.TraceId AND s1.SpanId = s2.ParentSpanId
    WHERE s1.Timestamp >= now() - INTERVAL {hours} HOUR
      AND s1.ServiceName != s2.ServiceName
    GROUP BY s1.ServiceName, s2.ServiceName
    ORDER BY call_count DESC
    LIMIT 20
    """
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_log_severity_distribution(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect log severity distribution by service."""
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
    FROM {database}.otel_logs
    WHERE Timestamp >= now() - INTERVAL {hours} HOUR
    GROUP BY ServiceName
    ORDER BY total_logs DESC
    """
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_log_volume_trends(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect hourly log volume trends."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_sli_trends(
    client: Client, database: str, hours: int
) -> list[dict[str, Any]]:
    """Collect SLI metric trends (error_rate, log_throughput)."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


def collect_slo_violations(
    client: Client, database: str, hours: int, error_rate_threshold: float = 1.0
) -> list[dict[str, Any]]:
    """Detect SLO violations where error rate exceeds threshold."""
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
    try:
        result = client.query(query)
        return [dict(zip(result.column_names, row)) for row in result.result_rows]
    except Exception:
        return []


# =============================================================================
# Analysis & Recommendations
# =============================================================================


def analyze_health(result: AnalysisResult) -> None:
    """Analyze data and generate health scores and recommendations."""
    service_latencies = {
        s["service"]: s.get("p95_ms", 0) for s in result.api_performance
    }

    # Calculate per-service health
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

    # Calculate overall health score
    if result.service_health:
        result.overall_health_score = sum(
            s.health_score for s in result.service_health
        ) // len(result.service_health)

    # Generate critical issues
    for svc in result.service_health:
        if svc.health_score < 50:
            result.critical_issues.append(
                f"**{svc.name}** is in critical state (score: {svc.health_score}). "
                f"Error rate: {svc.error_rate}%, p95: {svc.p95_latency_ms}ms"
            )

    # Generate warnings
    high_error_services = [s for s in result.service_health if s.error_rate > 1]
    if high_error_services:
        names = ", ".join(s.name for s in high_error_services[:3])
        result.warnings.append(
            f"Services with elevated error rates (>1%): {names}"
        )

    # Check for bottlenecks
    if result.bottlenecks:
        top_bottleneck = result.bottlenecks[0]
        result.warnings.append(
            f"Performance bottleneck detected: {top_bottleneck['service']}/{top_bottleneck['operation']} "
            f"(p95: {top_bottleneck['p95_ms']}ms, total time: {top_bottleneck['total_time_sec']}s)"
        )

    # Generate recommendations
    slow_apis = [a for a in result.api_performance if a.get("p95_ms", 0) > 1000]
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
        s for s in result.service_stats
        if s.get("minutes_since_last_log", 0) > 5
    ]
    if stale_services:
        names = ", ".join(s["service_name"] for s in stale_services[:3])
        result.recommendations.append(
            f"Check services with no recent logs: {names}"
        )

    # Enhanced analysis: HTTP 5xx error rate
    high_5xx_services = [
        s for s in result.http_status_distribution
        if s.get("error_5xx_rate", 0) > 1
    ]
    if high_5xx_services:
        for svc in high_5xx_services[:3]:
            result.warnings.append(
                f"HTTP 5xx error rate elevated: {svc['service']} "
                f"({svc['error_5xx_rate']}% of {svc['total_requests']} requests)"
            )

    # Enhanced analysis: SLO violations
    if result.slo_violations:
        violation_count = len(result.slo_violations)
        affected_services = set(v["service"] for v in result.slo_violations)
        result.critical_issues.append(
            f"SLO violations detected: {violation_count} periods with error rate >1% "
            f"across {len(affected_services)} services"
        )

    # Enhanced analysis: Error spans
    if result.error_spans:
        top_error_span = result.error_spans[0]
        result.warnings.append(
            f"Trace errors detected: {top_error_span['operation']} in {top_error_span['service']} "
            f"({top_error_span['error_count']} errors)"
        )

    # Enhanced analysis: Service dependencies with high error rates
    high_error_deps = [
        d for d in result.service_dependencies
        if d.get("call_count", 0) > 10 and d.get("error_count", 0) > 0
        and (d["error_count"] / d["call_count"]) > 0.05
    ]
    if high_error_deps:
        for dep in high_error_deps[:2]:
            error_pct = round(dep["error_count"] / dep["call_count"] * 100, 1)
            result.warnings.append(
                f"High error rate in service call: {dep['caller']} -> {dep['callee']} "
                f"({error_pct}% errors, {dep['call_count']} calls)"
            )

    # Enhanced analysis: Log volume anomalies
    if result.log_volume_trends:
        # Check for sudden log volume spikes
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
                        f"Log volume spike detected: {svc} "
                        f"({recent} logs vs {previous} previous hour, {round(recent/previous, 1)}x increase)"
                    )


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

    return "\n".join([header, separator] + rows) + "\n"


def generate_report(result: AnalysisResult) -> str:
    """Generate Markdown report from analysis results."""
    report = []

    # Header
    report.append("# Alt System Health Report")
    report.append("")
    report.append(f"**Generated**: {result.generated_at.strftime('%Y-%m-%d %H:%M:%S')}")
    report.append(f"**Analysis Period**: Last {result.hours_analyzed} hours")
    report.append("")

    # Overall Health Score
    status = get_health_status(result.overall_health_score)
    status_emoji = {"Healthy": "+", "Warning": "!", "Degraded": "!!", "Critical": "!!!"}
    report.append(f"## Overall System Health: {result.overall_health_score}/100 [{status}] {status_emoji.get(status, '')}")
    report.append("")

    # Executive Summary
    total_logs = sum(s.total_logs for s in result.service_health)
    total_errors = sum(s.error_count for s in result.service_health)
    healthy_count = len([s for s in result.service_health if s.health_score >= 90])
    degraded_count = len([s for s in result.service_health if s.health_score < 70])

    report.append("### Summary")
    report.append(f"- **Total Log Entries**: {total_logs:,}")
    report.append(f"- **Total Errors**: {total_errors:,}")
    report.append(f"- **Services Monitored**: {len(result.service_health)}")
    report.append(f"- **Healthy Services**: {healthy_count}")
    report.append(f"- **Degraded/Critical Services**: {degraded_count}")
    report.append("")

    # Critical Issues
    if result.critical_issues:
        report.append("## Critical Issues")
        report.append("")
        for issue in result.critical_issues:
            report.append(f"- {issue}")
        report.append("")

    # Warnings
    if result.warnings:
        report.append("## Warnings")
        report.append("")
        for warning in result.warnings:
            report.append(f"- {warning}")
        report.append("")

    # Recommendations
    if result.recommendations:
        report.append("## Recommendations")
        report.append("")
        for rec in result.recommendations:
            report.append(f"- {rec}")
        report.append("")

    # Service Health Dashboard
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
    report.append(format_table(health_data, ["service", "score", "status", "error_rate", "p95_ms", "logs"]))

    # API Performance
    report.append("## API Performance (Top by p95 Latency)")
    report.append("")
    if result.api_performance:
        report.append(format_table(
            result.api_performance[:15],
            ["service", "endpoint", "request_count", "avg_ms", "p95_ms", "p99_ms"]
        ))
    else:
        report.append("_No trace data available_\n")

    # Bottlenecks
    report.append("## Performance Bottlenecks (>1s operations)")
    report.append("")
    if result.bottlenecks:
        report.append(format_table(
            result.bottlenecks,
            ["service", "operation", "occurrences", "avg_ms", "p95_ms", "total_time_sec"]
        ))
    else:
        report.append("_No significant bottlenecks detected_\n")

    # Error Analysis
    report.append("## Error Analysis by Type")
    report.append("")
    if result.error_types:
        report.append(format_table(
            result.error_types,
            ["service", "error_type", "error_count", "sample_message"]
        ))
    else:
        report.append("_No error data available_\n")

    # Recent Errors
    report.append("## Recent Errors (Last 25)")
    report.append("")
    if result.recent_errors:
        report.append(format_table(
            result.recent_errors,
            ["timestamp", "service", "level", "error_type", "message"]
        ))
    else:
        report.append("_No recent errors_\n")

    # Hourly Error Trends (compact)
    report.append("## Error Trends (Last 6 Hours)")
    report.append("")
    recent_trends = [t for t in result.error_trends if t.get("error_count", 0) > 0][:30]
    if recent_trends:
        report.append(format_table(
            recent_trends,
            ["hour", "service_name", "error_count", "total_count", "error_rate"]
        ))
    else:
        report.append("_No error trends available_\n")

    # ==========================================================================
    # Enhanced Sections
    # ==========================================================================

    # HTTP Endpoint Performance
    report.append("## HTTP Endpoint Performance")
    report.append("")
    if result.http_endpoint_stats:
        report.append(format_table(
            result.http_endpoint_stats[:20],
            ["service", "route", "request_count", "avg_duration_ms", "p95_duration_ms", "error_rate"]
        ))
    else:
        report.append("_No HTTP endpoint data available_\n")

    # HTTP Status Distribution
    report.append("## HTTP Status Distribution by Service")
    report.append("")
    if result.http_status_distribution:
        report.append(format_table(
            result.http_status_distribution,
            ["service", "total_requests", "status_2xx", "status_4xx", "status_5xx", "error_5xx_rate"]
        ))
    else:
        report.append("_No HTTP status data available_\n")

    # Trace Span Analysis
    report.append("## Trace Span Analysis")
    report.append("")
    if result.span_type_stats:
        report.append(format_table(
            result.span_type_stats[:20],
            ["service", "span_kind", "span_count", "avg_duration_ms", "p95_duration_ms", "error_count"]
        ))
    else:
        report.append("_No trace span data available_\n")

    # Error Spans
    report.append("## Error Spans (Trace Errors)")
    report.append("")
    if result.error_spans:
        report.append(format_table(
            result.error_spans[:15],
            ["service", "operation", "error_count", "avg_duration_ms", "last_occurrence"]
        ))
    else:
        report.append("_No error spans detected_\n")

    # Service Dependencies
    report.append("## Service Dependencies")
    report.append("")
    if result.service_dependencies:
        report.append(format_table(
            result.service_dependencies,
            ["caller", "callee", "call_count", "avg_duration_ms", "p95_duration_ms", "error_count"]
        ))
    else:
        report.append("_No service dependencies detected_\n")

    # Log Severity Distribution
    report.append("## Log Severity Distribution")
    report.append("")
    if result.log_severity_distribution:
        report.append(format_table(
            result.log_severity_distribution,
            ["service", "total_logs", "debug_count", "info_count", "warn_count", "error_count", "error_rate"]
        ))
    else:
        report.append("_No log severity data available_\n")

    # Log Volume Trends (last 12 hours, top services)
    report.append("## Log Volume Trends (Hourly)")
    report.append("")
    if result.log_volume_trends:
        # Show top entries
        report.append(format_table(
            result.log_volume_trends[:24],
            ["hour", "service", "log_count", "error_count", "error_rate"]
        ))
    else:
        report.append("_No log volume trend data available_\n")

    # SLO Violations
    report.append("## SLO Violations (Error Rate > 1%)")
    report.append("")
    if result.slo_violations:
        report.append(format_table(
            result.slo_violations[:20],
            ["service", "time_bucket", "error_rate_pct", "sample_count"]
        ))
    else:
        report.append("_No SLO violations detected_\n")

    # SLI Trends Summary
    report.append("## SLI Trends Summary")
    report.append("")
    if result.sli_trends:
        # Group by service and metric, show latest values
        latest_sli: dict[str, dict[str, Any]] = {}
        for trend in result.sli_trends:
            key = f"{trend.get('service', '')}-{trend.get('metric', '')}"
            if key not in latest_sli:
                latest_sli[key] = trend
        sli_summary = list(latest_sli.values())[:20]
        report.append(format_table(
            sli_summary,
            ["service", "metric", "value", "time_bucket"]
        ))
    else:
        report.append("_No SLI trend data available_\n")

    # Footer
    report.append("---")
    report.append("")
    report.append("*Report generated by Alt System Health Analyzer (Enhanced)*")

    return "\n".join(report)


# =============================================================================
# Main
# =============================================================================


def run_analysis(config: ClickHouseConfig, hours: int, verbose: bool = False) -> AnalysisResult:
    """Run complete analysis and return results."""
    if verbose:
        print(f"Connecting to ClickHouse at {config.host}:{config.port}...")

    client = clickhouse_connect.get_client(
        host=config.host,
        port=config.port,
        username=config.user,
        password=config.password,
    )

    result = AnalysisResult(hours_analyzed=hours)

    if verbose:
        print("Collecting service statistics...")
    result.service_stats = collect_service_stats(client, config.database, hours)

    if verbose:
        print("Collecting error trends...")
    result.error_trends = collect_error_trends(client, config.database, hours)

    if verbose:
        print("Collecting API performance...")
    result.api_performance = collect_api_performance(client, config.database, hours)

    if verbose:
        print("Identifying bottlenecks...")
    result.bottlenecks = collect_bottlenecks(client, config.database, hours)

    if verbose:
        print("Collecting error types...")
    result.error_types = collect_error_types(client, config.database, hours)

    if verbose:
        print("Collecting recent errors...")
    result.recent_errors = collect_recent_errors(client, config.database, hours)

    # Enhanced metrics collection
    if verbose:
        print("Collecting HTTP endpoint statistics...")
    result.http_endpoint_stats = collect_http_endpoint_stats(client, config.database, hours)

    if verbose:
        print("Collecting HTTP status distribution...")
    result.http_status_distribution = collect_http_status_distribution(client, config.database, hours)

    if verbose:
        print("Collecting trace span statistics...")
    result.span_type_stats = collect_span_type_stats(client, config.database, hours)

    if verbose:
        print("Collecting error spans...")
    result.error_spans = collect_error_spans(client, config.database, hours)

    if verbose:
        print("Collecting service dependencies...")
    result.service_dependencies = collect_service_dependencies(client, config.database, hours)

    if verbose:
        print("Collecting log severity distribution...")
    result.log_severity_distribution = collect_log_severity_distribution(client, config.database, hours)

    if verbose:
        print("Collecting log volume trends...")
    result.log_volume_trends = collect_log_volume_trends(client, config.database, hours)

    if verbose:
        print("Collecting SLI trends...")
    result.sli_trends = collect_sli_trends(client, config.database, hours)

    if verbose:
        print("Detecting SLO violations...")
    result.slo_violations = collect_slo_violations(client, config.database, hours)

    if verbose:
        print("Analyzing health and generating recommendations...")
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
        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        output_file = args.output_dir / f"system_health_{timestamp}.md"
        output_file.write_text(report)

        print(f"Report generated: {output_file}")

        if args.verbose:
            print("\n" + "=" * 80)
            print(report)

        return 0

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        if args.verbose:
            import traceback
            traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())
