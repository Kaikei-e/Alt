"""Backward-compatible re-export from infra.config."""

from tag_generator.infra.config import (
    BatchConfig,
    DatabaseConfig,
    OTelEnvConfig,
    RedisConfig,
    TagGeneratorConfig,
)

__all__ = [
    "BatchConfig",
    "DatabaseConfig",
    "OTelEnvConfig",
    "RedisConfig",
    "TagGeneratorConfig",
]
