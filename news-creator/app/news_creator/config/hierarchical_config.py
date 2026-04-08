"""Hierarchical Summarization Configuration dataclass (Phase 1 refactoring).

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
class HierarchicalConfig:
    """Immutable configuration for hierarchical map-reduce summarization."""

    threshold_chars: int = 8_000
    threshold_clusters: int = 5
    chunk_max_chars: int = 6_000
    chunk_overlap_ratio: float = 0.15
    recursive_reduce_max_chars: int = 6_000
    recursive_reduce_max_depth: int = 3
    single_article_threshold: int = 20_000
    single_article_chunk_size: int = 6_000
    token_budget_percent: int = 75

    @classmethod
    def from_env(cls) -> HierarchicalConfig:
        """Create HierarchicalConfig from environment variables."""
        return cls(
            threshold_chars=_get_int("HIERARCHICAL_THRESHOLD_CHARS", 8_000),
            threshold_clusters=_get_int("HIERARCHICAL_THRESHOLD_CLUSTERS", 5),
            chunk_max_chars=_get_int("HIERARCHICAL_CHUNK_MAX_CHARS", 6_000),
            chunk_overlap_ratio=_get_float("HIERARCHICAL_CHUNK_OVERLAP_RATIO", 0.15),
            recursive_reduce_max_chars=_get_int("RECURSIVE_REDUCE_MAX_CHARS", 6_000),
            recursive_reduce_max_depth=_get_int("RECURSIVE_REDUCE_MAX_DEPTH", 3),
            single_article_threshold=_get_int("HIERARCHICAL_SINGLE_ARTICLE_THRESHOLD", 20_000),
            single_article_chunk_size=_get_int("HIERARCHICAL_SINGLE_ARTICLE_CHUNK_SIZE", 6_000),
            token_budget_percent=_get_int("HIERARCHICAL_TOKEN_BUDGET_PERCENT", 75),
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
