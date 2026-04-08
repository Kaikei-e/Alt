"""Scheduling Configuration dataclass (Phase 1 refactoring).

Following Python 3.14 best practices:
- Frozen dataclass for immutable configuration
- Factory method for environment loading
"""

from __future__ import annotations

import os
import logging
from dataclasses import dataclass

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class SchedulingConfig:
    """Immutable configuration for RT/BE scheduling settings."""

    rt_reserved_slots: int = 1
    aging_threshold_seconds: float = 60.0
    aging_boost: float = 0.5
    preemption_enabled: bool = True
    preemption_wait_threshold_seconds: float = 2.0
    priority_promotion_threshold_seconds: float = 120.0
    guaranteed_be_ratio: int = 5
    max_queue_depth: int = 10
    rt_mode: str = "fifo"
    # Concurrency settings
    request_concurrency: int = 1
    concurrency_source: str = "OLLAMA_NUM_PARALLEL"

    @classmethod
    def from_env(cls) -> SchedulingConfig:
        """Create SchedulingConfig from environment variables."""
        # Concurrency: OLLAMA_REQUEST_CONCURRENCY takes precedence over OLLAMA_NUM_PARALLEL
        env_concurrency = os.getenv("OLLAMA_REQUEST_CONCURRENCY")
        if env_concurrency is not None:
            request_concurrency = _get_int("OLLAMA_REQUEST_CONCURRENCY", 1)
            concurrency_source = "OLLAMA_REQUEST_CONCURRENCY"
        else:
            request_concurrency = _get_int("OLLAMA_NUM_PARALLEL", 1)
            concurrency_source = "OLLAMA_NUM_PARALLEL"

        return cls(
            rt_reserved_slots=_get_int("SCHEDULING_RT_RESERVED_SLOTS", 1),
            aging_threshold_seconds=_get_float(
                "SCHEDULING_AGING_THRESHOLD_SECONDS", 60.0
            ),
            aging_boost=_get_float("SCHEDULING_AGING_BOOST", 0.5),
            preemption_enabled=os.getenv(
                "SCHEDULING_PREEMPTION_ENABLED", "true"
            ).lower()
            == "true",
            preemption_wait_threshold_seconds=_get_float(
                "SCHEDULING_PREEMPTION_WAIT_THRESHOLD_SECONDS", 2.0
            ),
            priority_promotion_threshold_seconds=_get_float(
                "SCHEDULING_PRIORITY_PROMOTION_THRESHOLD_SECONDS", 120.0
            ),
            guaranteed_be_ratio=_get_int("SCHEDULING_GUARANTEED_BE_RATIO", 5),
            max_queue_depth=_get_int("MAX_QUEUE_DEPTH", 10),
            rt_mode=os.getenv("SCHEDULING_RT_MODE", "fifo").lower(),
            request_concurrency=request_concurrency,
            concurrency_source=concurrency_source,
        )


def _get_int(name: str, default: int) -> int:
    """Get integer value from environment variable with fallback."""
    try:
        return int(os.getenv(name, default))
    except ValueError:
        logger.warning("Invalid int for %s. Using default %s", name, default)
        return default


def _get_float(name: str, default: float) -> float:
    """Get float value from environment variable with fallback."""
    try:
        return float(os.getenv(name, default))
    except ValueError:
        logger.warning("Invalid float for %s. Using default %s", name, default)
        return default
