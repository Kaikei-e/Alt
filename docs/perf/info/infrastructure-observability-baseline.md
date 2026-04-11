# Infrastructure Observability Baseline: Auditing a 28-Service Microservice Platform

## Overview

This report documents a baseline health audit of the Alt platform — a microservice architecture with 28 services orchestrated via Docker Compose. The audit covered a 4.5-hour observation window and combined data from four sources: time-series metrics, HTTP access logs, structured application logs (OpenTelemetry), and container runtime statistics.

The goal was to establish a performance baseline, identify error hotspots, and surface resource bottlenecks before scaling the platform.

## Data Sources and Methodology

| Source | What It Captures | Collection Method |
|--------|-----------------|-------------------|
| Time-series metrics (Prometheus) | Service-level metrics, counters, histograms | Pull-based scraping |
| HTTP access logs (ClickHouse) | Request paths, status codes, response sizes | Nginx access log ingestion |
| Structured logs (OpenTelemetry → ClickHouse) | Application-level events with severity | OTEL collector pipeline |
| Container stats (Docker) | CPU, memory, network I/O, disk I/O | `docker stats` snapshots |

### Observation Window

- **Duration**: ~4.5 hours
- **Total HTTP requests**: 1,624
- **Total OTEL log entries**: 80,010
- **Services running**: 28/28 (all healthy)

## Findings

### 1. HTTP Traffic Profile

| Metric | Value |
|--------|-------|
| Overall error rate (4xx + 5xx) | 1.42% |
| 2xx (Success) | 98.46% |
| 4xx (Client Error) | 1.17% |
| 5xx (Server Error) | 0.25% |

**Traffic was dominated by health check polling** — 80% of all requests were internal health checks. Excluding those, actual API traffic was ~315 requests over 4.5 hours, averaging ~1.2 requests/minute during low-traffic periods.

A traffic spike of 749 requests in a single 5-minute interval corresponded to batch processing activity. Most errors concentrated during this spike.

**Top API endpoints by volume:**

| Endpoint Type | Requests | Avg Response Size |
|--------------|----------|-------------------|
| Article content fetch | 113 | 2.8 KB |
| Unread feed list | 83 | 2.8 KB |
| Mark as read | 70 | 69 B |
| Log aggregation | 18 | 146 B |
| Streaming summarization | 6 | 37.6 KB |

The streaming summarization endpoint stood out with 37 KB average response size — expected for an LLM-generated summary stream.

### 2. Service-Level Error Analysis

| Service | INFO | WARN | ERROR | Error Rate |
|---------|------|------|-------|------------|
| alt-backend | 47,811 | 22 | 64 | 0.13% |
| pre-processor | 26,796 | 440 | 640 | 2.30% |
| news-creator | — | 2,026 | 121 | HIGH |
| tag-generator | 1,383 | 47 | 15 | 1.06% |
| search-indexer | 574 | — | — | 0% |

**Error hotspots:**

1. **pre-processor (640 errors, 2.30%)**: Primary failures were LLM response timeouts and configuration lookup failures. Errors occurred at ~30s intervals, matching scheduled job execution.

2. **news-creator (121 errors + 2,026 warnings)**: The high warning count suggested retry loops or deprecation warnings in the content generation pipeline.

3. **alt-backend (64 errors, 0.13%)**: Repeated database persistence failures for article summaries — likely a constraint violation or connection pool issue.

### 3. Container Resource Usage

**CPU-intensive containers:**

| Container | CPU % | Notes |
|-----------|-------|-------|
| ClickHouse | 10.29% | Log ingestion write load |
| Grafana | 6.30% | Dashboard rendering |
| Kratos DB | 5.88% | Auth session queries |
| auth-hub | 4.82% | Token validation |

**Memory-intensive containers:**

| Container | Memory Usage | Notes |
|-----------|-------------|-------|
| news-creator | 2.59 GiB | ML model loaded in memory |
| tag-generator | 1.68 GiB | NLP model loaded in memory |
| ClickHouse | 1.33 GiB | Write buffer + indexes |

**Disk I/O leader:**

ClickHouse wrote 22 GB during the observation window — the logging system was heavily utilized. This warranted monitoring for disk capacity and evaluating TTL policies for log retention.

### 4. Health Check Results

All 28 containers passed health checks:
- 14 containers with explicit health checks: all **Healthy**
- 14 containers without health checks: all **Up**

Every service with an HTTP health endpoint responded correctly (database connected, search available, metrics healthy).

## Bottleneck Summary

| Bottleneck | Location | Impact | Severity |
|------------|----------|--------|----------|
| LLM response timeouts | pre-processor | Content processing pipeline stalls | HIGH |
| Database save failures | alt-backend | Article summaries not persisted | HIGH |
| Log storage write load | ClickHouse | 22 GB writes, 10% CPU | MEDIUM |
| ML service memory | news-creator | 2.59 GiB resident | LOW |

## Recommendations

### Immediate

1. **Fix routing for log aggregation endpoint** — 18 failed requests due to missing reverse proxy configuration
2. **Investigate database persistence failures** — Check for constraint violations and connection pool exhaustion
3. **Monitor LLM container health** — Multiple timeout errors suggest GPU memory pressure or model loading issues

### Short-term

4. **Review pre-processor configuration** — Frequent lookup failures point to missing database records
5. **Monitor ClickHouse disk usage** — 22 GB writes in 4.5 hours requires disk capacity planning and TTL optimization
6. **Audit news-creator warning volume** — 2,026 warnings is disproportionately high

### Long-term

7. **Set memory limits for ML services** — news-creator and tag-generator hold large models in memory without explicit limits
8. **Add distributed tracing** — Leverage existing OpenTelemetry infrastructure to trace cross-service request paths

## Key Takeaways

1. **Health checks alone don't reveal problems**: All 28 services were "healthy," yet the platform had a 2.3% error rate in its content processing pipeline and was silently dropping article summaries.

2. **Log volume analysis reveals hidden load**: ClickHouse's 22 GB write volume was invisible without disk I/O monitoring. The logging system itself was the heaviest I/O consumer.

3. **Separate health check traffic from real traffic**: 80% of HTTP requests were health checks. Without filtering these out, traffic analysis would be meaningless.

4. **Error patterns reveal scheduled job failures**: The 30-second interval error pattern immediately pointed to a cron-like scheduled job, narrowing the investigation scope.

5. **Four data sources are the minimum**: No single data source (metrics, HTTP logs, application logs, container stats) would have revealed all the issues found. Cross-referencing was essential.
