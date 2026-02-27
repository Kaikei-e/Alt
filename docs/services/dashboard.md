# Dashboard

_Last reviewed: February 28, 2026_

**Location:** `dashboard/`
**Python:** 3.12+
**Type:** Streamlit web application with SSE real-time monitoring

## Purpose

Recap system evaluation dashboard. A Streamlit-based web UI that provides real-time monitoring and evaluation metrics for the entire Recap pipeline. It combines database-driven metric visualization (classification, clustering, summarization) with live system resource monitoring via a built-in SSE server.

The service runs two processes inside a single container:
1. **Streamlit app** (port 8501) -- interactive dashboard UI querying `recap-db`
2. **SSE server** (port 8000) -- lightweight HTTP server streaming CPU, memory, GPU, and process data to the browser

## Directory Structure

```
dashboard/
├── pyproject.toml                  # Project config (Python >=3.12)
├── uv.lock
├── Dockerfile                      # Multi-stage build (python:3.12-slim)
├── entrypoint.sh                   # Starts SSE server + Streamlit
├── app.py                          # Streamlit application entry point
├── sse_server.py                   # Standalone SSE HTTP server (ThreadingHTTPServer)
├── system_monitor.py               # Host metrics collection (free, ps, nvidia-smi, psutil)
├── utils.py                        # DB connection, time windows, data fetching helpers
└── tabs/
    ├── __init__.py
    ├── overview.py                 # Recent activity summary
    ├── classification.py           # Genre classification metrics (accuracy, F1, per-genre)
    ├── clustering.py               # Clustering quality (DBCV, silhouette, noise ratio)
    ├── summarization.py            # Summarization quality (faithfulness, coverage, MMR)
    ├── system_monitor_tab.py       # Real-time system monitor (SSE-powered HTML component)
    ├── system_monitor_sse_client.py  # JS code loader for SSE client
    ├── log_analysis.py             # Error log analysis from log_errors table
    ├── admin_jobs.py               # Admin/graph/learning job status
    ├── recap_jobs.py               # Recap job status tracking
    └── static/
        └── sse_client.js           # Browser-side SSE client with reconnection logic
```

## Architecture & Flow

```mermaid
flowchart LR
    User[User Browser]
    ST[Streamlit :8501]
    SSE[SSE Server :8000]
    RDB[(recap-db<br/>PostgreSQL 18)]
    Host[Host System<br/>CPU / Mem / GPU]

    User --> ST
    ST -->|SQLAlchemy| RDB
    User -.->|EventSource| SSE
    SSE -->|free, ps,<br/>nvidia-smi| Host
```

The Streamlit app queries `recap-db` for pipeline metrics stored in `recap_system_metrics`, `log_errors`, `admin_jobs`, and `recap_jobs` tables. The SSE server runs as a separate process (started by `entrypoint.sh`) and gathers host-level metrics using subprocess commands (`free -b`, `ps aux`, `nvidia-smi`) and optionally `psutil`.

### Dual-Process Startup

`entrypoint.sh` manages both processes with graceful shutdown:

1. Starts `sse_server.py` in background
2. Waits 2 seconds, verifies SSE server is alive
3. Starts `streamlit run app.py` in background
4. Waits for either process to exit; runs cleanup on SIGTERM/SIGINT

## Tabs

| Tab | Key Metrics | Data Source |
|-----|-------------|-------------|
| **Overview** | Recent activity feed | `recap_system_metrics` |
| **Classification** | Accuracy, Macro F1, Hamming Loss, per-genre Precision/Recall/F1, dynamic thresholds | `recap_system_metrics` (metric_type=`classification`) |
| **Clustering** | DBCV score, Silhouette score, cluster count, noise ratio, cluster size distribution | `recap_system_metrics` (metric_type=`clustering`) |
| **Summarization** | Faithfulness (alignment), coverage score, MMR diversity, JSON errors, processing time | `recap_system_metrics` (metric_type=`summarization`) |
| **System Monitor** | CPU %, memory %, GPU utilization/temperature, hanging processes, top processes | SSE stream (real-time, 2s interval) |
| **Log Analysis** | Error counts by type, critical error watchlist (DB duplicate key, LLM 422, GPU OOM) | `log_errors` |
| **Admin Jobs** | Running/succeeded/failed counts, job duration | `admin_jobs` |
| **Recap Jobs** | Running/completed/failed counts, last stage tracking | `recap_jobs` |

