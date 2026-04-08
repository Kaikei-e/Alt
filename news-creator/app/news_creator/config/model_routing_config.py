"""Model Routing Configuration dataclass (Phase 1 refactoring).

Following Python 3.14 best practices:
- Frozen dataclass for immutable configuration
- Factory method for environment loading
"""

from __future__ import annotations

import os
import logging
from dataclasses import dataclass, field
from typing import FrozenSet

logger = logging.getLogger(__name__)


@dataclass(frozen=True)
class ModelRoutingConfig:
    """Immutable configuration for model bucket routing."""

    enabled: bool = True
    base_name: str = "gemma4-e4b-q4km"
    model_8k_name: str = "gemma4-e4b-q4km"
    model_60k_name: str = "gemma4-e4b-60k"
    model_60k_enabled: bool = False
    token_safety_margin_percent: int = 10
    token_safety_margin_fixed: int = 512
    oom_detection_enabled: bool = True
    warmup_enabled: bool = True
    warmup_keep_alive_minutes: int = 30
    # Derived field for quick lookup
    _bucket_model_names: FrozenSet[str] = field(default_factory=frozenset, repr=False)

    def __post_init__(self):
        """Initialize derived fields after dataclass construction."""
        # Use object.__setattr__ because frozen=True
        object.__setattr__(
            self,
            "_bucket_model_names",
            frozenset({self.model_8k_name, self.model_60k_name}),
        )

    def is_base_model_name(self, model_name: str) -> bool:
        """Check if the given model name is the base model name."""
        return model_name == self.base_name

    def is_bucket_model_name(self, model_name: str) -> bool:
        """Check if the given model name is a bucket model name."""
        return model_name in self._bucket_model_names

    @classmethod
    def from_env(cls) -> ModelRoutingConfig:
        """Create ModelRoutingConfig from environment variables."""
        return cls(
            enabled=os.getenv("MODEL_ROUTING_ENABLED", "true").lower() == "true",
            base_name=os.getenv("MODEL_BASE_NAME", "gemma4-e4b-q4km"),
            model_8k_name=os.getenv("MODEL_8K_NAME", "gemma4-e4b-q4km"),
            model_60k_name=os.getenv("MODEL_60K_NAME", "gemma4-e4b-60k"),
            model_60k_enabled=os.getenv("MODEL_60K_ENABLED", "false").lower() == "true",
            token_safety_margin_percent=_get_int("TOKEN_SAFETY_MARGIN_PERCENT", 10),
            token_safety_margin_fixed=_get_int("TOKEN_SAFETY_MARGIN_FIXED", 512),
            oom_detection_enabled=os.getenv("OOM_DETECTION_ENABLED", "true").lower()
            == "true",
            warmup_enabled=os.getenv("WARMUP_ENABLED", "true").lower() == "true",
            warmup_keep_alive_minutes=_get_int("WARMUP_KEEP_ALIVE_MINUTES", 30),
        )


def _get_int(name: str, default: int) -> int:
    """Get integer value from environment variable with fallback."""
    try:
        return int(os.getenv(name, default))
    except ValueError:
        logger.warning("Invalid int for %s. Using default %s", name, default)
        return default
