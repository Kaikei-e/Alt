"""Configuration loading for recap-subworker."""

from functools import lru_cache
from typing import Literal
from urllib.parse import urlparse, urlunparse

from pydantic import Field, AliasChoices, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Runtime configuration derived from environment variables."""

    model_config = SettingsConfigDict(
        env_prefix="RECAP_SUBWORKER_",
        env_file=".env",
        extra="ignore",
        secrets_dir="/run/secrets"
    )

    recap_db_password: str | None = Field(default=None)

    @model_validator(mode='after')
    def inject_db_password(self) -> 'Settings':
        # If RECAP_SUBWORKER_DB_PASSWORD_FILE is set, read password from that file
        import os
        password_file = os.getenv("RECAP_SUBWORKER_DB_PASSWORD_FILE")
        if password_file and not self.recap_db_password:
            try:
                with open(password_file, "r") as f:
                    self.recap_db_password = f.read().strip()
            except Exception:
                pass  # Fall back to secrets_dir or default

        if self.recap_db_password:
            u = urlparse(self.db_url)
            # u.netloc is "user:pass@host:port"
            if '@' in u.netloc:
                user_pass, host_port = u.netloc.rsplit('@', 1)
                if ':' in user_pass:
                    user, _ = user_pass.split(':', 1)
                    new_netloc = f"{user}:{self.recap_db_password}@{host_port}"
                else:
                    new_netloc = f"{user_pass}:{self.recap_db_password}@{host_port}"

                self.db_url = urlunparse((u.scheme, new_netloc, u.path, u.params, u.query, u.fragment))
        return self

    db_url: str = Field(
        "postgresql+asyncpg://recap_user:recap@recap-db:5432/recap",
        description="Async SQLAlchemy connection string",
        validation_alias=AliasChoices("RECAP_DB_URL", "RECAP_SUBWORKER_DB_URL"),
    )
    enable_umap_auto: bool = Field(True, validation_alias=AliasChoices("RECAP_ENABLE_UMAP_AUTO", "RECAP_SUBWORKER_ENABLE_UMAP_AUTO"))
    enable_umap_force: bool = Field(
        False,
        description="Force UMAP usage regardless of sentence count threshold",
        validation_alias=AliasChoices("RECAP_ENABLE_UMAP_FORCE", "RECAP_SUBWORKER_ENABLE_UMAP_FORCE"),
    )
    umap_threshold_sentences: int = Field(10, validation_alias=AliasChoices("RECAP_UMAP_THRESHOLD_SENTENCES", "RECAP_SUBWORKER_UMAP_THRESHOLD_SENTENCES"))

    # UMAP Parameters
    umap_n_components: int = Field(20, validation_alias=AliasChoices("RECAP_UMAP_N_COMPONENTS", "RECAP_SUBWORKER_UMAP_N_COMPONENTS"))
    umap_n_neighbors: int = Field(30, validation_alias=AliasChoices("RECAP_UMAP_N_NEIGHBORS", "RECAP_SUBWORKER_UMAP_N_NEIGHBORS"))
    umap_min_dist: float = Field(0.0, validation_alias=AliasChoices("RECAP_UMAP_MIN_DIST", "RECAP_SUBWORKER_UMAP_MIN_DIST"))

    # HDBSCAN Parameters
    hdbscan_min_cluster_size: int = Field(5, validation_alias=AliasChoices("RECAP_HDBSCAN_MIN_CLUSTER_SIZE", "RECAP_SUBWORKER_HDBSCAN_MIN_CLUSTER_SIZE"))
    hdbscan_min_samples: int = Field(5, validation_alias=AliasChoices("RECAP_HDBSCAN_MIN_SAMPLES", "RECAP_SUBWORKER_HDBSCAN_MIN_SAMPLES"))
    hdbscan_cluster_selection_method: Literal["eom", "leaf"] = Field("eom", validation_alias=AliasChoices("RECAP_HDBSCAN_SELECTION_METHOD", "RECAP_SUBWORKER_HDBSCAN_SELECTION_METHOD"))
    http_host: str = Field(
        "0.0.0.0",
        description="Bind host for the HTTP server",
        validation_alias=AliasChoices("HOST", "RECAP_SUBWORKER_HOST"),
    )
    http_port: int = Field(
        8002,
        ge=1,
        le=65535,
        description="Bind port for the HTTP server",
        validation_alias=AliasChoices("PORT", "RECAP_SUBWORKER_PORT"),
    )
    model_id: str = Field(
        "BAAI/bge-m3", description="Primary sentence-transformer model identifier"
    )
    distill_model_id: str = Field(
        "BAAI/bge-m3-distill-8l",
        description="Fallback model identifier for reduced CPU usage",
    )
    model_backend: Literal["sentence-transformers", "onnx", "hash"] = Field(
        "sentence-transformers",
        description="Embedding backend selection",
    )
    device: str = Field("cpu", description="Primary device for embedding inference")
    batch_size: int = Field(64, ge=1, description="Maximum sentences per embedding batch")
    max_total_sentences: int = Field(
        6000,
        ge=1,
        description="Service-side guardrail for the total number of sentences per request",
    )
    max_docs_per_genre: int = Field(
        5000, ge=10, description="Maximum allowed documents per request"
    )
    max_sentences_per_doc: int = Field(
        200,
        ge=10,
        description="Maximum sentences sampled from a single document",
    )
    max_sentences_per_cluster: int = Field(
        8,
        ge=1,
        le=50,
        description="Maximum representative sentences per cluster",
    )
    max_genre_sentences: int = Field(
        15,
        ge=1,
        le=50,
        description="Maximum sentences for genre-level highlight summary",
    )
    default_hdbscan_min_cluster_size: int = Field(5, ge=2)
    default_hdbscan_min_samples: int | None = Field(default=None, ge=1)
    default_umap_n_components: int = Field(25, ge=0)
    clustering_search_range_mcs_window_lower: int = Field(
        2,
        ge=0,
        description="Lower bound window size for min_cluster_size grid search (base - lower)",
    )
    clustering_search_range_mcs_window_upper: int = Field(
        5,
        ge=0,
        description="Upper bound window size for min_cluster_size grid search (base + upper)",
    )
    clustering_search_range_ms_max: int = Field(
        6,
        ge=2,
        description="Maximum value for min_samples grid search (range is 1 to max-1)",
    )
    max_tokens_budget: int = Field(12_000, ge=512, description="Token budget per request")
    dedup_threshold: float = Field(0.92, ge=0.0, le=1.0)
    embed_cache_size: int = Field(
        256,
        ge=0,
        description="Maximum cached embedding entries when cache is enabled",
    )
    prometheus_namespace: str = Field(
        "recap_subworker",
        description="Prefix for exported Prometheus metrics",
    )
    log_level: str = Field("INFO", description="Application log level")
    warmup_parallelism: int = Field(
        2,
        ge=1,
        description="Number of concurrent batches during warmup prime",
    )
    process_pool_size: int = Field(
        2,
        ge=1,
        description="Number of worker processes for CPU-heavy tasks",
    )
    pipeline_mode: Literal["inprocess", "processpool"] = Field(
        "inprocess",
        description="Execution strategy for recap pipeline workloads",
    )
    pipeline_worker_processes: int = Field(
        2,
        ge=1,
        description="Number of dedicated pipeline worker processes when process pools are enabled",
    )
    pipeline_worker_max_tasks_per_child: int = Field(
        50,
        ge=1,
        description="Maximum number of tasks a pipeline worker process handles before being replaced (prevents memory leaks)",
    )
    pipeline_worker_init_timeout_seconds: int = Field(
        300,
        ge=10,
        description="Timeout in seconds for pipeline worker process initialization",
    )
    max_background_runs: int = Field(
        2,
        ge=1,
        description="Maximum concurrent pipeline runs handled inside this instance",
    )
    run_execution_timeout_seconds: int = Field(
        2400,  # 40 minutes - matches recap-worker's MAX_POLL_ATTEMPTS (40 × 60s)
        ge=60,
        description="Hard timeout for a single genre run before it is aborted",
    )
    queue_warning_threshold: int = Field(
        25,
        ge=1,
        description="Log a warning when more than this many background runs are queued",
    )
    gunicorn_workers: int | None = Field(
        default=None,
        ge=1,
        description="Override for gunicorn worker processes; defaults to 2*CPU+1 when unset",
    )
    gunicorn_max_requests: int = Field(
        10000,
        ge=50,
        description="Number of requests a worker serves before recycling",
    )
    gunicorn_max_requests_jitter: int = Field(
        50,
        ge=0,
        description="Jitter added to max_requests to stagger worker recycling",
    )
    gunicorn_worker_timeout: int = Field(
        120,
        ge=30,
        description="Seconds before gunicorn kills an unresponsive worker",
    )
    gunicorn_graceful_timeout: int = Field(
        30,
        ge=5,
        description="Grace period for gracefully shutting down workers",
    )
    random_seed: int = Field(42, description="Random seed for stochastic components")
    otel_exporter_otlp_endpoint: str | None = Field(
        default=None,
        description="Optional OTLP endpoint for OpenTelemetry traces",
    )
    learning_graph_margin: float = Field(
        0.15,
        ge=0.0,
        description="Graph margin applied when building learning snapshot summaries",
    )
    learning_snapshot_days: int = Field(
        7,
        ge=1,
        le=30,
        description="Lookback window in days when generating learning snapshots (based on article published_at)",
    )
    learning_cluster_genres: str = Field(
        "",
        description="Comma-separated genres used when generating cluster drafts. If empty or '*', automatically detects all available genres from database (default behavior)",
    )
    learning_auto_detect_genres: bool = Field(
        True,
        description="If True, automatically detect and use all genres from database instead of using learning_cluster_genres. Default is True.",
    )
    recap_worker_learning_url: str = Field(
        "http://recap-worker:9005/admin/genre-learning",
        description="Full endpoint to POST genre learning payloads",
    )
    learning_request_timeout_seconds: float = Field(
        5.0,
        ge=0.5,
        description="Timeout for HTTP requests sending learning payloads",
    )
    learning_scheduler_enabled: bool = Field(
        True,
        description="Enable automatic periodic learning task execution",
    )
    learning_scheduler_interval_hours: float = Field(
        4.0,
        ge=0.001,
        le=168.0,
        description="Interval between learning task executions (hours)",
    )
    learning_bayes_enabled: bool = Field(
        True,
        description="Enable Bayes optimization for threshold tuning",
    )
    learning_bayes_iterations: int = Field(
        30,
        ge=10,
        le=100,
        description="Number of iterations for Bayes optimization",
    )
    learning_bayes_seed: int = Field(
        42,
        description="Random seed for Bayes optimization",
    )
    learning_bayes_min_samples: int = Field(
        100,
        ge=50,
        description="Minimum number of entries required to run Bayes optimization",
    )
    graph_build_enabled: bool = Field(
        True,
        description="Enable tag_label_graph rebuild before learning",
    )
    graph_build_windows: str = Field(
        "7,30",
        description="Comma-separated window days for graph building",
    )
    graph_build_max_tags: int = Field(
        6,
        ge=1,
        description="Maximum tags per article to consider",
    )
    graph_build_min_confidence: float = Field(
        0.3,
        ge=0.0,
        le=1.0,
        description="Minimum tag confidence to include",
    )
    graph_build_min_support: int = Field(
        3,
        ge=1,
        description="Minimum article count for an edge",
    )
    graph_build_max_concurrency: int = Field(
        1,
        ge=1,
        description="Maximum concurrent admin jobs (graph/learning) using the shared semaphore",
    )
    genre_classifier_model_path: str = Field(
        "data/genre_classifier.joblib",
        description="Path to the trained genre classifier model",
    )
    genre_subworker_threshold_overrides: str = Field(
        "{}",
        description="JSON string mapping genres to custom threshold values (overrides defaults and model-specific thresholds)",
        validation_alias=AliasChoices("RECAP_GENRE_THRESHOLDS", "RECAP_SUBWORKER_GENRE_THRESHOLDS"),
    )


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return cached settings instance."""

    return Settings()  # type: ignore[call-arg]
