"""Centralized configuration for the tag-generator service.

All settings are read from environment variables via pydantic-settings.
Sub-configs are nested for clear grouping (database, redis, otel, etc.).
"""

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class DatabaseConfig(BaseSettings):
    """Database connection settings."""

    model_config = SettingsConfigDict(env_prefix="DB_")

    host: str = "localhost"
    port: int = 5432
    name: str = "alt"
    tag_generator_user: str = Field(default="tag_generator", alias="DB_TAG_GENERATOR_USER")
    tag_generator_password: str = Field(default="", alias="DB_TAG_GENERATOR_PASSWORD")
    tag_generator_password_file: str | None = Field(default=None, alias="DB_TAG_GENERATOR_PASSWORD_FILE")
    sslmode: str = "prefer"


class RedisConfig(BaseSettings):
    """Redis connection settings."""

    model_config = SettingsConfigDict(env_prefix="REDIS_")

    streams_url: str = Field(default="redis://localhost:6379", alias="REDIS_STREAMS_URL")


class OTelEnvConfig(BaseSettings):
    """OpenTelemetry settings read from environment."""

    model_config = SettingsConfigDict(env_prefix="OTEL_")

    service_name: str = Field(default="tag-generator", alias="OTEL_SERVICE_NAME")
    enabled: bool = Field(default=True, alias="OTEL_ENABLED")
    endpoint: str = Field(default="http://localhost:4318", alias="OTEL_EXPORTER_OTLP_ENDPOINT")


class BatchConfig(BaseSettings):
    """Batch processing tuning knobs."""

    model_config = SettingsConfigDict(env_prefix="TAG_")

    batch_limit: int = Field(default=75, gt=0, le=1000)
    progress_log_interval: int = Field(default=10, gt=0)
    enable_gc_collection: bool = True
    memory_cleanup_interval: int = Field(default=25, gt=0)


class TagGeneratorConfig(BaseSettings):
    """Top-level configuration for the tag generation service.

    All fields can be overridden via environment variables with the TAG_ prefix.
    Example: TAG_BATCH_LIMIT=100 overrides batch_limit.
    """

    model_config = SettingsConfigDict(env_prefix="TAG_")

    processing_interval: int = Field(default=300, gt=0, description="Seconds between processing batches when idle")
    active_processing_interval: int = Field(
        default=180, gt=0, description="Seconds between processing batches when work is pending"
    )
    error_retry_interval: int = Field(default=60, gt=0, description="Seconds to wait after errors")
    batch_limit: int = Field(default=75, gt=0, le=1000, description="Articles per processing cycle")
    progress_log_interval: int = Field(default=10, gt=0, description="Log progress every N articles")
    enable_gc_collection: bool = Field(default=True, description="Enable manual garbage collection")
    memory_cleanup_interval: int = Field(default=25, gt=0, description="Articles between memory cleanup")
    max_connection_retries: int = Field(default=3, gt=0, description="Max database connection retries")
    connection_retry_delay: float = Field(default=5.0, gt=0, description="Seconds between connection attempts")
    # Health monitoring
    health_check_interval: int = Field(default=10, gt=0, description="Cycles between health checks")
    max_consecutive_empty_cycles: int = Field(default=20, gt=0, description="Max cycles with 0 articles before warning")
