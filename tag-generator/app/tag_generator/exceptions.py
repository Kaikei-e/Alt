"""Backward-compatible re-exports from domain.errors."""

from tag_generator.domain.errors import (
    BatchProcessingError,
    CursorError,
    DatabaseConnectionError,
    ModelLoadError,
    TagExtractionError,
    TagGeneratorError,
)

__all__ = [
    "BatchProcessingError",
    "CursorError",
    "DatabaseConnectionError",
    "ModelLoadError",
    "TagExtractionError",
    "TagGeneratorError",
]
