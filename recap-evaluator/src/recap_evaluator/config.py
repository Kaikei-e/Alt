"""Configuration settings for recap-evaluator service."""

import os
from pathlib import Path

from pydantic import Field, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


def _read_secret_file(env_name: str) -> str | None:
    """Read `{env_name}_FILE` (Docker secret path) if set, else `{env_name}`.

    Mirrors the `_FILE` suffix convention used across the repo's compose
    services (e.g. recap-worker's `env_var()` in config.rs) so a password can
    be mounted as a Docker secret instead of a plaintext env var.
    """
    file_path = os.getenv(f"{env_name}_FILE")
    if file_path:
        return Path(file_path).read_text().strip()
    return os.getenv(env_name)


class EvaluatorWeights(BaseSettings):
    """Weight distribution for composite summary quality score."""

    model_config = SettingsConfigDict(env_prefix="WEIGHT_")

    geval: float = Field(default=0.40, description="G-Eval weight (40%)")
    bertscore: float = Field(default=0.25, description="BERTScore weight (25%)")
    faithfulness: float = Field(default=0.25, description="Faithfulness weight (25%)")
    rouge_l: float = Field(default=0.10, description="ROUGE-L weight (10%)")

    @model_validator(mode="after")
    def validate_sum(self) -> "EvaluatorWeights":
        total = self.geval + self.bertscore + self.faithfulness + self.rouge_l
        if abs(total - 1.0) > 0.001:
            raise ValueError(f"Weights must sum to 1.0, got {total}")
        return self


class AlertThresholds(BaseSettings):
    """Alert thresholds for quality metrics."""

    model_config = SettingsConfigDict(env_prefix="ALERT_")

    # Genre Classification
    genre_macro_f1_warn: float = Field(default=0.70)
    genre_macro_f1_critical: float = Field(default=0.60)

    # Clustering
    clustering_silhouette_warn: float = Field(default=0.25)
    clustering_silhouette_critical: float = Field(default=0.15)

    # G-Eval Summary Quality (1-5 scale)
    geval_coherence_warn: float = Field(default=3.5)
    geval_coherence_critical: float = Field(default=3.0)
    geval_consistency_warn: float = Field(default=3.5)
    geval_consistency_critical: float = Field(default=3.0)
    geval_fluency_warn: float = Field(default=3.5)
    geval_fluency_critical: float = Field(default=3.0)
    geval_relevance_warn: float = Field(default=3.5)
    geval_relevance_critical: float = Field(default=3.0)

    # ROUGE Metrics (0-1 scale)
    rouge_l_f1_warn: float = Field(default=0.30)
    rouge_l_f1_critical: float = Field(default=0.20)

    # BERTScore Metrics (0-1 scale)
    bertscore_f1_warn: float = Field(default=0.60)
    bertscore_f1_critical: float = Field(default=0.40)

    # Faithfulness Metrics (0-1 scale, higher is better)
    faithfulness_score_warn: float = Field(default=0.60)
    faithfulness_score_critical: float = Field(default=0.40)

    # Hallucination Rate (0-1 scale, lower is better)
    hallucination_rate_warn: float = Field(default=0.30)
    hallucination_rate_critical: float = Field(default=0.50)

    # Overall Quality Score (0-1 scale)
    overall_quality_score_warn: float = Field(default=0.50)
    overall_quality_score_critical: float = Field(default=0.30)

    # Pipeline Health
    pipeline_success_rate_warn: float = Field(default=0.95)
    pipeline_success_rate_critical: float = Field(default=0.90)

    def get_warn(self, metric_name: str) -> float | None:
        """Get warn threshold for a given metric."""
        attr = f"{metric_name}_warn"
        return getattr(self, attr, None)

    def get_critical(self, metric_name: str) -> float | None:
        """Get critical threshold for a given metric."""
        attr = f"{metric_name}_critical"
        return getattr(self, attr, None)


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    # Database — either a pre-built DSN (tests/local dev) or discrete
    # host/port/user/name plus RECAP_DB_PASSWORD_FILE (Docker secret,
    # consistent with recap-worker/recap-db-migrator in compose/recap.yaml).
    # No default credentials.
    recap_db_dsn: str | None = Field(
        default=None, description="PostgreSQL connection string for recap-db"
    )
    recap_db_host: str = Field(default="recap-db")
    recap_db_port: int = Field(default=5432)
    recap_db_user: str = Field(default="recap_user")
    recap_db_name: str = Field(default="recap")
    db_pool_min_size: int = Field(default=5, ge=1, le=50)
    db_pool_max_size: int = Field(default=20, ge=1, le=100)

    # Ollama
    ollama_url: str = Field(default="http://localhost:11434")
    ollama_model: str = Field(default="gemma4-e4b-12k")
    ollama_timeout: int = Field(default=120, ge=10, le=600)
    ollama_concurrency: int = Field(default=5, ge=1, le=20)

    # Recap Worker API
    recap_worker_url: str = Field(default="http://localhost:8081")

    # Evaluation Settings
    evaluation_window_days: int = Field(default=14, ge=1, le=90)
    geval_sample_size: int = Field(default=50, ge=1, le=200)

    # Scheduler
    evaluation_schedule: str = Field(
        default="0 6 * * *",
        description="Cron expression for scheduled evaluation",
    )
    enable_scheduler: bool = Field(default=True)

    # Performance
    evaluation_thread_pool_size: int = Field(default=4, ge=1, le=16)

    # CORS
    cors_allowed_origins: list[str] = Field(default=["http://localhost:3000"])

    # Logging
    log_level: str = Field(default="INFO")
    log_format: str = Field(default="json")

    # Server
    host: str = Field(default="0.0.0.0")
    port: int = Field(default=8080)

    @model_validator(mode="after")
    def build_recap_db_dsn(self) -> "Settings":
        """Build recap_db_dsn from discrete host/port/user/name + password
        when RECAP_DB_DSN is not set directly. Fails fast (raises) if neither
        a full DSN nor a password (via RECAP_DB_PASSWORD/_FILE) is available —
        never falls back to a passwordless or default-credential DSN.
        """
        if self.recap_db_dsn:
            return self

        password = _read_secret_file("RECAP_DB_PASSWORD")
        if not password:
            raise ValueError(
                "RECAP_DB_DSN or RECAP_DB_PASSWORD/RECAP_DB_PASSWORD_FILE must be set"
            )
        self.recap_db_dsn = (
            f"postgres://{self.recap_db_user}:{password}"
            f"@{self.recap_db_host}:{self.recap_db_port}/{self.recap_db_name}"
        )
        return self
