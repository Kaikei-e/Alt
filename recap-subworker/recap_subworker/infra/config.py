"""Configuration loading for recap-subworker."""

from functools import lru_cache
from typing import Literal

from pydantic import Field, AliasChoices
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Runtime configuration derived from environment variables."""

    model_config = SettingsConfigDict(env_prefix="RECAP_SUBWORKER_", env_file=".env", extra="ignore")

    db_url: str = Field(
        "postgresql+asyncpg://recap_user:recap@recap-db:5432/recap",
        description="Async SQLAlchemy connection string",
        validation_alias=AliasChoices("RECAP_DB_URL", "RECAP_SUBWORKER_DB_URL"),
    )
    model_id: str = Field("BAAI/bge-m3", description="Primary sentence-transformer model identifier")
    distill_model_id: str = Field(
        "BAAI/bge-m3-distill-8l",
        description="Fallback model identifier for reduced CPU usage",
    )
    model_backend: Literal["sentence-transformers", "onnx"] = Field(
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
    max_docs_per_genre: int = Field(5000, ge=10, description="Maximum allowed documents per request")
    max_sentences_per_doc: int = Field(
        200,
        ge=10,
        description="Maximum sentences sampled from a single document",
    )
    max_sentences_per_cluster: int = Field(
        7,
        ge=1,
        le=50,
        description="Maximum representative sentences per cluster",
    )
    default_hdbscan_min_cluster_size: int = Field(5, ge=2)
    default_hdbscan_min_samples: int | None = Field(default=None, ge=1)
    default_umap_n_components: int = Field(25, ge=0)
    max_tokens_budget: int = Field(12_000, ge=512, description="Token budget per request")
    dedup_threshold: float = Field(0.92, ge=0.0, le=1.0)
    embed_cache_size: int = Field(
        256,
        ge=0,
        description="Maximum cached embedding entries when cache is enabled",
    )
    enable_umap_auto: bool = Field(
        True,
        description="Toggle automatic UMAP dimensionality reduction for large corpora",
    )
    umap_threshold_sentences: int = Field(
        10_000,
        ge=100,
        description="Sentence count above which UMAP pre-processing is attempted",
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
    random_seed: int = Field(42, description="Random seed for stochastic components")
    otel_exporter_otlp_endpoint: str | None = Field(
        default=None,
        description="Optional OTLP endpoint for OpenTelemetry traces",
    )


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return cached settings instance."""

    return Settings()  # type: ignore[call-arg]
