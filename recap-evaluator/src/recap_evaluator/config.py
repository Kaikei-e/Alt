"""Configuration settings for recap-evaluator service."""

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class AlertThreshold(BaseSettings):
    """Alert threshold configuration."""

    warn: float
    critical: float


class Settings(BaseSettings):
    """Application settings loaded from environment variables."""

    model_config = SettingsConfigDict(
        env_file=".env",
        env_file_encoding="utf-8",
        extra="ignore",
    )

    # Database
    recap_db_dsn: str = Field(
        default="postgres://recap_user:recap_pass@localhost:5432/recap",
        description="PostgreSQL connection string for recap-db",
    )
    db_pool_min_size: int = Field(default=5, description="Minimum database pool size")
    db_pool_max_size: int = Field(default=20, description="Maximum database pool size")

    # Ollama
    ollama_url: str = Field(
        default="http://localhost:11434",
        description="Ollama API base URL",
    )
    ollama_model: str = Field(
        default="gemma3-4b-16k",
        description="Ollama model to use for G-Eval (shares with news-creator)",
    )
    ollama_timeout: int = Field(
        default=120,
        description="Ollama request timeout in seconds",
    )

    # Recap Worker API (for genre evaluation)
    recap_worker_url: str = Field(
        default="http://localhost:8081",
        description="Recap worker API base URL",
    )

    # Evaluation Settings
    evaluation_window_days: int = Field(
        default=14,
        description="Number of days to look back for job history",
    )
    geval_sample_size: int = Field(
        default=50,
        description="Number of summaries to sample for G-Eval",
    )

    # Scheduler
    evaluation_schedule: str = Field(
        default="0 6 * * *",
        description="Cron expression for scheduled evaluation (default: daily at 6am)",
    )
    enable_scheduler: bool = Field(
        default=True,
        description="Enable scheduled evaluation runs",
    )

    # Logging
    log_level: str = Field(default="INFO", description="Logging level")
    log_format: str = Field(default="json", description="Log format (json or console)")

    # Server
    host: str = Field(default="0.0.0.0", description="Server host")
    port: int = Field(default=8080, description="Server port")


class AlertThresholds(BaseSettings):
    """Alert thresholds for quality metrics."""

    model_config = SettingsConfigDict(env_prefix="ALERT_")

    # Genre Classification
    genre_macro_f1_warn: float = Field(default=0.70)
    genre_macro_f1_critical: float = Field(default=0.60)

    # Clustering
    clustering_silhouette_warn: float = Field(default=0.25)
    clustering_silhouette_critical: float = Field(default=0.15)

    # G-Eval Summary Quality
    geval_coherence_warn: float = Field(default=3.5)
    geval_coherence_critical: float = Field(default=3.0)
    geval_consistency_warn: float = Field(default=3.5)
    geval_consistency_critical: float = Field(default=3.0)
    geval_fluency_warn: float = Field(default=3.5)
    geval_fluency_critical: float = Field(default=3.0)
    geval_relevance_warn: float = Field(default=3.5)
    geval_relevance_critical: float = Field(default=3.0)

    # Pipeline Health
    pipeline_success_rate_warn: float = Field(default=0.95)
    pipeline_success_rate_critical: float = Field(default=0.90)

    def get_threshold(self, metric_name: str) -> AlertThreshold | None:
        """Get alert threshold for a given metric."""
        warn_attr = f"{metric_name}_warn"
        critical_attr = f"{metric_name}_critical"

        if hasattr(self, warn_attr) and hasattr(self, critical_attr):
            return AlertThreshold(
                warn=getattr(self, warn_attr),
                critical=getattr(self, critical_attr),
            )
        return None


# Singleton instances
settings = Settings()
alert_thresholds = AlertThresholds()
