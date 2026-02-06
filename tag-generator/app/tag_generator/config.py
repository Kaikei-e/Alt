"""Configuration for tag generator service."""

from pydantic import Field
from pydantic_settings import BaseSettings, SettingsConfigDict


class TagGeneratorConfig(BaseSettings):
    """Configuration for the tag generation service.

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