## SSE Server

The SSE server (`sse_server.py`) is a `ThreadingHTTPServer` that streams system metrics to the browser.

### Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/stream` | GET | SSE event stream. Sends JSON payload every 2 seconds with `memory`, `cpu`, `gpu`, `hanging_count`, `top_processes`. |
| `/health` | GET | Returns JSON `{ status, service, port, system_monitor: { memory_available, cpu_available } }`. |

### SSE Payload Structure

```json
{
  "memory": { "total": 67252326400, "used": 18317783040, "available": 48067821568, "percent": 27.2 },
  "cpu": { "percent": 15.3 },
  "gpu": { "available": true, "gpus": [{ "index": "0", "utilization": 45.0, "memory_used": 4096.0, "memory_total": 24576.0, "temperature": 62, "name": "NVIDIA RTX 4090", "memory_percent": 16.7 }] },
  "hanging_count": 0,
  "top_processes": [{ "pid": "1234", "name": "python app.py", "cpu_percent": 12.5, "memory_mb": 512.3 }]
}
```

### System Monitor Data Collection (`system_monitor.py`)

| Function | Method | Fallback |
|----------|--------|----------|
| `get_memory_info()` | `free -b` (subprocess) | Returns zeroed dict |
| `get_cpu_info()` | `psutil.cpu_percent()` | `/proc/stat` parsing, then `top -bn2` |
| `get_gpu_info()` | `nvidia-smi --query-gpu=...` (subprocess) | Returns `{ available: false }` |
| `count_hanging_processes()` | `ps aux` filtering for `spawn_main`/`multiprocessing fork` | Returns 0 |
| `get_top_processes(N)` | `ps aux --sort=-%cpu` (subprocess) | Returns empty list |

### Browser SSE Client (`tabs/static/sse_client.js`)

The browser client uses the `EventSource` API with:
- Automatic reconnection with exponential backoff (base 3s, max 30s, with jitter)
- Configurable max reconnection attempts (default 5)
- Health check before connection attempt
- CORS support for cross-origin iframe scenarios (Streamlit `components.html`)
- Configurable SSE endpoint via query parameters (`sse_host`, `sse_port`, `sse_protocol`, `sse_path`)
- Default path: `/sse/dashboard/stream` (via Nginx proxy) or `/stream` (direct)

## Configuration & Environment Variables

### Database Connection (Docker Compose)

| Variable | Default | Description |
|----------|---------|-------------|
| `RECAP_DB_HOST` | `recap-db` | Database hostname |
| `RECAP_DB_PORT` | `5432` | Database port |
| `RECAP_DB_USER` | (from `.env`) | Database user |
| `RECAP_DB_PASSWORD_FILE` | `/run/secrets/recap_db_password` | Password secret file path |
| `RECAP_DB_NAME` | (from `.env`) | Database name |

### Application Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `RECAP_DB_DSN` | `postgresql://recap:recap@localhost:5435/recap_db` | Full DSN (used in `utils.py` for local dev) |
| `SSE_PORT` | `8000` | SSE server listen port |

### Time Windows

| Label | Duration | Seconds |
|-------|----------|---------|
| `4h` | 4 hours | 14400 |
| `24h` | 24 hours | 86400 |
| `3d` | 3 days | 259200 |

## Database Tables Queried

| Table | Tab(s) | Key Columns |
|-------|--------|-------------|
| `recap_system_metrics` | Overview, Classification, Clustering, Summarization | `job_id`, `metric_type`, `timestamp`, `metrics` (JSONB) |
| `log_errors` | Log Analysis | `timestamp`, `error_type`, `error_message`, `raw_line` |
| `admin_jobs` | Admin Jobs | `job_id`, `kind`, `status`, `started_at`, `finished_at`, `error`, `result` |
| `recap_jobs` | Recap Jobs | `job_id`, `status`, `last_stage`, `kicked_at`, `updated_at` |

## Docker

### Build

Multi-stage build using `python:3.12-slim`:
- **Builder stage**: Installs `uv` (0.5.14), syncs dependencies with `--frozen --no-dev`
- **Runtime stage**: Copies `.venv` from builder, installs `curl` and `procps` for health checks and system monitoring
- Runs as non-root user `dashboard`

