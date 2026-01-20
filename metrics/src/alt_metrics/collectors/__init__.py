"""ClickHouseメトリクスコレクター

各コレクターはClickHouseからデータを収集し、
エラー時は適切なログ出力と例外を発生させます。
"""

from alt_metrics.collectors.base import collect_error_trends, collect_service_stats
from alt_metrics.collectors.http import (
    collect_http_endpoint_stats,
    collect_http_status_distribution,
)
from alt_metrics.collectors.logs import (
    collect_error_types,
    collect_log_severity_distribution,
    collect_log_volume_trends,
    collect_recent_errors,
)
from alt_metrics.collectors.sli import (
    collect_sli_trends,
    collect_slo_violations,
)
from alt_metrics.collectors.traces import (
    collect_api_performance,
    collect_bottlenecks,
    collect_error_spans,
    collect_service_dependencies,
    collect_service_latency,
    collect_span_type_stats,
)
from alt_metrics.collectors.saturation import (
    collect_queue_saturation,
    collect_resource_utilization,
)

__all__ = [
    # Base collectors
    "collect_service_stats",
    "collect_error_trends",
    # Trace collectors
    "collect_api_performance",
    "collect_bottlenecks",
    "collect_service_latency",
    "collect_span_type_stats",
    "collect_error_spans",
    "collect_service_dependencies",
    # Log collectors
    "collect_error_types",
    "collect_recent_errors",
    "collect_log_severity_distribution",
    "collect_log_volume_trends",
    # HTTP collectors
    "collect_http_endpoint_stats",
    "collect_http_status_distribution",
    # SLI collectors
    "collect_sli_trends",
    "collect_slo_violations",
    # Saturation collectors (Golden Signals)
    "collect_resource_utilization",
    "collect_queue_saturation",
]
