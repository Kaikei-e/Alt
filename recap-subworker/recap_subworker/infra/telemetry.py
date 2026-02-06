"""Telemetry helpers for metrics and logging."""

from __future__ import annotations

from prometheus_client import Counter, Gauge, Histogram
from prometheus_fastapi_instrumentator import Instrumentator

from ..infra.config import Settings

# Module-level singleton to avoid lru_cache issues with unhashable Settings
_instrumentator: Instrumentator | None = None


def get_instrumentator(settings: Settings) -> Instrumentator:
    """Create or return the shared Instrumentator instance."""
    global _instrumentator
    if _instrumentator is None:
        _instrumentator = Instrumentator()
    return _instrumentator


def setup_metrics(app, settings: Settings) -> None:
    """Attach Prometheus instrumentation to the FastAPI app."""

    instrumentator = get_instrumentator(settings)
    instrumentator.instrument(app)
    instrumentator.expose(app)


REQUEST_EMBED_SENTENCES = Histogram(
    "recap_embed_sentences_total",
    "Number of sentences processed per embedding request",
    buckets=(10, 50, 100, 250, 500, 1000, 2000, 4000, 8000),
)

REQUEST_PROCESS_SECONDS = Histogram(
    "recap_request_process_seconds",
    "End-to-end processing duration per request",
    buckets=(0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60),
)

EMBED_SECONDS = Histogram(
    "recap_embed_seconds",
    "Embedding inference duration",
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10),
)

HDBSCAN_SECONDS = Histogram(
    "recap_hdbscan_seconds",
    "Clustering duration for HDBSCAN",
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10),
)

MMR_SELECTED = Counter(
    "recap_mmr_selected_total",
    "Total number of sentences selected by MMR",
)

DEDUP_REMOVED = Counter(
    "recap_dedup_removed_total",
    "Total number of sentences filtered due to deduplication",
)

# --- New metrics from refactoring plan ---

UMAP_SECONDS = Histogram(
    "recap_umap_seconds",
    "UMAP dimensionality reduction duration",
    buckets=(0.1, 0.25, 0.5, 1, 2, 5, 10, 20),
)

DEDUP_SECONDS = Histogram(
    "recap_dedup_seconds",
    "Deduplication stage duration (brute-force or FAISS)",
    labelnames=("method",),
    buckets=(0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5),
)

EMBED_CACHE_HITS = Counter(
    "recap_embed_cache_hits_total",
    "Total embedding cache hits",
)

EMBED_CACHE_MISSES = Counter(
    "recap_embed_cache_misses_total",
    "Total embedding cache misses",
)

DB_POOL_CHECKED_OUT = Gauge(
    "recap_db_pool_checked_out",
    "Number of DB connections currently checked out from the pool",
)

DB_POOL_SIZE = Gauge(
    "recap_db_pool_size",
    "Current size of the DB connection pool",
)

WORKER_RSS_BYTES = Gauge(
    "recap_worker_rss_bytes",
    "Resident set size of the worker process in bytes",
)

FAISS_DEDUP_ITEMS = Histogram(
    "recap_faiss_dedup_items",
    "Number of items processed by FAISS dedup",
    buckets=(50, 100, 250, 500, 1000, 2000, 4000, 8000),
)

# Learning task metrics
LEARNING_TASK_DURATION = Histogram(
    "recap_subworker_learning_task_duration_seconds",
    "Duration of learning task execution in seconds",
    buckets=(1.0, 5.0, 10.0, 30.0, 60.0, 120.0, 300.0, 600.0),
)

LEARNING_TASK_SUCCESS = Counter(
    "recap_subworker_learning_task_success_total",
    "Total number of successful learning task executions",
)

LEARNING_TASK_FAILURE = Counter(
    "recap_subworker_learning_task_failure_total",
    "Total number of failed learning task executions",
)

# Admin job metrics (graph build / learning)
ADMIN_JOB_STATUS_TOTAL = Counter(
    "recap_subworker_admin_job_status_total",
    "Total admin job status transitions",
    labelnames=("kind", "status"),
)

ADMIN_JOB_DURATION_SECONDS = Histogram(
    "recap_subworker_admin_job_duration_seconds",
    "Duration of admin jobs in seconds",
    labelnames=("kind",),
    buckets=(
        1.0,
        5.0,
        10.0,
        30.0,
        60.0,
        120.0,
        300.0,
        600.0,
        1200.0,
    ),
)