### Ports

| Host Port | Container Port | Service |
|-----------|---------------|---------|
| 8501 | 8501 | Streamlit UI |
| 8502 | 8000 | SSE server |

### Health Check

```dockerfile
HEALTHCHECK --interval=30s --timeout=10s --start-period=40s --retries=3 \
    CMD curl --fail http://localhost:8501/_stcore/health && curl --fail http://localhost:8000/health || exit 1
```

### GPU Reservation

The Docker Compose definition in `compose/recap.yaml` reserves all NVIDIA GPUs:
```yaml
deploy:
  resources:
    reservations:
      devices:
        - driver: nvidia
          count: all
          capabilities: [gpu, utility]
```

This is required for `nvidia-smi` to function inside the container for GPU monitoring. The dashboard itself does not perform GPU computation -- the reservation is solely for monitoring visibility.

## Dependencies

### Runtime

| Package | Version | Purpose |
|---------|---------|---------|
| `streamlit` | 1.54.0 | Dashboard UI framework |
| `pandas` | 2.2.3 | Data manipulation and display |
| `sqlalchemy` | 2.0.46 | Database connection and queries |
| `psycopg2-binary` | 2.9.11 | PostgreSQL driver |
| `psutil` | >=5.9.0 | CPU metrics collection (fallback to `/proc/stat` if unavailable) |

### Infrastructure

- **recap-db** (PostgreSQL 18) -- required; `depends_on: service_healthy` in Compose
- **NVIDIA GPU driver** -- optional; `nvidia-smi` must be accessible for GPU monitoring; gracefully degrades if unavailable

## Testing & Tooling

```bash
# Local development (requires recap-db running on localhost:5435)
cd dashboard && streamlit run app.py

# Docker startup
docker compose -f compose/compose.yaml -p alt up -d dashboard

# Or via the recap profile
docker compose -f compose/recap.yaml -p alt up -d dashboard

# Health checks
curl http://localhost:8501/_stcore/health   # Streamlit
curl http://localhost:8502/health            # SSE server (host port 8502 -> container 8000)
```

## Operational Runbook

1. Start the service: `docker compose -f compose/compose.yaml -p alt up -d dashboard`
2. Verify both processes are healthy:
   - `curl http://localhost:8501/_stcore/health` (Streamlit)
   - `curl http://localhost:8502/health` (SSE server)
3. Open `http://localhost:8501` in browser
4. Select time range (4h / 24h / 3d) at the top of the page
5. Navigate tabs to inspect pipeline metrics
6. System Monitor tab shows live CPU/memory/GPU -- check connection status indicator at bottom right

### Troubleshooting

| Symptom | Cause | Resolution |
|---------|-------|------------|
| Streamlit loads but shows "No data" | recap-db not running or empty tables | Verify recap-db health: `curl http://localhost:5435` |
| System Monitor shows "Connecting..." | SSE server not started or port mismatch | Check `docker logs dashboard` for SSE startup errors |
| GPU section shows "Not Available" | nvidia-smi not accessible in container | Verify GPU reservation in compose and NVIDIA runtime |
| Log Analysis shows "log_errors table not found" | Table not yet created in recap-db | Run Atlas migrations: `docker compose -f compose/recap.yaml -p alt up recap-db-migrator` |

## Observability

- **Streamlit logs**: Standard Python logging (INFO level, timestamped)
- **SSE server logs**: Separate logger (`sse_server`), logs connection/disconnection events
- **Docker health check**: Dual check (Streamlit `/_stcore/health` + SSE `/health`) every 30s
- **rask.group label**: `dashboard` (for log aggregation via `dashboard-logs` forwarder in `compose/logging.yaml`)

## LLM Notes

- The dashboard is read-only; it does not modify any database records
- SSE server runs as a separate OS process (not a thread) inside the same container, managed by `entrypoint.sh`
- The SSE client JavaScript runs inside a Streamlit `components.html` iframe (`about:srcdoc`), requiring special hostname detection logic to resolve the SSE server URL
- `system_monitor.py` deliberately uses subprocess commands (`free`, `ps`, `nvidia-smi`) instead of pure-Python libraries for better reliability in containerized environments
- The GPU reservation in Docker Compose is for monitoring only, not for ML inference
