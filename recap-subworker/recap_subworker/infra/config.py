"""Configuration loading for recap-subworker."""

from functools import lru_cache
from typing import Literal

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class Settings(BaseSettings):
    """Runtime configuration derived from environment variables."""

    model_config = SettingsConfigDict(env_prefix="RECAP_SUBWORKER_", env_file=".env", extra="ignore")

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


@lru_cache(maxsize=1)
def get_settings() -> Settings:
    """Return cached settings instance."""

    return Settings()  # type: ignore[call-arg]
