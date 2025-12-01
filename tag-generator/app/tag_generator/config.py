"""Configuration for tag generator service."""

from dataclasses import dataclass


@dataclass
class TagGeneratorConfig:
    """Configuration for the tag generation service."""

    processing_interval: int = 300  # seconds between processing batches when idle (5 minutes)
    active_processing_interval: int = 180  # seconds between processing batches when work is pending (3 minutes)
    error_retry_interval: int = 60  # seconds to wait after errors
    batch_limit: int = 75  # articles per processing cycle
    progress_log_interval: int = 10  # log progress every N articles
    enable_gc_collection: bool = True  # enable manual garbage collection
    memory_cleanup_interval: int = 25  # articles between memory cleanup
    max_connection_retries: int = 3  # max database connection retries
    connection_retry_delay: float = 5.0  # seconds between connection attempts
    # Health monitoring
    health_check_interval: int = 10  # cycles between health checks
    max_consecutive_empty_cycles: int = 20  # max cycles with 0 articles before warning
