"""Backward-compatible re-export from infra.config."""

from tag_generator.infra.config import (
    BatchConfig,
    OTelEnvConfig,
    RedisConfig,
    TagGeneratorConfig,
)

__all__ = [
    "BatchConfig",
    "OTelEnvConfig",
    "RedisConfig",
    "TagGeneratorConfig",
]
