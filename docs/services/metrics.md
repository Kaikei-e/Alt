# metrics

_Last reviewed: July 7, 2026_

**Location:** `metrics/`
**Python:** 3.13+
**Type:** CLI tool (not an HTTP service)

## Purpose

Alt system health analyzer. This is a CLI tool (not an HTTP service) that analyzes logs and trace data accumulated in ClickHouse, generating Japanese Markdown reports.

## Directory Structure

```
metrics/
├── pyproject.toml          # Project config (Python >=3.13)
├── uv.lock
├── CLAUDE.md
├── src/
│   └── alt_metrics/
│       ├── __init__.py
│       ├── __main__.py     # CLI entrypoint
│       ├── cli.py          # CLI command handling
│       ├── config.py       # Config management (incl. thresholds)
│       ├── models.py       # Pydantic data models
│       ├── analysis.py     # Health score calculation
│       ├── exceptions.py   # Custom exceptions
│       ├── collectors/     # ClickHouse data collection
│       │   ├── base.py     # Legacy logs
│       │   ├── traces.py   # OTel traces
│       │   ├── logs.py     # OTel logs
│       │   ├── http.py     # HTTP metrics
│       │   ├── sli.py      # SLI/SLO
│       │   └── saturation.py # Resource utilization & queue saturation (Golden Signals)
│       └── reports/
│           ├── japanese.py # Japanese report generation
│           └── templates/
│               └── report_ja.md.j2
└── tests/
    ├── conftest.py         # Shared fixtures
    ├── test_analysis.py
    ├── test_config.py
    ├── test_error_budget.py
    ├── test_collectors/
    │   ├── test_base.py
    │   ├── test_traces.py
    │   ├── test_logs.py
    │   ├── test_http.py
    │   ├── test_sli.py
    │   └── test_saturation.py
    └── test_reports/
        └── test_japanese.py
```

## Configuration

### ClickHouse Connection
| Variable | Default | Description |
|----------|---------|-------------|
| `APP_CLICKHOUSE_HOST` | localhost | ClickHouse host |
| `APP_CLICKHOUSE_PORT` | 8123 | ClickHouse port |
| `APP_CLICKHOUSE_USER` | default | ClickHouse user |
| `APP_CLICKHOUSE_PASSWORD` | - | ClickHouse password |
| `APP_CLICKHOUSE_PASSWORD_FILE` | - | Password file path |
| `APP_CLICKHOUSE_DATABASE` | rask_logs | Database name |

### Threshold Settings (Optional)
| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_THRESHOLD_ERROR_RATE_CRITICAL` | 10.0 | Critical error rate % |
| `METRICS_THRESHOLD_LATENCY_CRITICAL_MS` | 10000 | Critical latency ms |

### Report Settings
| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_REPORT_LANGUAGE` | ja | Report language |
| `METRICS_OUTPUT_DIR` | ./scripts/reports | Output directory |

## CLI Usage

This service is a CLI tool, not an HTTP service. It has no server/daemon mode and does not expose any endpoints.

### Commands

```bash
# Run system health analysis (default: last 24 hours, Japanese report)
uv run python -m alt_metrics analyze --hours 24 --verbose

# Specify report language and output directory
uv run python -m alt_metrics analyze --hours 12 --lang ja --output-dir ./reports

# Validate ClickHouse connection and table accessibility
uv run python -m alt_metrics validate
uv run python -m alt_metrics validate --verbose
```

### analyze options

| Flag | Default | Description |
|------|---------|-------------|
| `--hours` | 24 | Analysis period in hours |
| `--lang` | ja | Report language (`ja`, `en`) |
| `--output-dir` | `./scripts/reports` | Report output directory |
| `--verbose` | off | Enable detailed output |

### validate options

| Flag | Default | Description |
|------|---------|-------------|
| `--verbose` | off | Enable detailed output |

## Health Score Calculation

```
Score = 100 - Error Rate Penalty - Latency Penalty - Log Gap Penalty

Error Rate:
  > 10%: -40pts, > 5%: -25pts, > 1%: -10pts, > 0.5%: -5pts

Latency (p95):
  > 10s: -30pts, > 5s: -20pts, > 1s: -10pts, > 500ms: -5pts

Log Gap:
  > 10min: -30pts, > 5min: -15pts
```

## Key Patterns

- **Pydantic Models**: Type-safe data models
- **Structlog**: Structured logging
- **Jinja2 Templates**: Report generation
- **Custom Exceptions**: `CollectorError`, `ConfigurationError`, etc.

## Known failure patterns

Patterns from postmortems that shape what this analyzer must surface; see [[crystallized-knowledge]] §14 for the detection-gap metapattern.

- **Almost every incident was detected by user report, not alerting**: ERROR/WARN evidence existed in ClickHouse for 20+ of the first 23 PMs but nothing watched it, and unimplemented "Detect" action items caused repeat incidents (PM-2026-008 → 016 → 020) → analysis output is only useful if it is actually run and reviewed.
- **Bimodal latency is invisible at p50**: cache-expiry fan-out spikes (634ms MarkAsRead) hid for weeks → always report p95/p99, never mean/p50 alone → PM-2026-019.
- **HTTP 200 / healthy ≠ functioning**: SSE served heartbeats only for 4 weeks with all-200 statuses → status-code SLIs miss silent failures; body size, stream counts, and reconnect intervals must become metrics → PM-2026-045.
- **Log volume is a first-class health metric**: a retry storm generated 148GB of logs in 48-72h, nearly filling the shared host, and the resulting disk-full zeroed an OAuth token file (65h silent ingestion stop) → PM-2026-042/043.
- **Per-service log formats differ in ClickHouse**: Rust tracing puts the message in `fields.message`, Python structlog in `event` → collectors need explicit per-service mapping; auto-detection is non-deterministic → [[000315]].
- **"No Data" triage order for ClickHouse-backed views**: (1) table actually exists (migration applied), (2) datasource provisioning, (3) `OTEL_EXPORTER_OTLP_ENDPOINT` env, (4) writer type vs CH schema (RowBinary is strict: `FixedString(N)` needs exact bytes, `Enum8` as i8, `DateTime64` as i64) → [[000074]].

## References

### Official Documentation
- [ClickHouse Python Client](https://clickhouse.com/docs/en/integrations/python)
- [Pydantic](https://docs.pydantic.dev/)
- [structlog](https://www.structlog.org/)

### Best Practices
- [Claude Code Best Practices](https://www.anthropic.com/engineering/claude-code-best-practices)
