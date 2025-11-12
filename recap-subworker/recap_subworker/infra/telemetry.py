"""Telemetry helpers for metrics and logging."""

from __future__ import annotations

from prometheus_client import Counter, Histogram
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
